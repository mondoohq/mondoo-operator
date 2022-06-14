package integration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	webhooksv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mondoov2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	mondooadmission "go.mondoo.com/mondoo-operator/controllers/admission"
	"go.mondoo.com/mondoo-operator/controllers/k8s_scan"
	"go.mondoo.com/mondoo-operator/controllers/nodes"
	mondooscanapi "go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/tests/framework/installer"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	ctrl "sigs.k8s.io/controller-runtime"
)

type AuditConfigBaseSuite struct {
	suite.Suite
	ctx         context.Context
	testCluster *TestCluster
	auditConfig mondoov2.MondooAuditConfig
}

func (s *AuditConfigBaseSuite) SetupSuite() {
	s.ctx = context.Background()
	s.testCluster = StartTestCluster(installer.NewDefaultSettings(), s.T)
}

func (s *AuditConfigBaseSuite) TearDownSuite() {
	s.NoError(s.testCluster.UninstallOperator())
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

	// Verify scan API deployment and service
	s.validateScanApiDeployment(auditConfig)

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
}

func (s *AuditConfigBaseSuite) testMondooAuditConfigNodes(auditConfig mondoov2.MondooAuditConfig) {
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

	nodes := &corev1.NodeList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, nodes))

	// Verify the amount of CronJobs created is equal to the amount of nodes
	err := s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, cronJobs, listOpts))
		if len(nodes.Items) == len(cronJobs.Items) {
			return true, nil
		}
		return false, nil
	})
	s.NoErrorf(
		err,
		"The amount of node scanning CronJobs is not equal to the amount of cluster nodes. expected: %d; actual: %d",
		len(nodes.Items), len(cronJobs.Items))

	for _, c := range cronJobs.Items {
		found := false
		for _, n := range nodes.Items {
			if n.Name == c.Spec.JobTemplate.Spec.Template.Spec.NodeName {
				found = true
			}
		}
		s.Truef(found, "CronJob %s/%s does not have a corresponding cluster node.", c.Namespace, c.Name)
	}

	// Make sure we have 1 successful run for each CronJob
	selector := utils.LabelsToLabelSelector(cronJobLabels)
	s.True(s.testCluster.K8sHelper.WaitUntilCronJobsSuccessful(selector, auditConfig.Namespace), "Not all CronJobs have run successfully.")
}

func (s *AuditConfigBaseSuite) testMondooAuditConfigAdmission(auditConfig mondoov2.MondooAuditConfig) {
	s.auditConfig = auditConfig
	// Generate certificates manually
	serviceDNSNames := []string{
		// DNS names will take the form of ServiceName.ServiceNamespace.svc and .svc.cluster.local
		fmt.Sprintf("%s-webhook-service.%s.svc", auditConfig.Name, auditConfig.Namespace),
		fmt.Sprintf("%s-webhook-service.%s.svc.cluster.local", auditConfig.Name, auditConfig.Namespace),
	}
	secretName := mondooadmission.GetTLSCertificatesSecretName(auditConfig.Name)
	caCert, err := s.testCluster.MondooInstaller.GenerateServiceCerts(&auditConfig, secretName, serviceDNSNames)

	// Don't bother with further webhook tests if we couldnt' save the certificates
	s.Require().NoErrorf(err, "Error while generating/saving certificates for webhook service")
	// Disable imageResolution for the webhook image to be runnable.
	// Otherwise, mondoo-operator will try to resolve the locally-built mondoo-operator container
	// image, and fail because we haven't pushed this image publicly.
	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	// Enable webhook
	zap.S().Info("Create an audit config that enables only admission control.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	// Wait for Ready Pod
	webhookLabels := []string{mondooadmission.WebhookLabelKey + "=" + mondooadmission.WebhookLabelValue}
	webhookLabelsString := strings.Join(webhookLabels, ",")
	s.Truef(
		s.testCluster.K8sHelper.IsPodReady(webhookLabelsString, auditConfig.Namespace),
		"Mondoo webhook Pod is not in a Ready state.")

	// Verify scan API deployment and service
	s.validateScanApiDeployment(auditConfig)

	// Change the webhook from Ignore to Fail to prove that the webhook is active
	vwc := &webhooksv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			// namespace-name-mondoo
			Name: fmt.Sprintf("%s-%s-mondoo", auditConfig.Namespace, auditConfig.Name),
		},
	}
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(vwc), vwc),
		"Failed to retrieve ValidatingWebhookConfiguration")

	fail := webhooksv1.Fail
	for i := range vwc.Webhooks {
		vwc.Webhooks[i].FailurePolicy = &fail
	}

	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Update(s.ctx, vwc),
		"Failed to change Webhook FailurePolicy to Fail")

	// Try and fail to Update() a Deployment
	listOpts, err := utils.LabelSelectorListOptions(webhookLabelsString)
	s.NoError(err)
	listOpts.Namespace = auditConfig.Namespace

	deployments := &appsv1.DeploymentList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, listOpts))

	s.Equalf(1, len(deployments.Items), "Deployments count for webhook should be precisely one")

	deployments.Items[0].Labels["testLabel"] = "testValue"

	s.Errorf(
		s.testCluster.K8sHelper.Clientset.Update(s.ctx, &deployments.Items[0]),
		"Expected failed updated of Deployment because certificate setup is incomplete")

	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(vwc), vwc),
		"Failed to retrieve ValidatingWebhookConfiguration")

	// Now put the CA data into the webhook
	for i := range vwc.Webhooks {
		vwc.Webhooks[i].ClientConfig.CABundle = caCert.Bytes()
	}

	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Update(s.ctx, vwc),
		"Failed to add CA data to Webhook")

	// Some time is needed before the webhook starts working. Might be a better way to check this but
	// will have to do with a sleep for now.
	time.Sleep(5 * time.Second)

	// Now the Deployment Update() should work
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Update(s.ctx, &deployments.Items[0]),
		"Expected update of Deployment to succeed after CA data applied to webhook")
}

func (s *AuditConfigBaseSuite) validateScanApiDeployment(auditConfig mondoov2.MondooAuditConfig) {
	scanApiLabelsString := utils.LabelsToLabelSelector(mondooscanapi.DeploymentLabels(auditConfig))
	s.Truef(
		s.testCluster.K8sHelper.IsPodReady(scanApiLabelsString, auditConfig.Namespace),
		"Mondoo scan API Pod is not in a Ready state.")

	scanApiService := mondooscanapi.ScanApiService(auditConfig.Namespace, auditConfig)
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(scanApiService), scanApiService),
		"Failed to get scan API service.")

	expectedService := mondooscanapi.ScanApiService(auditConfig.Namespace, auditConfig)
	s.NoError(ctrl.SetControllerReference(&auditConfig, expectedService, s.testCluster.K8sHelper.Clientset.Scheme()))
	s.Truef(k8s.AreServicesEqual(*expectedService, *scanApiService), "Scan API service is not as expected.")
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
