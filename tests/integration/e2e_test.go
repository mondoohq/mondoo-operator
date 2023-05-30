package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	mondoov2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	mondooadmission "go.mondoo.com/mondoo-operator/controllers/admission"
	"go.mondoo.com/mondoo-operator/controllers/nodes"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/version"
	"go.mondoo.com/mondoo-operator/tests/framework/installer"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/policy"
	nexusK8s "go.mondoo.com/mondoo-operator/tests/framework/nexus/k8s"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type E2eTestSuite struct {
	suite.Suite
	ctx         context.Context
	spaceClient *nexus.Space
	integration *nexusK8s.Integration
	token       string
	testCluster *TestCluster

	auditConfig mondoov2.MondooAuditConfig
}

func (s *E2eTestSuite) SetupSuite() {
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
	s.token = token

	settings := installer.NewDefaultSettings().SetToken(token)

	s.testCluster = StartTestCluster(s.ctx, settings, s.T)
}

func (s *E2eTestSuite) TearDownSuite() {
	s.NoError(s.testCluster.UninstallOperator())
	s.NoError(s.integration.Delete(s.ctx))
}

func (s *E2eTestSuite) AfterTest(suiteName, testName string) {
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
	}
}

func (s *E2eTestSuite) TestE2e_NodeScan() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, true, false, true)

	s.testMondooAuditConfigNodes(auditConfig)

	nodes := &corev1.NodeList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, nodes))

	nodeNames := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}

	assets, err := s.spaceClient.ListAssetsWithScores(s.ctx, s.integration.Mrn())
	s.NoError(err, "Failed to list assets")
	assetNames := make([]string, 0, len(assets))
	for _, asset := range assets {
		assetNames = append(assetNames, asset.Asset.Name)
	}

	s.ElementsMatch(nodeNames, assetNames, "Node names do not match")
	for _, asset := range assets {
		s.Equal(uint32(policy.ScoreType_RESULT), asset.Score.Type, "Assets should be scored")
	}
}

func (s *E2eTestSuite) testMondooAuditConfigNodes(auditConfig mondoov2.MondooAuditConfig) {
	s.auditConfig = auditConfig

	// Disable container image resolution to be able to run the k8s resources scan CronJob with a local image.
	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	zap.S().Info("Create an audit config that enables only nodes scanning.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

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

	err = s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&auditConfig, version.Version)
	s.NoErrorf(err, "Couldn't find expected version in MondooAuditConfig.Status.ReconciledByOperatorVersion")
}

// disableContainerImageResolution Creates a MondooOperatorConfig that disables container image resolution. This is needed
// in order to be able to execute the integration tests with local images. A function is returned that will cleanup the
// operator config that was created. It is advised to call it with defer such that the operator config is always deleted
// regardless of the test outcome.
func (s *E2eTestSuite) disableContainerImageResolution() func() {
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

func TestE2eTestSuite(t *testing.T) {
	s := new(E2eTestSuite)
	defer func(s *E2eTestSuite) {
		HandlePanics(recover(), func() {
			if err := s.testCluster.UninstallOperator(); err != nil {
				zap.S().Errorf("Failed to uninstall Mondoo operator. %v", err)
			}
		}, s.T)
	}(s)
	suite.Run(t, s)
}
