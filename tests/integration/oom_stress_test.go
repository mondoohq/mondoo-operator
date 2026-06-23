// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mondoov2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers/container_image"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
)

const (
	oomStressNamespace = "oom-stress-targets"

	// How long to wait for the scan pod to start and either OOM or complete.
	oomStressTimeout = 25 * time.Minute

	// Poll interval for checking pod status.
	oomStressPollInterval = 5 * time.Second
)

// stressImage is a public container image with a real package manager.
// These are chosen to maximize the number of packages discovered per image,
// which increases heap pressure in the scanner process.
type stressImage struct {
	name  string
	image string
}

var defaultStressImages = []stressImage{
	{name: "ubuntu-2404", image: "ubuntu:24.04"},
	{name: "ubuntu-2204", image: "ubuntu:22.04"},
	{name: "ubuntu-2004", image: "ubuntu:20.04"},
	{name: "debian-12", image: "debian:12"},
	{name: "debian-11", image: "debian:11"},
	{name: "fedora-40", image: "fedora:40"},
	{name: "fedora-39", image: "fedora:39"},
	{name: "amazonlinux-2023", image: "amazonlinux:2023"},
	{name: "amazonlinux-2", image: "amazonlinux:2"},
	{name: "rockylinux-9", image: "rockylinux:9"},
	{name: "almalinux-9", image: "almalinux:9"},
	{name: "oraclelinux-9", image: "oraclelinux:9"},
	{name: "alpine-319", image: "alpine:3.19"},
	{name: "python-312", image: "python:3.12"},
	{name: "python-311", image: "python:3.11"},
	{name: "node-20", image: "node:20"},
	{name: "node-22", image: "node:22"},
	{name: "ruby-33", image: "ruby:3.3"},
}

// OOMStressSuite reproduces the OOM condition observed in SVA's container
// image scanning. It deploys many distinct, package-heavy images and runs
// a container image scan with reduced memory limits. The test asserts that
// the scan pod is OOM-killed, confirming the memory accumulation issue.
//
// This suite embeds AuditConfigBaseSuite so it can be trivially patched into
// the main integration test flow later. It uses the same K8sHelper, installer,
// and log-gathering infrastructure.
//
// Run with: go test -v -timeout 30m -run TestOOMStressSuite ./tests/integration/
// Requires: OOM_STRESS_TEST=1 env var (skipped otherwise to avoid accidental runs)
type OOMStressSuite struct {
	AuditConfigBaseSuite
	targetNamespaceCreated bool
	registryHost           string
}

func (s *OOMStressSuite) SetupSuite() {
	s.AuditConfigBaseSuite.SetupSuite()
	s.testCluster.Settings.SuiteName = "OOMStressSuite"
}

func (s *OOMStressSuite) TearDownSuite() {
	s.cleanupTargetNamespace()
	s.AuditConfigBaseSuite.TearDownSuite()
}

func (s *OOMStressSuite) AfterTest(suiteName, testName string) {
	s.AuditConfigBaseSuite.AfterTest(suiteName, testName)
	s.cleanupTargetNamespace()
}

func (s *OOMStressSuite) cleanupTargetNamespace() {
	if !s.targetNamespaceCreated {
		return
	}
	ctx := context.Background()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: oomStressNamespace}}
	if err := s.testCluster.K8sHelper.Clientset.Delete(ctx, ns); err != nil {
		zap.S().Warnf("Failed to delete stress test namespace: %v", err)
	}
	s.targetNamespaceCreated = false
}

// deployLocalRegistry deploys an in-cluster Docker registry and seeds it with
// all stress images using crane. This avoids Docker Hub rate limits during
// scanning — Docker Hub is only hit once (by the crane Job), and the scanner
// pulls from the local registry.
func (s *OOMStressSuite) deployLocalRegistry(images []stressImage) {
	ctx := context.Background()
	registryLabels := map[string]string{"app": "oom-stress-registry"}
	s.registryHost = fmt.Sprintf("registry.%s.svc.cluster.local", oomStressNamespace)

	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registry",
			Namespace: oomStressNamespace,
			Labels:    registryLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: registryLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: registryLabels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "registry",
						Image: "registry:2",
						Ports: []corev1.ContainerPort{{ContainerPort: 5000}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					}},
				},
			},
		},
	}
	s.Require().NoError(s.testCluster.K8sHelper.Clientset.Create(ctx, dep), "Failed to create registry deployment")

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registry",
			Namespace: oomStressNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: registryLabels,
			Ports: []corev1.ServicePort{{
				Port:       80,
				TargetPort: intstr.FromInt32(5000),
			}},
		},
	}
	s.Require().NoError(s.testCluster.K8sHelper.Clientset.Create(ctx, svc), "Failed to create registry service")

	zap.S().Info("Waiting for registry pod to be ready...")
	s.Require().True(
		s.testCluster.K8sHelper.IsPodReady("app=oom-stress-registry", oomStressNamespace),
		"Registry pod did not become ready")

	// Build the crane copy script.
	var copies []string
	for _, img := range images {
		copies = append(copies, fmt.Sprintf(
			`echo "Copying %s..." && crane copy "docker.io/library/%s" "%s/%s" --insecure`,
			img.image, img.image, s.registryHost, img.image))
	}

	var loginCmd string
	if u := os.Getenv("DOCKER_HUB_USERNAME"); u != "" {
		p := os.Getenv("DOCKER_HUB_PASSWORD")
		loginCmd = fmt.Sprintf("crane auth login -u '%s' -p '%s' index.docker.io", u, p)
	} else {
		loginCmd = "echo 'No Docker Hub credentials, using anonymous pulls'"
	}

	script := fmt.Sprintf("set -e\n%s\n%s\necho 'All images seeded.'",
		loginCmd, strings.Join(copies, "\n"))

	backoff := int32(2)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "seed-registry",
			Namespace: oomStressNamespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					InitContainers: []corev1.Container{{
						Name:    "wait-for-registry",
						Image:   "busybox:1.36",
						Command: []string{"sh", "-c", fmt.Sprintf("until wget -qO- http://%s/v2/ >/dev/null 2>&1; do echo 'waiting for registry...'; sleep 2; done", s.registryHost)},
					}},
					Containers: []corev1.Container{{
						Name:    "crane",
						Image:   "gcr.io/go-containerregistry/crane:latest",
						Command: []string{"sh", "-c", script},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					}},
				},
			},
		},
	}
	s.Require().NoError(s.testCluster.K8sHelper.Clientset.Create(ctx, job), "Failed to create seed-registry job")

	zap.S().Info("Waiting for seed-registry job to complete (this pulls from Docker Hub once)...")
	err := s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		cur := &batchv1.Job{}
		if err := s.testCluster.K8sHelper.Clientset.Get(ctx, client.ObjectKeyFromObject(job), cur); err != nil {
			return false, nil
		}
		for _, c := range cur.Status.Conditions {
			if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
				return true, nil
			}
			if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
				return false, fmt.Errorf("seed-registry job failed: %s", c.Message)
			}
		}
		return false, nil
	})
	s.Require().NoError(err, "seed-registry job did not complete successfully")

	zap.S().Info("Local registry seeded with all stress images.")
}

// deployStressTargets creates the target namespace, deploys a local registry,
// seeds it with images, and creates pods referencing the local registry.
func (s *OOMStressSuite) deployStressTargets(images []stressImage) {
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: oomStressNamespace}}
	s.Require().NoError(s.testCluster.K8sHelper.Clientset.Create(ctx, ns), "Failed to create stress target namespace")
	s.targetNamespaceCreated = true

	s.deployLocalRegistry(images)

	for _, img := range images {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      img.name,
				Namespace: oomStressNamespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":    img.name,
					"app.kubernetes.io/part-of": "oom-stress-test",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:    "target",
					Image:   fmt.Sprintf("%s/%s", s.registryHost, img.image),
					Command: []string{"sleep", "infinity"},
				}},
			},
		}
		s.Require().NoErrorf(
			s.testCluster.K8sHelper.Clientset.Create(ctx, pod),
			"Failed to create stress target pod %s", img.name)
	}

	zap.S().Infof("Deployed %d stress target pods, waiting for them to be ready...", len(images))

	allReady := s.testCluster.K8sHelper.IsPodReady(
		"app.kubernetes.io/part-of=oom-stress-test", oomStressNamespace)
	s.Require().True(allReady, "Not all stress target pods became ready")

	zap.S().Info("All stress target pods are ready.")
}

// oomStressAuditConfig returns a MondooAuditConfig configured for the stress
// test: container scanning enabled with reduced memory limits, targeting the
// stress namespace.
func (s *OOMStressSuite) oomStressAuditConfig(memoryLimit string) mondoov2.MondooAuditConfig {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, true, false)

	auditConfig.Spec.Containers.Resources = corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(memoryLimit),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}

	// Only scan the stress namespace — exclude everything else so we control
	// exactly which images are scanned.
	auditConfig.Spec.Filtering.Namespaces.Include = []string{oomStressNamespace}

	return auditConfig
}

// waitForScanPodTermination waits for the container scan Job's pod to reach a
// terminal state (Succeeded, Failed, or OOMKilled). Returns the terminal pod.
func (s *OOMStressSuite) waitForScanPodTermination(auditConfig mondoov2.MondooAuditConfig) *corev1.Pod {
	ctx := context.Background()
	cronJobLabels := container_image.CronJobLabels(auditConfig)
	listOpts := &client.ListOptions{
		Namespace:     auditConfig.Namespace,
		LabelSelector: labels.SelectorFromSet(cronJobLabels),
	}

	deadline := time.Now().Add(oomStressTimeout)
	var lastPod *corev1.Pod

	for time.Now().Before(deadline) {
		podList := &corev1.PodList{}
		if err := s.testCluster.K8sHelper.Clientset.List(ctx, podList, listOpts); err != nil {
			zap.S().Warnf("Failed to list scan pods: %v", err)
			time.Sleep(oomStressPollInterval)
			continue
		}

		for i := range podList.Items {
			pod := &podList.Items[i]
			lastPod = pod

			switch pod.Status.Phase {
			case corev1.PodSucceeded:
				return pod
			case corev1.PodFailed:
				return pod
			case corev1.PodRunning:
				// Check for OOMKilled containers that haven't caused pod failure yet
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
						return pod
					}
					if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
						return pod
					}
				}
			}
		}
		time.Sleep(oomStressPollInterval)
	}

	if lastPod != nil {
		return lastPod
	}
	s.Fail("Timed out waiting for scan pod to appear")
	return nil
}

// isOOMKilled checks whether any container in the pod was OOM-killed.
func isOOMKilled(pod *corev1.Pod) bool {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
			return true
		}
		if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
			return true
		}
	}
	// Also check exit code 137 (SIGKILL from cgroup OOM)
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode == 137 {
			return true
		}
	}
	return false
}

// collectScanLogs retrieves logs from the scan pod for analysis.
func (s *OOMStressSuite) collectScanLogs(pod *corev1.Pod) string {
	if pod == nil {
		return ""
	}
	logs, err := s.testCluster.K8sHelper.Kubectl(
		"logs", "-n", pod.Namespace, pod.Name, "--tail=500")
	if err != nil {
		zap.S().Warnf("Failed to collect scan pod logs: %v", err)
		return ""
	}
	return logs
}

func countCompletedImages(logs string) int {
	return strings.Count(logs, "successfully uploaded")
}

// TestContainerScan_OOMUnderMemoryPressure is the main stress test.
// It deploys 18 package-heavy container images, runs a scan with a 512Mi
// memory limit, and asserts the scan pod is OOM-killed.
func (s *OOMStressSuite) TestContainerScan_OOMUnderMemoryPressure() {
	memoryLimit := envOrDefault("OOM_STRESS_MEMORY_LIMIT", "512Mi")

	zap.S().Infof("Starting OOM stress test with memory limit %s and %d images", memoryLimit, len(defaultStressImages))

	// 1. Deploy target pods with package-heavy images
	s.deployStressTargets(defaultStressImages)

	// 2. Create audit config with reduced memory
	auditConfig := s.oomStressAuditConfig(memoryLimit)
	s.auditConfig = auditConfig

	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	zap.S().Info("Creating MondooAuditConfig for stress test...")
	s.Require().NoError(
		s.testCluster.K8sHelper.Clientset.Create(context.Background(), &auditConfig),
		"Failed to create MondooAuditConfig")

	// 3. Wait for the CronJob to be created
	zap.S().Info("Waiting for container scan CronJob...")
	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      container_image.CronJobName(auditConfig.Name),
			Namespace: auditConfig.Namespace,
		},
	}
	err := s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		if err := s.testCluster.K8sHelper.Clientset.Get(
			context.Background(), client.ObjectKeyFromObject(cronJob), cronJob); err != nil {
			return false, nil
		}
		return true, nil
	})
	s.Require().NoError(err, "Container scan CronJob was not created")

	// 4. Wait for scan pod to terminate (OOM or complete)
	zap.S().Info("Waiting for scan pod to terminate...")
	pod := s.waitForScanPodTermination(auditConfig)
	s.Require().NotNil(pod, "Scan pod never appeared")

	// 5. Collect and log results
	logs := s.collectScanLogs(pod)
	imagesCompleted := countCompletedImages(logs)

	zap.S().Infof("Scan pod terminated. Phase: %s, OOMKilled: %v, Images completed: %d/%d",
		pod.Status.Phase, isOOMKilled(pod), imagesCompleted, len(defaultStressImages))

	// Log container statuses for debugging
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil {
			zap.S().Infof("Container %s: reason=%s, exitCode=%d",
				cs.Name, cs.State.Terminated.Reason, cs.State.Terminated.ExitCode)
		}
	}

	// 6. Assert OOM
	s.True(isOOMKilled(pod),
		fmt.Sprintf("Expected scan pod to be OOM-killed, but pod phase is %s. "+
			"The scanner completed %d/%d images without running out of memory. "+
			"Try lowering OOM_STRESS_MEMORY_LIMIT (currently %s) or adding more images.",
			pod.Status.Phase, imagesCompleted, len(defaultStressImages), memoryLimit))

	s.Less(imagesCompleted, len(defaultStressImages),
		"Scanner should not complete all images before OOM")

	zap.S().Infof("OOM stress test passed: scanner OOM-killed after %d/%d images at %s memory limit",
		imagesCompleted, len(defaultStressImages), memoryLimit)
}

// TestContainerScan_CompletesWithAdequateMemory is the inverse test.
// Same images, but with 4Gi memory — the scan should complete successfully.
// This confirms the fix path: increasing memory resolves the OOM.
func (s *OOMStressSuite) TestContainerScan_CompletesWithAdequateMemory() {
	if os.Getenv("OOM_STRESS_FULL") != "1" {
		s.T().Skip("Skipping adequate-memory test (set OOM_STRESS_FULL=1 to run)")
	}

	memoryLimit := envOrDefault("OOM_STRESS_ADEQUATE_MEMORY", "4Gi")

	zap.S().Infof("Starting adequate-memory test with %s limit and %d images", memoryLimit, len(defaultStressImages))

	s.deployStressTargets(defaultStressImages)

	auditConfig := s.oomStressAuditConfig(memoryLimit)
	s.auditConfig = auditConfig

	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	s.Require().NoError(
		s.testCluster.K8sHelper.Clientset.Create(context.Background(), &auditConfig),
		"Failed to create MondooAuditConfig")

	cronJobLabels := container_image.CronJobLabels(auditConfig)
	s.True(
		s.testCluster.K8sHelper.WaitUntilCronJobsSuccessful(
			utils.LabelsToLabelSelector(cronJobLabels), auditConfig.Namespace),
		"Container scan should complete successfully with adequate memory")

	zap.S().Infof("Adequate-memory test passed: scanner completed all images at %s", memoryLimit)
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func TestOOMStressSuite(t *testing.T) {
	if os.Getenv("OOM_STRESS_TEST") != "1" {
		t.Skip("OOM stress test skipped (set OOM_STRESS_TEST=1 to run)")
	}

	s := new(OOMStressSuite)
	defer func(s *OOMStressSuite) {
		HandlePanics(recover(), func() {
			if err := s.testCluster.UninstallOperator(); err != nil {
				zap.S().Errorf("Failed to uninstall Mondoo operator. %v", err)
			}
			if s.spaceClient != nil {
				if err := s.spaceClient.Delete(s.ctx); err != nil {
					zap.S().Errorf("Failed to delete Mondoo space. %v", err)
				}
			}
		}, s.T)
	}(s)
	suite.Run(t, s)
}
