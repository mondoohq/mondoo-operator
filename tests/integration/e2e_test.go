package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/cnquery/upstream"
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
	nexusClient *nexus.Client
	integration *nexusK8s.Integration
	token       string
	testCluster *TestCluster

	auditConfig mondoov2.MondooAuditConfig
}

func (s *E2eTestSuite) SetupSuite() {
	s.ctx = context.Background()
	nexusClient, err := nexus.NewClient(&upstream.ServiceAccountCredentials{
		Mrn:         "//agents.api.mondoo.app/spaces/test-infallible-taussig-796596/serviceaccounts/2IGhvB7gzFSqo1E8R5wePR6P5mV",
		ParentMrn:   "//captain.api.mondoo.app/spaces/test-infallible-taussig-796596",
		PrivateKey:  "-----BEGIN PRIVATE KEY-----\nMIG2AgEAMBAGByqGSM49AgEGBSuBBAAiBIGeMIGbAgEBBDCD4LW1SK1KCY1WdajO\npUIuIeyi2EhT0D01rniyMh39QsTyTZcY0PoWyW7TxHmB+CyhZANiAARyikEXFDN2\nS/NTKRoiCqIkChZLOm3git+A+UNJu2FxXdFz8c1hPyHoknMSw3ZB7FR9WmfKPgfA\njMT50NDSz/10i8GflassD7SOmiNRDzccDasuWnIDhAt0L5sAuOpgol0=\n-----END PRIVATE KEY-----\n",
		Certificate: "-----BEGIN CERTIFICATE-----\nMIICkDCCAhegAwIBAgIQS5YdYYa5x7vj9F7iq9BJrDAKBggqhkjOPQQDAzBJMUcw\nRQYDVQQKEz4vL2NhcHRhaW4uYXBpLm1vbmRvby5hcHAvc3BhY2VzL3Rlc3QtaW5m\nYWxsaWJsZS10YXVzc2lnLTc5NjU5NjAgFw0yMjExMzAxMzE0MTVaGA85OTk5MTIz\nMTIzNTk1OVowSTFHMEUGA1UEChM+Ly9jYXB0YWluLmFwaS5tb25kb28uYXBwL3Nw\nYWNlcy90ZXN0LWluZmFsbGlibGUtdGF1c3NpZy03OTY1OTYwdjAQBgcqhkjOPQIB\nBgUrgQQAIgNiAARyikEXFDN2S/NTKRoiCqIkChZLOm3git+A+UNJu2FxXdFz8c1h\nPyHoknMSw3ZB7FR9WmfKPgfAjMT50NDSz/10i8GflassD7SOmiNRDzccDasuWnID\nhAt0L5sAuOpgol2jgcEwgb4wDgYDVR0PAQH/BAQDAgWgMBMGA1UdJQQMMAoGCCsG\nAQUFBwMBMAwGA1UdEwEB/wQCMAAwdAYDVR0RBG0wa4JpLy9hZ2VudHMuYXBpLm1v\nbmRvby5hcHAvc3BhY2VzL3Rlc3QtaW5mYWxsaWJsZS10YXVzc2lnLTc5NjU5Ni9z\nZXJ2aWNlYWNjb3VudHMvMklHaHZCN2d6RlNxbzFFOFI1d2VQUjZQNW1WMBMGA1Ud\nJgQMDAp2ZXJzaW9uOnYyMAoGCCqGSM49BAMDA2cAMGQCMG5LJTJfgcBp5cO0nC9V\nGsCcTRRUheY5NJeVwVSOYOT0Gi+IIe7KEclggUthKA7h4gIwF39KfAHi0MQ4PeT4\nNs8jGggfH9Dqxe3iscPL1b6v9jHO6+gf6MPytrg1Ejy9T5bI\n-----END CERTIFICATE-----\n",
		ApiEndpoint: "http://127.0.0.1:8989",
	})
	s.Require().NoError(err, "Failed to create Nexus client")
	s.nexusClient = nexusClient

	integration, err := s.nexusClient.K8s.CreateIntegration("test-integration").
		EnableNodesScan().
		EnableWorkloadsScan().
		Run(s.ctx)
	s.Require().NoError(err, "Failed to create k8s integration")
	s.integration = integration

	token, err := s.integration.GetLongLivedToken(s.ctx)
	s.Require().NoError(err, "Failed to get long lived integration token")
	s.token = token

	settings := installer.NewDefaultSettings().SetToken(token)
	// if s.installRelease {
	// 	settings = installer.NewReleaseSettings()
	// }

	// if s.enableCnspec {
	// 	settings = settings.EnableCnspec()
	// }

	s.testCluster = StartTestCluster(settings, s.T)
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
	}
}

func (s *E2eTestSuite) TestE2e() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, true, false, s.testCluster.Settings.GetEnableCnspec())

	s.testMondooAuditConfigNodes(auditConfig)

	s.nexusClient.AssetStore.ListAssets(s.ctx, &policy.AssetSearchFilter{
		SpaceMrn: "",
	})
}

func (s *E2eTestSuite) testMondooAuditConfigNodes(auditConfig mondoov2.MondooAuditConfig) {
	s.auditConfig = auditConfig
	zap.S().Info("Create an audit config that enables only nodes scanning.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	zap.S().Info("Verify the nodes scanning cron jobs are created.")

	cronJobs := &batchv1.CronJobList{}
	cronJobLabels := nodes.CronJobLabels(auditConfig)

	// Lits only the CronJobs in the namespace of the MondooAuditConfig and only the ones that exactly match our labels.
	listOpts := &client.ListOptions{Namespace: auditConfig.Namespace, LabelSelector: labels.SelectorFromSet(cronJobLabels)}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, cronJobs, listOpts))

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
