// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mondoov2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers/container_image"
	"go.mondoo.com/mondoo-operator/controllers/k8s_scan"
	"go.mondoo.com/mondoo-operator/controllers/nodes"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/logger"
	"go.mondoo.com/mondoo-operator/pkg/version"
	"go.mondoo.com/mondoo-operator/tests/framework/installer"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/assets"
	nexusK8s "go.mondoo.com/mondoo-operator/tests/framework/nexus/k8s"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type AuditConfigBaseSuite struct {
	suite.Suite
	ctx            context.Context
	spaceClient    *nexus.Space
	integration    *nexusK8s.Integration
	testCluster    *TestCluster
	auditConfig    mondoov2.MondooAuditConfig
	installRelease bool
}

func (s *AuditConfigBaseSuite) SetupSuite() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.SetLogger(logger.NewLogger())
	s.ctx = context.Background()

	nexusClient, err := nexus.NewClient()
	s.Require().NoError(err, "Failed to create Nexus client")
	s.spaceClient, err = nexusClient.CreateSpace()
	s.Require().NoError(err, "Failed to create Nexus space")
	log.Log.Info("Created Nexus space", "space", s.spaceClient.Mrn())

	// TODO: this is only needed because the integration creation is not part of the MondooInstaller struct.
	// That code will move there once all tests are migrated to use the E2E approach.
	k8sHelper, err := utils.CreateK8sHelper()
	s.Require().NoError(err, "Failed to create K8s helper")

	ns := &corev1.Namespace{}
	s.NoError(k8sHelper.Clientset.Get(s.ctx, client.ObjectKey{Name: "kube-system"}, ns))
	integration, err := s.spaceClient.K8s.CreateIntegration("test-integration-" + string(ns.UID)).
		EnableNodesScan().
		EnableWorkloadsScan().
		Run(s.ctx)
	s.Require().NoError(err, "Failed to create k8s integration")
	s.integration = integration

	settings := installer.NewDefaultSettings().SetToken(integration.Token())
	if s.installRelease {
		settings = installer.NewReleaseSettings().SetToken(integration.Token())
	}

	s.testCluster = StartTestCluster(s.ctx, settings, s.T)
}

func (s *AuditConfigBaseSuite) TearDownSuite() {
	s.NoError(s.testCluster.UninstallOperator())
	s.NoError(s.spaceClient.Delete(s.ctx))
}

func (s *AuditConfigBaseSuite) AfterTest(suiteName, testName string) {
	if s.testCluster != nil {
		s.testCluster.GatherAllMondooLogs(testName, installer.MondooNamespace)
		s.NoError(s.testCluster.CleanupAuditConfigs())

		operatorConfig := &mondoov2.MondooOperatorConfig{
			ObjectMeta: metav1.ObjectMeta{Name: mondoov2.MondooOperatorConfigName},
		}
		s.NoErrorf(s.testCluster.K8sHelper.DeleteResourceIfExists(operatorConfig), "Failed to delete MondooOperatorConfig")

		zap.S().Info("Waiting for cleanup of the test cluster.")
		// wait for resources to be gone
		k8sScanListOpts := &client.ListOptions{Namespace: s.auditConfig.Namespace, LabelSelector: labels.SelectorFromSet(k8s_scan.CronJobLabels(s.auditConfig))}
		err := s.testCluster.K8sHelper.EnsureNoPodsPresent(k8sScanListOpts)
		s.NoErrorf(err, "Failed to wait for k8s scan pods to be gone")

		containerScanListOpts := &client.ListOptions{Namespace: s.auditConfig.Namespace, LabelSelector: labels.SelectorFromSet(container_image.CronJobLabels(s.auditConfig))}
		err = s.testCluster.K8sHelper.EnsureNoPodsPresent(containerScanListOpts)
		s.NoErrorf(err, "Failed to wait for container scan pods to be gone")

		nodeScanListOpts := &client.ListOptions{Namespace: s.auditConfig.Namespace, LabelSelector: labels.SelectorFromSet(nodes.NodeScanningLabels(s.auditConfig))}
		err = s.testCluster.K8sHelper.EnsureNoPodsPresent(nodeScanListOpts)
		s.NoErrorf(err, "Failed to wait for node scan pods to be gone")

		zap.S().Info("Cleanup done. Cluster should be good to go for the next test.")

		s.Require().NoError(s.spaceClient.DeleteAssets(s.ctx), "Failed to delete assets in space")
		s.Require().NoError(s.integration.DeleteCiCdProjectIfExists(s.ctx), "Failed to delete CICD project for integration")

		_, err = s.testCluster.K8sHelper.Kubectl("delete", "pods", "-n", "default", "--all", "--wait")
		s.Require().NoError(err, "Failed to delete all pods")
	}
}

func (s *AuditConfigBaseSuite) testMondooAuditConfigKubernetesResources(auditConfig mondoov2.MondooAuditConfig) {
	s.auditConfig = auditConfig

	// Disable container image resolution to be able to run the k8s resources scan CronJob with a local image.
	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	zap.S().Info("Create an audit config that enables only workloads scanning.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	s.Require().True(s.testCluster.K8sHelper.WaitUntilMondooClientSecretExists(s.ctx, s.auditConfig.Namespace), "Mondoo SA not created")

	// K8s scan
	zap.S().Info("Make sure the Mondoo k8s resources scan CronJob is created.")
	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: k8s_scan.CronJobName(auditConfig.Name), Namespace: auditConfig.Namespace},
	}
	err := s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		if err := s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(cronJob), cronJob); err != nil {
			return false, nil
		}
		return true, nil
	})
	s.NoError(err, "Kubernetes resources scanning CronJob was not created.")

	time.Sleep(20 * time.Second)

	cronJobLabels := k8s_scan.CronJobLabels(auditConfig)
	s.True(
		s.testCluster.K8sHelper.WaitUntilCronJobsSuccessful(utils.LabelsToLabelSelector(cronJobLabels), auditConfig.Namespace),
		"Kubernetes resources scan CronJob did not run successfully.")

	err = s.testCluster.K8sHelper.CheckForPodInStatus(&auditConfig, "client-k8s-scan")
	s.NoErrorf(err, "Couldn't find k8s scan pod in Podlist of the MondooAuditConfig Status")

	err = s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&auditConfig, version.Version)
	s.NoErrorf(err, "Couldn't find expected version in MondooAuditConfig.Status.ReconciledByOperatorVersion")

	// Verify the workloads have been sent upstream and have scores.
	workloadNames, err := s.testCluster.K8sHelper.GetWorkloadNames(s.ctx)
	s.NoError(err, "Failed to get workload names.")
	zap.S().Info("number of workload", " amount ", len(workloadNames))

	currentCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: k8s_scan.CronJobName(auditConfig.Name), Namespace: auditConfig.Namespace},
	}
	err = s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		if err := s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(currentCronJob), currentCronJob); err != nil {
			return false, nil
		}
		return true, nil
	})
	s.NoError(err, "Kubernetes resources scanning CronJob was not found.")
	s.Equal(cronJob.Spec.Schedule, currentCronJob.Spec.Schedule, "CronJob schedule was changed.")

	foundMondooAuditConfig, err := s.testCluster.K8sHelper.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
	s.NoError(err, "Failed to find MondooAuditConfig")
	s.Equal(cronJob.Spec.Schedule, foundMondooAuditConfig.Spec.KubernetesResources.Schedule, "CronJob schedule was not updated in MondooAuditConfig")

	// The number of assets from upstream is limited by paganiation.
	// In case we have more than 100 workloads, we need to call this mutlple times, with different page numbers.
	assets, err := s.spaceClient.ListAssetsWithScores(s.ctx)
	s.NoError(err, "Failed to list assets with scores.")
	zap.S().Info("number of assets from upstream: ", len(assets))

	// TODO: the cluster name is non-deterministic currently so we cannot test for it
	nonDetermenisticAssets := utils.ExcludeNonDetermenisticAssets(assets)

	// TODO: this number should exclude services and the cluster asset
	srvs := &corev1.ServiceList{}
	err = s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		if err := s.testCluster.K8sHelper.Clientset.List(s.ctx, srvs); err != nil {
			return false, nil
		}
		return true, nil
	})
	s.NoError(err, "Failed to list Kubernetes Services")
	s.Equalf(len(assets)-1-len(srvs.Items), len(nonDetermenisticAssets), "Cluster and/or Services assets were sent upstream.")

	assetNames := utils.AssetNames(nonDetermenisticAssets)
	s.ElementsMatchf(workloadNames, assetNames, "Workloads were not sent upstream.")

	s.AssetsNotUnscored(assets)

	status, err := s.integration.GetStatus(s.ctx)
	s.NoError(err, "Failed to get status")
	s.Equal("ACTIVE", status)
}

func (s *AuditConfigBaseSuite) testMondooAuditConfigContainers(auditConfig mondoov2.MondooAuditConfig) {
	nginxLabel := "app.kubernetes.io/name=nginx"
	_, err := s.testCluster.K8sHelper.Kubectl("run", "-n", "default", "nginx", "--image", "ghcr.io/nginx/nginx-unprivileged", "-l", nginxLabel)
	s.Require().NoError(err, "Failed to create nginx pod.")
	redisLabel := "app.kubernetes.io/name=redis"
	_, err = s.testCluster.K8sHelper.Kubectl("run", "-n", "default", "redis", "--image", "quay.io/opstree/redis", "-l", redisLabel)
	s.Require().NoError(err, "Failed to create redis pod.")

	s.True(s.testCluster.K8sHelper.IsPodReady(nginxLabel, "default"), "nginx pod is not ready")
	s.True(s.testCluster.K8sHelper.IsPodReady(redisLabel, "default"), "redis pod is not ready")
	s.auditConfig = auditConfig

	// Disable container image resolution to be able to run the k8s resources scan CronJob with a local image.
	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	zap.S().Info("Create an audit config that enables only workloads scanning.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	// Get the available container images at the time the cronjob is created.
	pods := &corev1.PodList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, pods), "Failed to list pods")

	// K8s container image scan
	zap.S().Info("Make sure the Mondoo k8s container image scan CronJob is created.")
	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: container_image.CronJobName(auditConfig.Name), Namespace: auditConfig.Namespace},
	}
	err = s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		if err := s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(cronJob), cronJob); err != nil {
			return false, nil
		}
		return true, nil
	})
	s.NoError(err, "Kubernetes container image scanning CronJob was not created.")

	cronJobLabels := container_image.CronJobLabels(auditConfig)
	s.True(
		s.testCluster.K8sHelper.WaitUntilCronJobsSuccessful(utils.LabelsToLabelSelector(cronJobLabels), auditConfig.Namespace),
		"Kubernetes container image scan CronJob did not run successfully.")

	err = s.testCluster.K8sHelper.CheckForPodInStatus(&auditConfig, "client-containers-scan")
	s.NoErrorf(err, "Couldn't find container image scan pod in Podlist of the MondooAuditConfig Status")

	err = s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&auditConfig, version.Version)
	s.NoErrorf(err, "Couldn't find expected version in MondooAuditConfig.Status.ReconciledByOperatorVersion")

	containerImages, err := utils.ContainerImages(pods.Items, auditConfig)
	s.NoError(err, "Failed to get container image names")

	currentCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: container_image.CronJobName(auditConfig.Name), Namespace: auditConfig.Namespace},
	}
	err = s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		if err := s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(currentCronJob), currentCronJob); err != nil {
			return false, nil
		}
		return true, nil
	})
	s.NoError(err, "Kubernetes container scanning CronJob was not found.")
	s.Equal(cronJob.Spec.Schedule, currentCronJob.Spec.Schedule, "CronJob schedule was changed.")

	foundMondooAuditConfig, err := s.testCluster.K8sHelper.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
	s.NoError(err, "Failed to find MondooAuditConfig")
	s.Equal(cronJob.Spec.Schedule, foundMondooAuditConfig.Spec.Containers.Schedule, "CronJob schedule was not updated in MondooAuditConfig")

	// Verify the container images have been sent upstream and have scores.
	// The number of assets from upstream is limited by paganiation.
	// In case we have more than 100 workloads, we need to call this mutlple times, with different page numbers.
	assets, err := s.spaceClient.ListAssetsWithScores(s.ctx)
	s.NoError(err, "Failed to list assets with scores")

	assetNames := utils.AssetNames(assets)
	s.Subset(assetNames, containerImages, "Container images were not sent upstream.")

	s.AssetsNotUnscored(assets)

	status, err := s.integration.GetStatus(s.ctx)
	s.NoError(err, "Failed to get status")
	s.Equal("ACTIVE", status)
}

func (s *AuditConfigBaseSuite) testMondooAuditConfigNodesCronjobs(auditConfig mondoov2.MondooAuditConfig) {
	s.auditConfig = auditConfig

	// Disable container image resolution to be able to run the k8s resources scan CronJob with a local image.
	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	zap.S().Info("Create an audit config that enables only nodes scanning.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	s.Require().True(s.testCluster.K8sHelper.WaitUntilMondooClientSecretExists(s.ctx, s.auditConfig.Namespace), "Mondoo SA not created")

	zap.S().Info("Verify the nodes scanning cron jobs are created.")

	cronJobs := &batchv1.CronJobList{}
	cronJobLabels := nodes.NodeScanningLabels(auditConfig)

	// List only the CronJobs in the namespace of the MondooAuditConfig and only the ones that exactly match our labels.
	listOpts := &client.ListOptions{Namespace: auditConfig.Namespace, LabelSelector: labels.SelectorFromSet(cronJobLabels)}

	nodeList := &corev1.NodeList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, nodeList))

	// Verify the amount of CronJobs created is equal to the amount of nodes
	err := s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, cronJobs, listOpts))
		if len(nodeList.Items) == len(cronJobs.Items) {
			return true, nil
		}
		return false, nil
	})
	s.NoErrorf(
		err,
		"The amount of node scanning CronJobs is not equal to the amount of cluster nodes. expected: %d; actual: %d",
		len(nodeList.Items), len(cronJobs.Items))

	for _, c := range cronJobs.Items {
		found := false
		for _, n := range nodeList.Items {
			if n.Name == c.Spec.JobTemplate.Spec.Template.Spec.NodeName {
				found = true
			}
		}
		s.Truef(found, "CronJob %s/%s does not have a corresponding cluster node.", c.Namespace, c.Name)
	}

	// Make sure we have 1 successful run for each CronJob
	selector := utils.LabelsToLabelSelector(cronJobLabels)
	s.True(s.testCluster.K8sHelper.WaitUntilCronJobsSuccessful(selector, auditConfig.Namespace), "Not all CronJobs have run successfully.")

	base := fmt.Sprintf("%s%s", auditConfig.Name, nodes.CronJobNameBase)
	for _, node := range nodeList.Items {
		nodeIdentifier := nodes.NodeNameOrHash(k8s.ResourceNameMaxLength-len(base), node.Name)
		err := s.testCluster.K8sHelper.CheckForPodInStatus(&auditConfig, "client-node-"+nodeIdentifier)
		s.NoErrorf(err, "Couldn't find NodeScan Pod for node "+node.Name+" in Podlist of the MondooAuditConfig Status")
	}

	err = s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&auditConfig, version.Version)
	s.NoErrorf(err, "Couldn't find expected version in MondooAuditConfig.Status.ReconciledByOperatorVersion")

	// Verify nodes are sent upstream and have scores.
	nodes := &corev1.NodeList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, nodes))

	nodeNames := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}

	currentCronJobs := &batchv1.CronJobList{}
	err = s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, currentCronJobs, listOpts))
		if len(nodeList.Items) == len(currentCronJobs.Items) {
			return true, nil
		}
		return false, nil
	})
	s.NoError(err, "Kubernetes node scanning CronJob was not found.")
	s.Equal(cronJobs.Items[0].Spec.Schedule, currentCronJobs.Items[0].Spec.Schedule, "CronJob schedule was changed.")

	foundMondooAuditConfig, err := s.testCluster.K8sHelper.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
	s.NoError(err, "Failed to find MondooAuditConfig")
	s.Equal(cronJobs.Items[0].Spec.Schedule, foundMondooAuditConfig.Spec.Nodes.Schedule, "CronJob schedule was not updated in MondooAuditConfig")

	// The number of assets from upstream is limited by paganiation.
	// In case we have more than 100 workloads, we need to call this mutlple times, with different page numbers.
	assets, err := s.spaceClient.ListAssetsWithScores(s.ctx)
	s.NoError(err, "Failed to list assets")
	assetNames := utils.AssetNames(assets)

	s.ElementsMatch(assetNames, nodeNames, "Node names do not match")
	s.AssetsNotUnscored(assets)

	status, err := s.integration.GetStatus(s.ctx)
	s.NoError(err, "Failed to get status")
	s.Equal("ACTIVE", status)
}

func (s *AuditConfigBaseSuite) testMondooAuditConfigAllDisabled(auditConfig mondoov2.MondooAuditConfig) {
	s.auditConfig = auditConfig
	// Disable image resolution so locally-built container images can be used.
	// Otherwise, mondoo-operator will try to resolve the locally-built container
	// images, and fail because they haven't been pushed publicly.
	operatorConfig := &mondoov2.MondooOperatorConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: mondoov2.MondooOperatorConfigName,
		},
		Spec: mondoov2.MondooOperatorConfigSpec{
			SkipContainerResolution: true,
		},
	}
	s.Require().NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, operatorConfig), "Failed to create MondooOperatorConfig")

	// Enable nothing
	zap.S().Info("Create an audit config that enables nothing.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &s.auditConfig),
		"Failed to create Mondoo audit config.")

	err := s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&s.auditConfig, version.Version)
	s.NoErrorf(err, "Couldn't find expected version in MondooAuditConfig.Status.ReconciledByOperatorVersion")

	status, err := s.integration.GetStatus(s.ctx)
	s.NoError(err, "Failed to get status")
	s.Equal("ACTIVE", status)
}

// disableContainerImageResolution Creates a MondooOperatorConfig that disables container image resolution. This is needed
// in order to be able to execute the integration tests with local images. A function is returned that will cleanup the
// operator config that was created. It is advised to call it with defer such that the operator config is always deleted
// regardless of the test outcome.
func (s *AuditConfigBaseSuite) disableContainerImageResolution() func() {
	operatorConfig := &mondoov2.MondooOperatorConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: mondoov2.MondooOperatorConfigName,
		},
		Spec: mondoov2.MondooOperatorConfigSpec{
			SkipContainerResolution: true,
		},
	}
	s.Require().NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, operatorConfig), "Failed to create MondooOperatorConfig")

	return func() {
		// Bring back the default image resolution behavior
		s.NoErrorf(
			s.testCluster.K8sHelper.Clientset.Delete(s.ctx, operatorConfig),
			"Failed to restore container resolution in MondooOperatorConfig")
	}
}

var (
	defaultK8sPolicyMrns = []string{
		"//policy.api.mondoo.app/policies/mondoo-kubernetes-best-practices",
		"//policy.api.mondoo.app/policies/mondoo-kubernetes-security",
	}
	defaultK8sNodePolicyMrns = []string{
		"//policy.api.mondoo.app/policies/mondoo-kubernetes-security",
		"//policy.api.mondoo.app/policies/mondoo-linux-security",
	}
	defaultOsPolicyMrns = []string{
		"//policy.api.mondoo.app/policies/mondoo-linux-security",
	}
)

func (s *AuditConfigBaseSuite) AssetsNotUnscored(assets []assets.AssetWithScore) {
	for _, asset := range assets {
		// We don't score scratch containers at the moment so they are always unscored.
		// We don't have policies for a cluster asset enabled at the moment so they are always unscored.
		if asset.Platform.Name != "scratch" && asset.Platform.Name != "k8s-cluster" && asset.Platform.Name != "k8s-namespace" && asset.Platform.Name != "k8s-service" {
			if asset.Grade == "U" || asset.Grade == "" {
				zap.S().Infof("Asset %s has no score", asset.Name)
			}
			s.NotEqualf("U", asset.Grade, "Asset %s should not be unscored", asset.Name)

			// Check which were the scored policies
			scoredPolicies := []string{}
			for _, p := range asset.PolicyScores {
				if p.Grade != "U" {
					scoredPolicies = append(scoredPolicies, p.Mrn)
				}
			}
			expectedPolicies := defaultK8sNodePolicyMrns
			if strings.Contains(asset.Platform.Name, "k8s") {
				expectedPolicies = defaultK8sPolicyMrns
			} else if strings.Contains(asset.Name, "nginx") || strings.Contains(asset.Name, "redis") || strings.Contains(asset.Name, "k3d") || asset.Platform.Runtime == "docker-image" {
				expectedPolicies = defaultOsPolicyMrns
			}
			s.ElementsMatchf(expectedPolicies, scoredPolicies, "Scored policies for asset %s should be the default k8s policies", asset.Name)
		}
	}
}

func (s *AuditConfigBaseSuite) CiCdJobNotUnscored(assets []nexusK8s.CiCdJob) {
	for _, asset := range assets {
		s.NotEqualf("U", asset.Grade, "CI/CD job %s should not be unscored", asset.Name)
	}
}
