// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	webhooksv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mondoov2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	mondooadmission "go.mondoo.com/mondoo-operator/controllers/admission"
	"go.mondoo.com/mondoo-operator/controllers/container_image"
	"go.mondoo.com/mondoo-operator/controllers/k8s_scan"
	"go.mondoo.com/mondoo-operator/controllers/nodes"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/version"
	"go.mondoo.com/mondoo-operator/tests/framework/installer"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/policy"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/assets"
	nexusK8s "go.mondoo.com/mondoo-operator/tests/framework/nexus/k8s"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	webhookLocalPort         = 18443
	maxRetriesWebhookConnect = 120
	maxRetriesProcessGone    = 5
	maxRetriesCreate         = 5
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
	s.ctx = context.Background()

	sa, err := utils.GetServiceAccount()
	s.Require().NoError(err, "Service account not set")
	nexusClient, err := nexus.NewClient(sa)

	s.Require().NoError(err, "Failed to create Nexus client")
	s.spaceClient = nexusClient.GetSpace()

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

	token, err := s.integration.GetRegistrationToken(s.ctx)
	s.Require().NoError(err, "Failed to get long lived integration token")

	settings := installer.NewDefaultSettings().SetToken(token)
	if s.installRelease {
		settings = installer.NewReleaseSettings().SetToken(token)
	}

	s.testCluster = StartTestCluster(s.ctx, settings, s.T)
}

func (s *AuditConfigBaseSuite) TearDownSuite() {
	s.NoError(s.testCluster.UninstallOperator())
	s.NoError(s.integration.Delete(s.ctx))
}

func (s *AuditConfigBaseSuite) AfterTest(suiteName, testName string) {
	if s.testCluster != nil {
		s.testCluster.GatherAllMondooLogs(testName, installer.MondooNamespace)
		s.NoError(s.testCluster.CleanupAuditConfigs())
		secret := &corev1.Secret{}
		secret.Name = mondooadmission.GetTLSCertificatesSecretName(s.auditConfig.Name)
		secret.Namespace = s.auditConfig.Namespace
		s.NoErrorf(s.testCluster.K8sHelper.DeleteResourceIfExists(secret), "Failed to delete TLS secret")

		operatorConfig := &mondoov2.MondooOperatorConfig{
			ObjectMeta: metav1.ObjectMeta{Name: mondoov2.MondooOperatorConfigName},
		}
		s.NoErrorf(s.testCluster.K8sHelper.DeleteResourceIfExists(operatorConfig), "Failed to delete MondooOperatorConfig")

		zap.S().Info("Waiting for cleanup of the test cluster.")
		// wait for deployments to be gone
		// sometimes the operator still terminates ,e.g. the webhook, while the next test already started
		// the new test then fails because resources vanish during the test
		scanApiListOpts := &client.ListOptions{Namespace: s.auditConfig.Namespace, LabelSelector: labels.SelectorFromSet(scanapi.DeploymentLabels(s.auditConfig))}
		err := s.testCluster.K8sHelper.EnsureNoPodsPresent(scanApiListOpts)
		s.NoErrorf(err, "Failed to wait for ScanAPI Pods to be gone")

		webhookLabels := mondooadmission.WebhookDeploymentLabels()
		webhookListOpts := &client.ListOptions{Namespace: s.auditConfig.Namespace, LabelSelector: labels.SelectorFromSet(webhookLabels)}
		err = s.testCluster.K8sHelper.EnsureNoPodsPresent(webhookListOpts)
		s.NoErrorf(err, "Failed to wait for Webhook Pods to be gone")

		// not sure why the above list does not work. It returns zero deployments. So, first a plain sleep to stabilize the test.
		zap.S().Info("Cleanup done. Cluster should be good to go for the next test.")

		s.Require().NoError(s.spaceClient.DeleteAssetsManagedBy(s.ctx, s.testCluster.ManagedBy()), "Failed to delete assets for integration")
		s.Require().NoError(s.integration.DeleteCiCdProjectIfExists(s.ctx), "Failed to delete CICD project for integration")
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

	// Verify scan API deployment and service
	s.validateScanApiDeployment(auditConfig)

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

	assets, err := s.spaceClient.ListAssetsWithScores(s.ctx, s.integration.Mrn(), "")
	s.NoError(err, "Failed to list assets with scores.")

	assetNames := utils.AssetNames(assets)
	s.ElementsMatch(workloadNames, assetNames, "Workloads were not sent upstream.")

	s.AssetsNotUnscored(assets)
}

func (s *AuditConfigBaseSuite) testMondooAuditConfigContainers(auditConfig mondoov2.MondooAuditConfig) {
	s.auditConfig = auditConfig

	// Disable container image resolution to be able to run the k8s resources scan CronJob with a local image.
	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	zap.S().Info("Create an audit config that enables only workloads scanning.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	// K8s container image scan
	zap.S().Info("Make sure the Mondoo k8s container image scan CronJob is created.")
	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: container_image.CronJobName(auditConfig.Name), Namespace: auditConfig.Namespace},
	}
	err := s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
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

	// Verify the container images have been sent upstream and have scores.
	pods := &corev1.PodList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, pods), "Failed to list pods")

	containerImages, err := utils.ContainerImages(pods.Items, auditConfig)
	s.NoError(err, "Failed to get container image names")

	assets, err := s.spaceClient.ListAssetsWithScores(s.ctx, s.integration.Mrn(), "container_image")
	s.NoError(err, "Failed to list assets with scores")

	assetNames := utils.AssetNames(assets)
	s.ElementsMatch(containerImages, assetNames, "Container images were not sent upstream.")

	s.AssetsNotUnscored(assets)
}

func (s *AuditConfigBaseSuite) testMondooAuditConfigNodes(auditConfig mondoov2.MondooAuditConfig) {
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
	cronJobLabels := nodes.CronJobLabels(auditConfig)

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

	// Verify the garbage collect cron job
	gcCronJobs := &batchv1.CronJobList{}
	gcCronJobLabels := nodes.GarbageCollectCronJobLabels(auditConfig)

	// List only the CronJobs in the namespace of the MondooAuditConfig and only the ones that exactly match our labels.
	gcListOpts := &client.ListOptions{Namespace: auditConfig.Namespace, LabelSelector: labels.SelectorFromSet(gcCronJobLabels)}

	// Verify the amount of CronJobs created is 1
	err = s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, gcCronJobs, gcListOpts))
		if 1 == len(cronJobs.Items) {
			return true, nil
		}
		return false, nil
	})
	s.NoErrorf(
		err,
		"The amount of garbage collect CronJobs is not 1 expected: 1; actual: %d", len(cronJobs.Items))

	err = s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&auditConfig, version.Version)
	s.NoErrorf(err, "Couldn't find expected version in MondooAuditConfig.Status.ReconciledByOperatorVersion")

	// Verify nodes are sent upstream and have scores.
	nodes := &corev1.NodeList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, nodes))

	nodeNames := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}

	assets, err := s.spaceClient.ListAssetsWithScores(s.ctx, s.integration.Mrn(), "")
	s.NoError(err, "Failed to list assets")
	assetNames := utils.AssetNames(assets)

	s.ElementsMatch(nodeNames, assetNames, "Node names do not match")
	s.AssetsNotUnscored(assets)
}

func (s *AuditConfigBaseSuite) testMondooAuditConfigAdmission(auditConfig mondoov2.MondooAuditConfig) {
	// Disable imageResolution for the webhook image to be runnable.
	// Otherwise, mondoo-operator will try to resolve the locally-built mondoo-operator container
	// image, and fail because we haven't pushed this image publicly.
	cleanup := s.disableContainerImageResolution()
	defer cleanup()
	s.verifyAdmissionWorking(auditConfig)
}

func (s *AuditConfigBaseSuite) verifyAdmissionWorking(auditConfig mondoov2.MondooAuditConfig) {
	s.auditConfig = auditConfig
	// Generate certificates manually
	caCert, err := s.manuallyCreateCertificates()

	// Don't bother with further webhook tests if we couldnt' save the certificates
	s.Require().NoErrorf(err, "Error while generating/saving certificates for webhook service")

	// Enable webhook
	zap.S().Info("Create an audit config that enables only admission control.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	s.Require().True(s.testCluster.K8sHelper.WaitUntilMondooClientSecretExists(s.ctx, s.auditConfig.Namespace), "Mondoo SA not created")

	// Wait for Ready Pod
	zap.S().Info("Waiting for webhook Pod to become ready.")
	webhookLabelsString := s.getWebhookLabelsString()
	s.Truef(
		s.testCluster.K8sHelper.IsPodReady(webhookLabelsString, auditConfig.Namespace),
		"Mondoo webhook Pod is not in a Ready state.")
	zap.S().Info("Webhook Pod is ready.")

	// Verify scan API deployment and service
	s.validateScanApiDeployment(auditConfig)

	// Check number of Pods depending on mode
	webhookListOpts, err := s.getWebhookListOps()
	s.NoError(err)
	pods := &corev1.PodList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, pods, webhookListOpts))
	numPods := 1
	if auditConfig.Spec.Admission.Mode == mondoov2.Enforcing {
		numPods = 2
	}
	if auditConfig.Spec.Admission.Replicas != nil {
		numPods = int(*auditConfig.Spec.Admission.Replicas)
	}
	failMessage := fmt.Sprintf("Pods count for webhook should be precisely %d because of mode and replicas", numPods)
	s.Equalf(numPods, len(pods.Items), failMessage)

	s.verifyWebhookAndStart(webhookListOpts, caCert)

	err = s.testCluster.K8sHelper.CheckForDegradedCondition(&auditConfig, mondoov2.AdmissionDegraded, corev1.ConditionFalse)
	s.NoErrorf(err, "Admission shouldn't be in degraded state")

	err = s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&auditConfig, version.Version)
	s.NoErrorf(err, "Couldn't find expected version in MondooAuditConfig.Status.ReconciledByOperatorVersion")

	zap.S().Info("Waiting for Webhook to accept connections (max 120s).")
	err = s.checkWebhookAvailability()
	s.NoErrorf(err, "Couldn't access Webhook via port-forward")
	zap.S().Info("Webhook should be working by now.")
	s.checkDeployments(&auditConfig)
}

func (s *AuditConfigBaseSuite) testMondooAuditConfigAdmissionScaleDownScanApi(auditConfig mondoov2.MondooAuditConfig) {
	// Disable imageResolution for the webhook image to be runnable.
	// Otherwise, mondoo-operator will try to resolve the locally-built mondoo-operator container
	// image, and fail because we haven't pushed this image publicly.
	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	// first verify admission is working
	s.verifyAdmissionWorking(auditConfig)

	// now check what happens when it is degraded
	listOpts, err := utils.LabelSelectorListOptions(utils.LabelsToLabelSelector(scanapi.DeploymentLabels(auditConfig)))
	s.NoError(err)
	listOpts.Namespace = auditConfig.Namespace

	podList := &corev1.PodList{}
	err = s.testCluster.K8sHelper.Clientset.List(s.ctx, podList, listOpts)
	s.NoErrorf(err, "Scan API Pod should be present")

	err = s.testCluster.K8sHelper.Clientset.Delete(s.ctx, &podList.Items[0], &client.DeleteOptions{})
	s.NoErrorf(err, "Scan API Pod could not be deleted")

	err = s.testCluster.K8sHelper.WaitForResourceDeletion(&podList.Items[0])
	s.NoErrorf(err, "Scan API Pod did not get deleted")

	zap.S().Info("MondooAuditConfig condition should be updated to degraded.")
	err = s.testCluster.K8sHelper.CheckForDegradedCondition(&auditConfig, mondoov2.AdmissionDegraded, corev1.ConditionTrue)
	s.NoErrorf(err, "Admission should be in degraded state")

	// try to change deployment => should fail
	deployments := &appsv1.DeploymentList{}
	webhookListOpts, err := s.getWebhookListOps()
	s.NoError(err)
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, webhookListOpts))

	s.Equalf(1, len(deployments.Items), "Deployments count for webhook should be precisely one")

	deployments.Items[0].Labels["testLabel"] = "testValue"

	s.Errorf(
		s.testCluster.K8sHelper.Clientset.Update(s.ctx, &deployments.Items[0]),
		"Expected failed updated of Deployment because Scan API is unreachable")
}

func (s *AuditConfigBaseSuite) testMondooAuditConfigAdmissionMissingSA(auditConfig mondoov2.MondooAuditConfig) {
	s.auditConfig = auditConfig
	// Disable imageResolution for the webhook image to be runnable.
	// Otherwise, mondoo-operator will try to resolve the locally-built mondoo-operator container
	// image, and fail because we haven't pushed this image publicly.
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

	// Enable webhook
	zap.S().Info("Create an audit config that enables only admission control.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	s.Require().True(s.testCluster.K8sHelper.WaitUntilMondooClientSecretExists(s.ctx, s.auditConfig.Namespace), "Mondoo SA not created")

	// Pod should not start, because of missing service account

	// do not wait until IsPodReady timeout, pod will not be present
	// something like eventually from ginko would be nice, first iteration just with a sleep.
	// just a grace period
	time.Sleep(10 * time.Second)
	listOpts, err := utils.LabelSelectorListOptions(utils.LabelsToLabelSelector(scanapi.DeploymentLabels(auditConfig)))
	s.NoError(err)
	listOpts.Namespace = auditConfig.Namespace
	podList := &corev1.PodList{}

	err = s.testCluster.K8sHelper.Clientset.List(s.ctx, podList, listOpts)
	s.NoErrorf(err, "Couldn't list scan API pod.")
	s.Equalf(0, len(podList.Items), "No ScanAPI Pod should be present")

	// Check for the ScanAPI Deployment to be present.
	deployments := &appsv1.DeploymentList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, listOpts))

	s.Equalf(1, len(deployments.Items), "Deployments count for ScanAPI should be precisely one")

	err = s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		// Condition of MondooAuditConfig should be updated
		foundMondooAuditConfig, err := s.testCluster.K8sHelper.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
		if err != nil {
			return false, err
		}
		condition, err := s.testCluster.K8sHelper.GetMondooAuditConfigConditionByType(foundMondooAuditConfig, mondoov2.ScanAPIDegraded)
		if err != nil {
			return false, err
		}
		if strings.Contains(condition.Message, "error looking up service account") {
			return true, nil
		}
		return false, nil
	})

	s.NoErrorf(err, "Couldn't find condition message about missing service account")

	// The SA is missing, but the actual reconcile loop gets finished. The SA is outside of the operators scope.
	err = s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&auditConfig, version.Version)
	s.NoErrorf(err, "Couldn't find expected version in MondooAuditConfig.Status.ReconciledByOperatorVersion")
}

func (s *AuditConfigBaseSuite) testMondooAuditConfigAllDisabled(auditConfig mondoov2.MondooAuditConfig) {
	s.auditConfig = auditConfig
	// Disable imageResolution for the webhook image to be runnable.
	// Otherwise, mondoo-operator will try to resolve the locally-built mondoo-operator container
	// image, and fail because we haven't pushed this image publicly.
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
}

func (s *AuditConfigBaseSuite) testUpgradePreviousReleaseToLatest(auditConfig mondoov2.MondooAuditConfig) {
	s.auditConfig = auditConfig

	serviceDNSNames := []string{
		// DNS names will take the form of ServiceName.ServiceNamespace.svc and .svc.cluster.local
		fmt.Sprintf("%s-webhook-service.%s.svc", auditConfig.Name, auditConfig.Namespace),
		fmt.Sprintf("%s-webhook-service.%s.svc.cluster.local", auditConfig.Name, auditConfig.Namespace),
	}
	secretName := mondooadmission.GetTLSCertificatesSecretName(auditConfig.Name)
	_, err := s.testCluster.MondooInstaller.GenerateServiceCerts(&auditConfig, secretName, serviceDNSNames)

	// Don't bother with further webhook tests if we couldnt' save the certificates
	s.Require().NoErrorf(err, "Error while generating/saving certificates for webhook service")

	// Disable imageResolution for the webhook image to be runnable.
	// Otherwise, mondoo-operator will try to resolve the locally-built mondoo-operator container
	// image, and fail because we haven't pushed this image publicly.
	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	s.Require().True(s.testCluster.K8sHelper.WaitUntilMondooClientSecretExists(s.ctx, s.auditConfig.Namespace), "Mondoo SA not created")

	// Verify scan API deployment and service
	s.validateScanApiDeployment(auditConfig)

	err = s.testCluster.K8sHelper.CheckForDegradedCondition(&auditConfig, mondoov2.AdmissionDegraded, corev1.ConditionFalse)
	s.Require().NoErrorf(err, "Admission shouldn't be in degraded state")

	err = s.testCluster.K8sHelper.CheckForDegradedCondition(&auditConfig, mondoov2.NodeScanningDegraded, corev1.ConditionFalse)
	s.Require().NoErrorf(err, "Node scanning shouldn't be in degraded state")

	err = s.testCluster.K8sHelper.CheckForDegradedCondition(&auditConfig, mondoov2.K8sResourcesScanningDegraded, corev1.ConditionFalse)
	s.Require().NoErrorf(err, "k8s resource scanning shouldn't be in degraded state")

	// everything is fine, now upgrade to current branch/release

	branchInstaller := installer.NewMondooInstaller(installer.NewDefaultSettings(), s.T)
	err = branchInstaller.InstallOperator()
	s.NoErrorf(err, "Failed updating the latest operator release to this branch")

	s.validateScanApiDeployment(auditConfig)

	err = s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&auditConfig, version.Version)
	s.NoErrorf(err, "Couldn't find release version in MondooAuditConfig.Status.ReconciledByOperatorVersion")
}

func (s *AuditConfigBaseSuite) validateScanApiDeployment(auditConfig mondoov2.MondooAuditConfig) {
	scanApiLabelsString := utils.LabelsToLabelSelector(scanapi.DeploymentLabels(auditConfig))
	zap.S().Info("Waiting for scan API Pod to become ready.")
	s.Truef(
		s.testCluster.K8sHelper.IsPodReady(scanApiLabelsString, auditConfig.Namespace),
		"Mondoo scan API Pod is not in a Ready state.")
	zap.S().Info("Scan API Pod is ready.")

	scanApiService := scanapi.ScanApiService(auditConfig.Namespace, auditConfig)
	err := s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		err := s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(scanApiService), scanApiService)
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	s.NoErrorf(err, "Failed to get scan API service.")

	expectedService := scanapi.ScanApiService(auditConfig.Namespace, auditConfig)
	s.NoError(ctrl.SetControllerReference(&auditConfig, expectedService, s.testCluster.K8sHelper.Clientset.Scheme()))
	s.Truef(k8s.AreServicesEqual(*expectedService, *scanApiService), "Scan API service is not as expected.")

	// might take some time because of reconcile loop
	zap.S().Info("Waiting for good condition of Scan API")
	err = s.testCluster.K8sHelper.WaitForGoodCondition(&auditConfig, mondoov2.ScanAPIDegraded)
	s.NoErrorf(err, "ScanAPI shouldn't be in degraded state")

	err = s.testCluster.K8sHelper.CheckForPodInStatus(&auditConfig, "client-scan-api")
	s.NoErrorf(err, "Couldn't find ScanAPI in Podlist of the MondooAuditConfig Status")
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

func (s *AuditConfigBaseSuite) getPassingDeployment() *appsv1.Deployment {
	labels := map[string]string{
		"admission-result": "pass",
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "passing-deployment",
			Namespace: "mondoo-operator",
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: ptr.To(false),
					Containers: []corev1.Container{
						{
							Name:            "ubuntu",
							Image:           "ubuntu:20.04",
							Command:         []string{"/bin/sh", "-c"},
							Args:            []string{"exit 0"},
							ImagePullPolicy: corev1.PullAlways,
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("100Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("100Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"NET_RAW"},
								},
								RunAsNonRoot:             ptr.To(true),
								RunAsUser:                ptr.To(int64(1000)),
								ReadOnlyRootFilesystem:   ptr.To(true),
								AllowPrivilegeEscalation: ptr.To(false),
								Privileged:               ptr.To(false),
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "exit 0"},
									},
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "exit 0"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (s *AuditConfigBaseSuite) getFailingDeployment() *appsv1.Deployment {
	labels := map[string]string{
		"admission-result": "fail",
	}
	deployment := s.getPassingDeployment().DeepCopy()
	deployment.ObjectMeta.Name = "failing-deployment"
	deployment.ObjectMeta.Labels = labels
	deployment.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
		Privileged: ptr.To(true),
	}
	return deployment
}

func (s *AuditConfigBaseSuite) checkDeployments(auditConfig *mondoov2.MondooAuditConfig) {
	passingDeployment := s.getPassingDeployment()
	failingDeployment := s.getFailingDeployment()

	// retry because sometimes we see these errors, although all previous checks reached the endpoint:
	//   Internal error occurred: failed calling webhook "policy.k8s.mondoo.com":
	//   failed to call webhook:
	//   Post "https://mondoo-client-webhook-service.mondoo-operator.svc:443/validate-k8s-mondoo-com?timeout=10s": EOF
	zap.S().Infof("Create a Deployment which should pass. (max. %d retries)", maxRetriesCreate)
	var err error
	for i := 0; i < maxRetriesCreate; i++ {
		err = s.testCluster.K8sHelper.Clientset.Create(s.ctx, passingDeployment)
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	s.NoErrorf(err, "Failed to create Deployment which should pass.")

	// TODO: validate passing deployment in nexus
	cicdProject, err := s.integration.GetCiCdProject(s.ctx)
	s.Require().NoError(err, "Failed to get CICD project")

	assets, err := cicdProject.ListAssets(s.ctx, "")
	s.Require().NoError(err, "Failed to list CICD assets")

	assetNames := utils.AssetNames(assets)
	s.Contains(assetNames, fmt.Sprintf("%s/%s", passingDeployment.Namespace, passingDeployment.Name))
	s.AssetsNotUnscored(assets)

	zap.S().Info("Create a Deployment which should be denied in enforcing mode.")
	err = s.testCluster.K8sHelper.Clientset.Create(s.ctx, failingDeployment)

	if auditConfig.Spec.Admission.Mode == mondoov2.Enforcing {
		s.Errorf(err, "Created Deployment which should have been denied.")
	} else {
		s.NoErrorf(err, "Failed creating a Deployment in permissive mode.")
	}

	assets, err = cicdProject.ListAssets(s.ctx, "")
	s.Require().NoError(err, "Failed to list CICD assets")

	assetNames = utils.AssetNames(assets)
	s.Contains(assetNames, fmt.Sprintf("%s/%s", failingDeployment.Namespace, failingDeployment.Name))
	s.AssetsNotUnscored(assets)

	s.NoErrorf(s.testCluster.K8sHelper.DeleteResourceIfExists(passingDeployment), "Failed to delete passingDeployment")
	s.NoErrorf(s.testCluster.K8sHelper.DeleteResourceIfExists(failingDeployment), "Failed to delete failingDeployment")
	s.NoErrorf(s.testCluster.K8sHelper.WaitForResourceDeletion(passingDeployment), "Error waiting for deleteion of passingDeployment")
	s.NoErrorf(s.testCluster.K8sHelper.WaitForResourceDeletion(failingDeployment), "Error waiting for deleteion of failingDeployment")
}

func (s *AuditConfigBaseSuite) getWebhookLabelsString() string {
	webhookDeploymentLabels := mondooadmission.WebhookDeploymentLabels()

	keyValuesWithEquals := []string{}
	for key, val := range webhookDeploymentLabels {
		keyValuesWithEquals = append(keyValuesWithEquals, key+"="+val)
	}
	webhookLabelsString := strings.Join(keyValuesWithEquals, ",")
	return webhookLabelsString
}

func (s *AuditConfigBaseSuite) getWebhookListOps() (*client.ListOptions, error) {
	webhookListOpts, err := utils.LabelSelectorListOptions(s.getWebhookLabelsString())
	if err != nil {
		return webhookListOpts, err
	}
	webhookListOpts.Namespace = s.auditConfig.Namespace
	return webhookListOpts, nil
}

func (s *AuditConfigBaseSuite) manuallyCreateCertificates() (*bytes.Buffer, error) {
	serviceDNSNames := []string{
		// DNS names will take the form of ServiceName.ServiceNamespace.svc and .svc.cluster.local
		fmt.Sprintf("%s-webhook-service.%s.svc", s.auditConfig.Name, s.auditConfig.Namespace),
		fmt.Sprintf("%s-webhook-service.%s.svc.cluster.local", s.auditConfig.Name, s.auditConfig.Namespace),
	}
	secretName := mondooadmission.GetTLSCertificatesSecretName(s.auditConfig.Name)
	return s.testCluster.MondooInstaller.GenerateServiceCerts(&s.auditConfig, secretName, serviceDNSNames)
}

// verifyWebhookAndStart Checks the ValidatingWebhookConfiguration, adds the CA data and waits for webhook to start working
func (s *AuditConfigBaseSuite) verifyWebhookAndStart(webhookListOpts *client.ListOptions, caCert *bytes.Buffer) {
	vwc := &webhooksv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-mondoo", s.auditConfig.Namespace, s.auditConfig.Name),
		},
	}
	s.NoErrorf(s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		if err := s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(vwc), vwc); err == nil {
			return true, nil
		}
		return false, nil
	}), "Failed to retrieve ValidatingWebhookConfiguration")

	if s.auditConfig.Spec.Admission.Mode == mondoov2.Enforcing {
		s.Equalf(*vwc.Webhooks[0].FailurePolicy, webhooksv1.Fail, "Webhook failurePolicy should be 'Fail' because of enforcing mode")
	} else {
		s.Equalf(*vwc.Webhooks[0].FailurePolicy, webhooksv1.Ignore, "Webhook failurePolicy should be 'Ignore' because of permissive mode")
	}

	if *vwc.Webhooks[0].FailurePolicy == webhooksv1.Fail {

		deployments := &appsv1.DeploymentList{}
		s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, webhookListOpts))

		s.Equalf(1, len(deployments.Items), "Deployments count for webhook should be precisely one")

		deployments.Items[0].Labels["testLabel"] = "testValue"

		s.Errorf(
			s.testCluster.K8sHelper.Clientset.Update(s.ctx, &deployments.Items[0]),
			"Expected failed updated of Deployment because certificate setup is incomplete")

	}

	for i := range vwc.Webhooks {
		vwc.Webhooks[i].ClientConfig.CABundle = caCert.Bytes()
	}

	// Set the failure policy to ignore always so we can ensure that we can restart the webhook. After the webhook has restarted,
	// we can rollback to the original value. This workaround is needed to enforce that the webhook has successfully reloaded the
	// CA secret which we set after it is created. If we do not force it to restart, there is no reliable way of knowing when it
	// has the correct CA data mounted and the tests become flaky.
	currentFailurePolicy := *vwc.Webhooks[0].FailurePolicy
	*vwc.Webhooks[0].FailurePolicy = webhooksv1.Ignore

	zap.S().Info("Update the webhook with the CA data.")
	s.NoErrorf(s.testCluster.K8sHelper.Clientset.Update(s.ctx, vwc), "Failed to add CA data to Webhook")

	// Restart the scan API pods to ensure the cert secret is reloaded.
	webhookLabels := mondooadmission.WebhookDeploymentLabels()

	webhookPods := &corev1.PodList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, webhookPods, webhookListOpts), "Failed to list webhook pods")

	zap.S().Info("Restart the webhook pods such that it is certain the CA secret has been reloaded.")
	for _, p := range webhookPods.Items {
		s.NoError(s.testCluster.K8sHelper.Clientset.Delete(s.ctx, &p), "Failed to delete webhook pod")
	}

	time.Sleep(2 * time.Second)

	s.Truef(
		s.testCluster.K8sHelper.IsPodReady(utils.LabelsToLabelSelector(webhookLabels), s.auditConfig.Namespace),
		"Mondoo webhook Pod is not in a Ready state.")
	zap.S().Info("Webhook Pod is ready.")

	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(vwc), vwc),
		"Failed to retrieve ValidatingWebhookConfiguration")
	*vwc.Webhooks[0].FailurePolicy = currentFailurePolicy
	s.NoErrorf(s.testCluster.K8sHelper.Clientset.Update(s.ctx, vwc), "Failed to add CA data to Webhook")

	// Sometime the Pod restart takes longer than 1 second, so we wait for the endpoints to be ready
	// when accessing the endpoints later on, we came across such errors:
	// ... pod mondoo-client-webhook-manager-6c5ccc449d-d7zn9 and container webhook. Container is in state ContainerCreating
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-webhook-service", s.auditConfig.Name),
			Namespace: s.auditConfig.Namespace,
		},
		Subsets: []corev1.EndpointSubset{},
	}
	zap.S().Info("Getting endpoints for webhook.")
	for i := 0; i < maxRetriesCreate; i++ {
		s.NoErrorf(
			s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(endpoints), endpoints),
			"Failed to retrieve endpoints for webhook")
		if len(endpoints.Subsets) > 0 {
			zap.S().Info("endpoints Addresses: ", endpoints.Subsets[0].Addresses)
			zap.S().Info("endpoints NotReadyAddresses: ", endpoints.Subsets[0].NotReadyAddresses)
			break
		}
		zap.S().Debug("Endpoints for webhook are not ready yet. Retrying...")
		time.Sleep(2 * time.Second)
	}

	zap.S().Info("Wait for webhook to start working.")
}

func (s *AuditConfigBaseSuite) checkWebhookAvailability() error {
	webhookService := mondooadmission.WebhookService(s.auditConfig.Namespace, s.auditConfig)
	// there is this package https://pkg.go.dev/k8s.io/client-go/tools/portforward
	// But it seems this is a bit complicated in combination with minikube
	// because of that we use kubectl directly
	cmd := s.createPortForwardCmd(webhookService)
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("couldn't start port-forward: %w", err)
	}
	// kubectl port-forwarding does not return but will run until interrupted
	// We have to get rid of the port-forward at the end, because we need to create a new one for each test
	defer func() {
		err := s.stopCmd(cmd)
		if err != nil {
			zap.S().Errorf("couldn't stop port-forward: %v\n", err)
		}
	}()
	zap.S().Info("Created port-forward via kubectl for webhook with pid: ", cmd.Process.Pid)

	webhookUrl := fmt.Sprintf("https://127.0.0.1:%d/validate-k8s-mondoo-com", webhookLocalPort)
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	client := &http.Client{Transport: customTransport}
	client.Timeout = 500 * time.Millisecond
	var resp *http.Response
	var webhookErr error
	for i := 0; i < maxRetriesWebhookConnect; i++ {
		time.Sleep(2 * time.Second)
		resp, webhookErr = client.Post(webhookUrl, "application/json", strings.NewReader("{}"))
		if webhookErr == nil {
			zap.S().Infof("Webhook is available: %s", resp.Status)
			resp.Body.Close()
			return nil
		} else {
			zap.S().Debug("Webhook is not available yet: ", webhookErr)
			// this error will not recover itself over time, we have to re-connect
			if strings.HasSuffix(webhookErr.Error(), "connection refused") {
				zap.S().Debug("Trying to restart port-forward")
				err := s.stopCmd(cmd)
				if err != nil {
					zap.S().Errorf("couldn't stop port-forward: %v\n", err)
					continue
				}
				cmd = s.createPortForwardCmd(webhookService)
				err = cmd.Start()
				if err != nil {
					zap.S().Errorf("couldn't start port-forward: %v\n", err)
				} else {
					zap.S().Info("Created port-forward via kubectl for webhook with pid: ", cmd.Process.Pid)
				}
			}
		}
	}
	return fmt.Errorf("webhook not available: %w", webhookErr)
}

func (s *AuditConfigBaseSuite) stopCmd(cmd *exec.Cmd) error {
	zap.S().Debug("Trying to stop port-forward with pid: ", cmd.Process.Pid)
	err := cmd.Process.Kill()
	if err != nil {
		return fmt.Errorf("couldn't kill kubectl port-forward: %w", err)
	}
	_, err = cmd.Process.Wait()
	return err
}

func (s *AuditConfigBaseSuite) createPortForwardCmd(webhookService *corev1.Service) *exec.Cmd {
	kubectlArgs := []string{
		"-n",
		webhookService.Namespace,
		"port-forward",
		"svc/" + webhookService.Name,
		fmt.Sprintf("%d:%d", webhookLocalPort, webhookService.Spec.Ports[0].Port),
	}

	return exec.Command("kubectl", kubectlArgs...)
}

func (s *AuditConfigBaseSuite) AssetsNotUnscored(assets []assets.AssetWithScore) {
	for _, asset := range assets {
		// We don't score scratch containers at the moment so they are always unscored.
		if asset.Asset.PlatformName != "scratch" {
			s.NotEqualf(uint32(policy.ScoreType_UNSCORED), asset.Score.Type, "Asset %s should not be unscored", asset.Asset.Name)
		}
	}
}
