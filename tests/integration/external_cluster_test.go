// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/tests/framework/utils"
)

const (
	targetClusterName = "mondoo-target"
	testNamespace     = "external-cluster-test"
)

// ExternalClusterSuite tests external cluster scanning infrastructure with k3d.
// This test verifies that k3d clusters can communicate and kubeconfig secrets work.
// It does NOT require Mondoo credentials - it only tests the infrastructure setup.
type ExternalClusterSuite struct {
	suite.Suite
	ctx               context.Context
	k8sHelper         *utils.K8sHelper
	targetClusterIP   string
	managementContext string
}

func (s *ExternalClusterSuite) SetupSuite() {
	// Only run on k3d
	distro := os.Getenv("K8S_DISTRO")
	if distro != "k3d" {
		s.T().Skip("External cluster test requires k3d (K8S_DISTRO=k3d)")
	}

	cfg := zap.NewDevelopmentConfig()
	logger, _ := cfg.Build()
	zap.ReplaceGlobals(logger)

	s.ctx = context.Background()

	// Create K8s helper
	var err error
	s.k8sHelper, err = utils.CreateK8sHelper()
	s.Require().NoError(err, "Failed to create K8s helper")

	// Verify a management cluster exists
	if err := s.verifyManagementClusterExists(); err != nil {
		s.T().Fatalf("Management cluster not found: %v. Create one with 'k3d cluster create <name>' first.", err)
	}

	// Save the management cluster context
	if err := s.saveManagementContext(); err != nil {
		s.T().Fatalf("Failed to get management cluster context: %v", err)
	}

	// Create target k3d cluster
	zap.S().Info("Creating target k3d cluster...")
	if err := s.createTargetCluster(); err != nil {
		s.T().Fatalf("Failed to create target cluster: %v", err)
	}

	// Switch back to management cluster context
	zap.S().Infof("Switching back to management cluster context: %s", s.managementContext)
	if err := s.switchToManagementContext(); err != nil {
		s.cleanupTargetCluster()
		s.T().Fatalf("Failed to switch to management context: %v", err)
	}

	// Connect Docker networks
	zap.S().Info("Connecting Docker networks...")
	if err := s.connectDockerNetworks(); err != nil {
		s.cleanupTargetCluster()
		s.T().Fatalf("Failed to connect Docker networks: %v", err)
	}

	// Get target cluster IP
	zap.S().Info("Getting target cluster IP...")
	if err := s.getTargetClusterIP(); err != nil {
		s.cleanupTargetCluster()
		s.T().Fatalf("Failed to get target cluster IP: %v", err)
	}

	// Create test namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
	}
	if err := s.k8sHelper.Clientset.Create(s.ctx, ns); err != nil {
		// Ignore if already exists
		if !strings.Contains(err.Error(), "already exists") {
			s.cleanupTargetCluster()
			s.T().Fatalf("Failed to create test namespace: %v", err)
		}
	}
}

func (s *ExternalClusterSuite) TearDownSuite() {
	// Clean up namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
	}
	_ = s.k8sHelper.Clientset.Delete(s.ctx, ns)

	s.cleanupTargetCluster()
}

func (s *ExternalClusterSuite) verifyManagementClusterExists() error {
	cmd := exec.Command("kubectl", "cluster-info")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("no kubernetes cluster available: %v, output: %s", err, string(output))
	}

	cmd = exec.Command("docker", "ps", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list docker containers: %v", err)
	}

	containers := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, c := range containers {
		if strings.Contains(c, "server-0") && !strings.Contains(c, targetClusterName) {
			zap.S().Infof("Found existing management cluster container: %s", c)
			return nil
		}
	}

	return fmt.Errorf("no k3d management cluster container found")
}

func (s *ExternalClusterSuite) saveManagementContext() error {
	cmd := exec.Command("kubectl", "config", "current-context")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get current context: %v", err)
	}
	s.managementContext = strings.TrimSpace(string(output))
	zap.S().Infof("Management cluster context: %s", s.managementContext)
	return nil
}

func (s *ExternalClusterSuite) switchToManagementContext() error {
	cmd := exec.Command("kubectl", "config", "use-context", s.managementContext)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to switch to management context %s: %v, output: %s", s.managementContext, err, string(output))
	}
	return nil
}

func (s *ExternalClusterSuite) createTargetCluster() error {
	cmd := exec.Command("k3d", "cluster", "create", targetClusterName, "--api-port", "6444")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create target cluster: %v, output: %s", err, string(output))
	}
	return nil
}

func (s *ExternalClusterSuite) cleanupTargetCluster() {
	zap.S().Info("Cleaning up target cluster...")
	cmd := exec.Command("k3d", "cluster", "delete", targetClusterName)
	if output, err := cmd.CombinedOutput(); err != nil {
		zap.S().Warnf("Failed to delete target cluster: %v, output: %s", err, string(output))
	}
}

func (s *ExternalClusterSuite) connectDockerNetworks() error {
	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list docker containers: %v", err)
	}

	containers := strings.Split(strings.TrimSpace(string(output)), "\n")
	var managementContainer string
	for _, c := range containers {
		if strings.Contains(c, "server-0") && !strings.Contains(c, targetClusterName) {
			managementContainer = c
			zap.S().Infof("Found management cluster container: %s", managementContainer)
			break
		}
	}

	if managementContainer == "" {
		return fmt.Errorf("could not find management cluster container. Available containers: %v", containers)
	}

	targetNetwork := fmt.Sprintf("k3d-%s", targetClusterName)
	cmd = exec.Command("docker", "network", "connect", targetNetwork, managementContainer)
	if output, err := cmd.CombinedOutput(); err != nil {
		if !strings.Contains(string(output), "already exists") {
			return fmt.Errorf("failed to connect networks: %v, output: %s", err, string(output))
		}
		zap.S().Infof("Container %s already connected to network %s", managementContainer, targetNetwork)
	} else {
		zap.S().Infof("Connected container %s to network %s", managementContainer, targetNetwork)
	}

	return nil
}

func (s *ExternalClusterSuite) getTargetClusterIP() error {
	cmd := exec.Command("docker", "inspect", fmt.Sprintf("k3d-%s-server-0", targetClusterName),
		"--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get target cluster IP: %v", err)
	}

	ips := strings.Fields(strings.TrimSpace(string(output)))
	if len(ips) == 0 {
		return fmt.Errorf("no IP address found for target cluster")
	}
	s.targetClusterIP = ips[0]
	zap.S().Infof("Target cluster IP: %s", s.targetClusterIP)
	return nil
}

func (s *ExternalClusterSuite) createKubeconfigSecret(secretName string) error {
	// Write kubeconfig to temp file
	tmpFile := "/tmp/external-cluster-test-kubeconfig.yaml"

	// Get kubeconfig from k3d
	cmd := exec.Command("k3d", "kubeconfig", "get", targetClusterName)
	kubeconfig, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get target kubeconfig: %v", err)
	}

	// Replace localhost with target cluster IP
	kubeconfigStr := string(kubeconfig)
	kubeconfigStr = strings.ReplaceAll(kubeconfigStr, "0.0.0.0:6444", fmt.Sprintf("%s:6443", s.targetClusterIP))
	kubeconfigStr = strings.ReplaceAll(kubeconfigStr, "127.0.0.1:6444", fmt.Sprintf("%s:6443", s.targetClusterIP))

	// Write to temp file
	if err := os.WriteFile(tmpFile, []byte(kubeconfigStr), 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig to temp file: %v", err)
	}

	// Use yq to add insecure-skip-tls-verify and remove certificate-authority-data
	// The k3d certificate was generated for localhost, not the Docker network IP
	cmd = exec.Command("yq", "-i",
		`(.clusters[].cluster.insecure-skip-tls-verify) = true | del(.clusters[].cluster.certificate-authority-data)`,
		tmpFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to modify kubeconfig with yq: %v, output: %s", err, string(output))
	}

	// Read the modified kubeconfig
	modifiedKubeconfig, err := os.ReadFile(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to read modified kubeconfig: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"kubeconfig": modifiedKubeconfig,
		},
	}

	return s.k8sHelper.Clientset.Create(s.ctx, secret)
}

// TestExternalCluster_KubeconfigConnectivity verifies that a pod in the management cluster
// can use a kubeconfig secret to connect to the target cluster.
func (s *ExternalClusterSuite) TestExternalCluster_KubeconfigConnectivity() {
	kubeconfigSecret := "target-kubeconfig"

	// Create kubeconfig secret
	zap.S().Info("Creating kubeconfig secret for target cluster...")
	s.Require().NoError(s.createKubeconfigSecret(kubeconfigSecret), "Failed to create kubeconfig secret")

	// Clean up secret after test
	defer func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kubeconfigSecret,
				Namespace: testNamespace,
			},
		}
		_ = s.k8sHelper.Clientset.Delete(s.ctx, secret)
	}()

	// Verify kubeconfig secret was created correctly
	secret := &corev1.Secret{}
	err := s.k8sHelper.Clientset.Get(s.ctx, client.ObjectKey{
		Name:      kubeconfigSecret,
		Namespace: testNamespace,
	}, secret)
	s.Require().NoError(err, "Failed to get kubeconfig secret")
	s.Contains(string(secret.Data["kubeconfig"]), s.targetClusterIP, "Kubeconfig should contain target cluster IP")

	// Create a test pod that uses the kubeconfig to connect to the target cluster
	zap.S().Info("Testing connectivity to target cluster from within a pod...")
	podName := "connectivity-test"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: testNamespace,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "test",
					Image:   "bitnami/kubectl:latest",
					Command: []string{"kubectl", "get", "nodes", "-o", "name"},
					Env: []corev1.EnvVar{
						{
							Name:  "KUBECONFIG",
							Value: "/etc/kubeconfig/kubeconfig",
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "kubeconfig",
							MountPath: "/etc/kubeconfig",
							ReadOnly:  true,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "kubeconfig",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: kubeconfigSecret,
						},
					},
				},
			},
		},
	}

	s.Require().NoError(s.k8sHelper.Clientset.Create(s.ctx, pod), "Failed to create connectivity test pod")

	defer func() {
		_ = s.k8sHelper.Clientset.Delete(s.ctx, pod)
	}()

	// Wait for pod to complete (with timeout)
	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var finalPhase corev1.PodPhase
	for {
		select {
		case <-timeout:
			s.Fail("Timeout waiting for connectivity test pod to complete")
			return
		case <-ticker.C:
			currentPod := &corev1.Pod{}
			if err := s.k8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(pod), currentPod); err != nil {
				continue
			}
			finalPhase = currentPod.Status.Phase
			if finalPhase == corev1.PodSucceeded || finalPhase == corev1.PodFailed {
				goto done
			}
		}
	}
done:

	s.Equal(corev1.PodSucceeded, finalPhase, "Connectivity test pod should succeed")

	// Get pod logs to verify we got nodes from target cluster
	logs, err := s.k8sHelper.Kubectl("logs", "-n", testNamespace, podName)
	s.Require().NoError(err, "Failed to get connectivity test pod logs")
	s.Contains(logs, fmt.Sprintf("node/k3d-%s-server-0", targetClusterName), "Should see target cluster node in output")

	zap.S().Infof("Connectivity test output: %s", logs)
	zap.S().Info("External cluster connectivity test completed successfully!")
}

func TestExternalClusterSuite(t *testing.T) {
	if os.Getenv("K8S_DISTRO") != "k3d" {
		t.Skip("External cluster test requires k3d (K8S_DISTRO=k3d)")
	}

	s := new(ExternalClusterSuite)
	defer func(s *ExternalClusterSuite) {
		HandlePanics(recover(), func() {
			s.cleanupTargetCluster()
		}, s.T)
	}(s)
	suite.Run(t, s)
}
