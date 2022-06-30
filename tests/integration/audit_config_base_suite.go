package integration

import (
	"context"
	_ "embed"
	"fmt"
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
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mondoov2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	mondooadmission "go.mondoo.com/mondoo-operator/controllers/admission"
	"go.mondoo.com/mondoo-operator/controllers/k8s_scan"
	"go.mondoo.com/mondoo-operator/controllers/k8s_scan/container_image"
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

		zap.S().Info("Waiting for cleanup of the test cluster.")
		// wait for deployments to be gone
		// sometimes the operator still terminates ,e.g. the webhook, while the next test already started
		// the new test then fails because resources vanish during the test
		// Check for the ScanAPI Deployment to be present.
		/*
			listOpts := &client.ListOptions{Namespace: s.auditConfig.Namespace, LabelSelector: labels.SelectorFromSet(mondooscanapi.DeploymentLabels(s.auditConfig))}
			zap.S().Info("Searching for ScanAPI Deployment.", "listOpts=", listOpts)

			err := s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
				deployments := &appsv1.DeploymentList{}
				err := s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, listOpts)
				if err == nil {
					if len(deployments.Items) == 0 {
						return true, nil
					}
				} else {
					return false, err
				}
				return false, nil
			})
			s.NoErrorf(err, "Failed to wait for ScanAPI Deployment to be gone")

			webhookLabels := map[string]string{mondooadmission.WebhookLabelKey: mondooadmission.WebhookLabelValue}
			webhookListOpts := &client.ListOptions{Namespace: s.auditConfig.Namespace, LabelSelector: labels.SelectorFromSet(webhookLabels)}
			zap.S().Info("Searching for Webhook Deployment.", "listOpts=", webhookListOpts)
			err = s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
				deployments := &appsv1.DeploymentList{}
				err := s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, webhookListOpts)
				if err == nil {
					if len(deployments.Items) == 0 {
						return true, nil
					}
				} else {
					return false, err
				}
				return false, nil
			})
			s.NoErrorf(err, "Failed to wait for ScanAPI Deployment to be gone")
		*/

		// not sure why the above list does not work. It returns zero deployments. So, first a plain sleep to stabilize the test.
		time.Sleep(time.Second * 5)
		zap.S().Info("Cleanup done. Cluster should be good to go for the next test.")
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

	// K8s container image scan
	zap.S().Info("Make sure the Mondoo k8s container image scan CronJob is created.")
	cronJob = &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: container_image.CronJobName(auditConfig.Name), Namespace: auditConfig.Namespace},
	}
	err = s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		if err := s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(cronJob), cronJob); err != nil {
			return false, nil
		}
		return true, nil
	})
	s.NoError(err, "Kubernetes container image scanning CronJob was not created.")

	cronJobLabels = container_image.CronJobLabels(auditConfig)
	s.True(
		s.testCluster.K8sHelper.WaitUntilCronJobsSuccessful(utils.LabelsToLabelSelector(cronJobLabels), auditConfig.Namespace),
		"Kubernetes container image scan CronJob did not run successfully.")

	err = s.testCluster.K8sHelper.CheckForPodInStatus(&auditConfig, "client-k8s-images-scan")
	s.NoErrorf(err, "Couldn't find container image scan pod in Podlist of the MondooAuditConfig Status")

	err = s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&auditConfig)
	s.NoErrorf(err, "Couldn't find expected version in MondooAuditConfig.Status.ReconciledByOperatorVersion")
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

	for _, node := range nodes.Items {
		err := s.testCluster.K8sHelper.CheckForPodInStatus(&auditConfig, "client-node-"+node.Name)
		s.NoErrorf(err, "Couldn't find NodeScan Pod for node "+node.Name+" in Podlist of the MondooAuditConfig Status")
	}

	err = s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&auditConfig)
	s.NoErrorf(err, "Couldn't find expected version in MondooAuditConfig.Status.ReconciledByOperatorVersion")
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
	zap.S().Info("Waiting for webhook Pod to become ready.")
	webhookLabels := []string{mondooadmission.WebhookLabelKey + "=" + mondooadmission.WebhookLabelValue}
	webhookLabelsString := strings.Join(webhookLabels, ",")
	s.Truef(
		s.testCluster.K8sHelper.IsPodReady(webhookLabelsString, auditConfig.Namespace),
		"Mondoo webhook Pod is not in a Ready state.")
	zap.S().Info("Webhook Pod is ready.")

	// Verify scan API deployment and service
	s.validateScanApiDeployment(auditConfig)

	// Check number of Pods depending on mode
	webhookListOpts, err := utils.LabelSelectorListOptions(webhookLabelsString)
	s.NoError(err)
	webhookListOpts.Namespace = auditConfig.Namespace
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

	if auditConfig.Spec.Admission.Mode == mondoov2.Enforcing {
		s.Equalf(*vwc.Webhooks[0].FailurePolicy, webhooksv1.Fail, "Webhook failurePolicy should be 'Fail' because of enforcing mode")
	} else {
		s.Equalf(*vwc.Webhooks[0].FailurePolicy, webhooksv1.Ignore, "Webhook failurePolicy should be 'Ignore' because of permissive mode")
	}

	if *vwc.Webhooks[0].FailurePolicy == webhooksv1.Fail {
		// Try and fail to Update() a Deployment
		deployments := &appsv1.DeploymentList{}
		s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, webhookListOpts))

		s.Equalf(1, len(deployments.Items), "Deployments count for webhook should be precisely one")

		deployments.Items[0].Labels["testLabel"] = "testValue"

		s.Errorf(
			s.testCluster.K8sHelper.Clientset.Update(s.ctx, &deployments.Items[0]),
			"Expected failed updated of Deployment because certificate setup is incomplete")

	}

	// Now put the CA data into the webhook
	for i := range vwc.Webhooks {
		vwc.Webhooks[i].ClientConfig.CABundle = caCert.Bytes()
	}

	zap.S().Info("Update the webhook with the CA data.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Update(s.ctx, vwc),
		"Failed to add CA data to Webhook")

	// Some time is needed before the webhook starts working. Might be a better way to check this but
	// will have to do with a sleep for now.
	zap.S().Info("Wait for webhook to start working.")
	time.Sleep(10 * time.Second)

	zap.S().Info("Webhook should be working by now.")

	err = s.testCluster.K8sHelper.CheckForDegradedCondition(&auditConfig, mondoov2.AdmissionDegraded)
	s.NoErrorf(err, "Admission shouldn't be in degraded state")

	err = s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&auditConfig)
	s.NoErrorf(err, "Couldn't find expected version in MondooAuditConfig.Status.ReconciledByOperatorVersion")

	s.checkPods(&auditConfig)
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

	// Pod should not start, because of missing service account
	var scanApiLabels []string
	for k, v := range mondooscanapi.DeploymentLabels(auditConfig) {
		scanApiLabels = append(scanApiLabels, fmt.Sprintf("%s=%s", k, v))
	}
	scanApiLabelsString := strings.Join(scanApiLabels, ",")
	// do not wait until IsPodReady timeout, pod will not be present
	// something like eventually from ginko would be nice, first iteration just with a sleep.
	// just a grace period
	time.Sleep(10 * time.Second)
	listOpts, err := utils.LabelSelectorListOptions(scanApiLabelsString)
	s.NoError(err)
	listOpts.Namespace = auditConfig.Namespace
	podList := &corev1.PodList{}

	zap.S().Info(listOpts)
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
	err = s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&auditConfig)
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

	err := s.testCluster.K8sHelper.CheckForReconciledOperatorVersion(&s.auditConfig)
	s.NoErrorf(err, "Couldn't find expected version in MondooAuditConfig.Status.ReconciledByOperatorVersion")
}

func (s *AuditConfigBaseSuite) validateScanApiDeployment(auditConfig mondoov2.MondooAuditConfig) {
	scanApiLabelsString := utils.LabelsToLabelSelector(mondooscanapi.DeploymentLabels(auditConfig))
	zap.S().Info("Waiting for scan API Pod to become ready.")
	s.Truef(
		s.testCluster.K8sHelper.IsPodReady(scanApiLabelsString, auditConfig.Namespace),
		"Mondoo scan API Pod is not in a Ready state.")
	zap.S().Info("Scan API Pod is ready.")

	scanApiService := mondooscanapi.ScanApiService(auditConfig.Namespace, auditConfig)
	err := s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		err := s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(scanApiService), scanApiService)
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	s.NoErrorf(err, "Failed to get scan API service.")

	expectedService := mondooscanapi.ScanApiService(auditConfig.Namespace, auditConfig)
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

func (s *AuditConfigBaseSuite) getPassingPod() *corev1.Pod {
	labels := map[string]string{
		"admission-result": "pass",
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "passing-pod",
			Namespace: "mondoo-operator",
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
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
						RunAsNonRoot:             pointer.Bool(true),
						RunAsUser:                pointer.Int64(1000),
						ReadOnlyRootFilesystem:   pointer.Bool(true),
						AllowPrivilegeEscalation: pointer.Bool(false),
						Privileged:               pointer.Bool(false),
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
			AutomountServiceAccountToken: pointer.Bool(false),
			ServiceAccountName:           "mondoo-operator-controller-manager",
			DeprecatedServiceAccount:     "",
		},
	}
}

func (s *AuditConfigBaseSuite) getFailingPod() *corev1.Pod {
	labels := map[string]string{
		"admission-result": "fail",
	}
	pod := s.getPassingPod().DeepCopy()
	pod.ObjectMeta.Name = "failing-pod"
	pod.ObjectMeta.Labels = labels
	pod.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
		Privileged:               pointer.Bool(true),
		RunAsNonRoot:             pointer.Bool(false),
		AllowPrivilegeEscalation: pointer.Bool(true),
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{"CAP_SYS_ADMIN"},
		},
	}
	return pod
}

func (s *AuditConfigBaseSuite) checkPods(auditConfig *mondoov2.MondooAuditConfig) {
	passingPod := s.getPassingPod()
	failingPod := s.getFailingPod()

	zap.S().Info("Create a Pod which should pass.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, passingPod),
		"Failed to create Pod which should pass.")

	zap.S().Info("Create a Pod which should be denied in enforcing mode.")
	err := s.testCluster.K8sHelper.Clientset.Create(s.ctx, failingPod)

	if auditConfig.Spec.Admission.Mode == mondoov2.Enforcing {
		s.Errorf(err, "Created Pod which should have been denied.")
	} else {
		s.NoErrorf(err, "Failed creating a Pod in permissive mode.")
	}

	s.NoErrorf(s.testCluster.K8sHelper.DeleteResourceIfExists(passingPod), "Failed to delete passingPod")
	s.NoErrorf(s.testCluster.K8sHelper.DeleteResourceIfExists(failingPod), "Failed to delete failingPod")
	s.NoErrorf(s.testCluster.K8sHelper.WaitForResourceDeletion(passingPod), "Error waiting for deleteion of passingPod")
	s.NoErrorf(s.testCluster.K8sHelper.WaitForResourceDeletion(failingPod), "Error waiting for deleteion of failingPod")
}
