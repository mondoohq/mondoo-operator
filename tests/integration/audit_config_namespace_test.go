package integration

// import (
// 	"context"
// 	"fmt"
// 	"strings"
// 	"testing"

// 	"github.com/stretchr/testify/suite"
// 	"go.uber.org/zap"

// 	webhooksv1 "k8s.io/api/admissionregistration/v1"
// 	appsv1 "k8s.io/api/apps/v1"
// 	corev1 "k8s.io/api/core/v1"
// 	rbacv1 "k8s.io/api/rbac/v1"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"sigs.k8s.io/controller-runtime/pkg/client"

// 	mondoov1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
// 	mondoocontrollers "go.mondoo.com/mondoo-operator/controllers"
// 	mondoowebhooks "go.mondoo.com/mondoo-operator/controllers/webhooks"
// 	"go.mondoo.com/mondoo-operator/tests/framework/installer"
// 	"go.mondoo.com/mondoo-operator/tests/framework/utils"
// )

// type AuditConfigNamespaceSuite struct {
// 	suite.Suite
// 	ctx           context.Context
// 	testCluster   *TestCluster
// 	objsToCleanup []client.Object
// }

// func (s *AuditConfigNamespaceSuite) SetupSuite() {
// 	s.ctx = context.Background()
// 	s.testCluster = StartTestCluster(installer.NewDefaultSettings(), s.T)
// }

// func (s *AuditConfigNamespaceSuite) TearDownSuite() {
// 	s.NoError(s.testCluster.UninstallOperator())
// }

// func (s *AuditConfigNamespaceSuite) AfterTest(suiteName, testName string) {
// 	if s.testCluster != nil {
// 		s.testCluster.GatherAllMondooLogs(testName, installer.MondooNamespace)
// 		s.NoError(s.testCluster.CleanupAuditConfigs())

// 		for _, o := range s.objsToCleanup {
// 			s.NoError(s.testCluster.K8sHelper.DeleteResourceIfExists(o))
// 		}
// 		s.objsToCleanup = make([]client.Object, 0)
// 	}
// }

// func (s *AuditConfigNamespaceSuite) TestAuditConfigReconcile_Workloads() {
// 	auditConfig := utils.DefaultAuditConfig(s.testCluster.Settings.Namespace, true, false, false)
// 	s.testMondooAuditConfigWorkloads(auditConfig)
// }

// func (s *MondooInstallationSuite) TestAuditConfigReconcile_NonDefaultNamespace() {
// 	ns := &corev1.Namespace{}
// 	ns.Name = "some-namespace"
// 	s.Require().NoErrorf(s.testCluster.K8sHelper.Clientset.Create(s.ctx, ns), "Failed to create namespace.")
// 	s.objsToCleanup = append(s.objsToCleanup, ns)
// 	zap.S().Info("Created test namespace.")

// 	s.Require().NoErrorf(s.testCluster.CreateClientSecret(ns.Name), "Failed to create client secret.")
// 	zap.S().Infof("Created client secret in namespace %q.", ns.Name)

// 	sa := &corev1.ServiceAccount{}
// 	sa.Name = "mondoo-sa"
// 	sa.Namespace = ns.Name
// 	s.Require().NoErrorf(s.testCluster.K8sHelper.Clientset.Create(s.ctx, sa), "Failed to create service account.")
// 	s.objsToCleanup = append(s.objsToCleanup, sa)
// 	zap.S().Infof("Created service account %q in namespace %q.", sa.Name, ns.Name)

// 	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
// 	clusterRoleBinding.Name = "mondoo-operator-workload2"
// 	clusterRoleBinding.RoleRef.APIGroup = rbacv1.GroupName
// 	clusterRoleBinding.RoleRef.Kind = "ClusterRole"
// 	clusterRoleBinding.RoleRef.Name = "mondoo-operator-workload"

// 	subject := rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: sa.Name, Namespace: sa.Namespace}
// 	clusterRoleBinding.Subjects = append(clusterRoleBinding.Subjects, subject)
// 	s.Require().NoErrorf(
// 		s.testCluster.K8sHelper.Clientset.Create(s.ctx, clusterRoleBinding), "Failed to create cluster role binding.")
// 	s.objsToCleanup = append(s.objsToCleanup, clusterRoleBinding)
// 	zap.S().Infof("Created cluster role binding %q.", clusterRoleBinding.Name)

// 	auditConfig := utils.DefaultAuditConfig(ns.Name, true, false, false)
// 	auditConfig.Spec.Workloads.ServiceAccount = sa.Name

// 	s.testMondooAuditConfig(auditConfig)
// 	s.testCluster.GatherAllMondooLogs(s.T().Name(), auditConfig.Namespace) // Gather the logs from the non-default ns
// }

// func (s *MondooInstallationSuite) testMondooAuditConfigWorkloads(auditConfig mondoov1.MondooAuditConfig) {
// 	zap.S().Info("Create an audit config that enables only workloads scanning.")
// 	s.NoErrorf(
// 		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
// 		"Failed to create Mondoo audit config.")

// 	zap.S().Info("Make sure the Mondoo k8s client is ready.")
// 	workloadsLabels := []string{installer.MondooClientsK8sLabel, installer.MondooClientsLabel}
// 	workloadsLabelsString := strings.Join(workloadsLabels, ",")
// 	s.Truef(
// 		s.testCluster.K8sHelper.IsPodReady(workloadsLabelsString, auditConfig.Namespace),
// 		"Mondoo workloads clients are not in a Ready state.")

// 	zap.S().Info("Verify the pods are actually created from a Deployment.")
// 	listOpts, err := utils.LabelSelectorListOptions(workloadsLabelsString)
// 	listOpts.Namespace = auditConfig.Namespace
// 	s.NoError(err)

// 	deployments := &appsv1.DeploymentList{}
// 	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, listOpts))

// 	// Verify there is just 1 deployment that its name matches the name of the CR and that the
// 	// replica size is 1.
// 	s.Equalf(1, len(deployments.Items), "Deployments count in Mondoo namespace is incorrect.")
// 	expectedWorkloadDeploymentName := fmt.Sprintf(mondoocontrollers.WorkloadDeploymentNameTemplate, auditConfig.Name)
// 	s.Equalf(expectedWorkloadDeploymentName, deployments.Items[0].Name, "Deployment name does not match expected name based from audit config name.")
// 	s.Equalf(int32(1), *deployments.Items[0].Spec.Replicas, "Deployment does not have 1 replica.")
// }

// func (s *MondooInstallationSuite) testMondooAuditConfig(auditConfig mondoov1.MondooAuditConfig) {
// 	zap.S().Info("Create an audit config that enables only workloads scanning.")
// 	s.NoErrorf(
// 		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
// 		"Failed to create Mondoo audit config.")

// 	zap.S().Info("Make sure the Mondoo k8s client is ready.")
// 	workloadsLabels := []string{installer.MondooClientsK8sLabel, installer.MondooClientsLabel}
// 	workloadsLabelsString := strings.Join(workloadsLabels, ",")
// 	s.Truef(
// 		s.testCluster.K8sHelper.IsPodReady(workloadsLabelsString, auditConfig.Namespace),
// 		"Mondoo workloads clients are not in a Ready state.")

// 	zap.S().Info("Verify the pods are actually created from a Deployment.")
// 	listOpts, err := utils.LabelSelectorListOptions(workloadsLabelsString)
// 	listOpts.Namespace = auditConfig.Namespace
// 	s.NoError(err)

// 	deployments := &appsv1.DeploymentList{}
// 	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, listOpts))

// 	// Verify there is just 1 deployment that its name matches the name of the CR and that the
// 	// replica size is 1.
// 	s.Equalf(1, len(deployments.Items), "Deployments count in Mondoo namespace is incorrect.")
// 	expectedWorkloadDeploymentName := fmt.Sprintf(mondoocontrollers.WorkloadDeploymentNameTemplate, auditConfig.Name)
// 	s.Equalf(expectedWorkloadDeploymentName, deployments.Items[0].Name, "Deployment name does not match expected name based from audit config name.")
// 	s.Equalf(int32(1), *deployments.Items[0].Spec.Replicas, "Deployment does not have 1 replica.")

// 	zap.S().Info("Enable nodes auditing.")
// 	// First retrieve the newest version of the audit config, otherwise we might get errors.
// 	s.NoError(s.testCluster.K8sHelper.Clientset.Get(
// 		s.ctx, client.ObjectKeyFromObject(&auditConfig), &auditConfig))

// 	// Turn on node scanning; turn off workload scanning
// 	auditConfig.Spec.Nodes.Enable = true
// 	auditConfig.Spec.Workloads.Enable = false
// 	s.NoErrorf(
// 		s.testCluster.K8sHelper.Clientset.Update(s.ctx, &auditConfig),
// 		"Failed to update Mondoo audit config.")

// 	zap.S().Info("Verify the nodes client is ready.")
// 	nodesLabels := []string{installer.MondooClientsNodesLabel, installer.MondooClientsLabel}
// 	nodesLabelsString := strings.Join(nodesLabels, ",")
// 	s.Truef(
// 		s.testCluster.K8sHelper.IsPodReady(nodesLabelsString, auditConfig.Namespace),
// 		"Mondoo nodes clients are not in a Ready state.")

// 	zap.S().Info("Verify the pods are actually created from a DaemonSet.")
// 	listOpts, err = utils.LabelSelectorListOptions(nodesLabelsString)
// 	listOpts.Namespace = auditConfig.Namespace
// 	s.NoError(err)

// 	daemonSets := &appsv1.DaemonSetList{}
// 	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, daemonSets, listOpts))

// 	// Verify there is just 1 daemon set and that its name matches the name of the CR.
// 	s.Equalf(1, len(daemonSets.Items), "DaemonSets count in Mondoo namespace is incorrect.")
// 	expectedDaemonSetName := fmt.Sprintf(mondoocontrollers.NodeDaemonSetNameTemplate, auditConfig.Name)
// 	s.Equalf(expectedDaemonSetName, daemonSets.Items[0].Name, "DaemonSet name does not match expected name based from audit config name.")

// 	zap.S().Info("Enable webhook.")

// 	// Generate certificates manually
// 	serviceDNSNames := []string{
// 		// DNS names will take the form of ServiceName-ServiceNamespace.svc and .svc.cluster.local
// 		fmt.Sprintf("%s-webhook-service.%s.svc", auditConfig.Name, auditConfig.Namespace),
// 		fmt.Sprintf("%s-webhook-service.%s.svc.cluster.local", auditConfig.Name, auditConfig.Namespace),
// 	}
// 	secretName := "webhook-server-cert"
// 	caCert, err := s.testCluster.MondooInstaller.GenerateServiceCerts(&auditConfig, secretName, serviceDNSNames)

// 	// Don't bother with further webhook tests if we couldnt' save the certificates
// 	if s.NoErrorf(err, "Error while generating/saving certificates for webhook service") {
// 		// Disable imageResolution for the webhook image to be runnable.
// 		// Otherwise, mondoo-operator will try to resolve the locally-built mondoo-operator container
// 		// image, and fail because we haven't pushed this image publicly.
// 		operatorConfig := &mondoov1.MondooOperatorConfig{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Name: mondoov1.MondooOperatorConfigName,
// 			},
// 		}
// 		s.Require().NoErrorf(
// 			s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(operatorConfig), operatorConfig),
// 			"Failed to get existing MondooOperatorConfig")

// 		operatorConfig.Spec.SkipContainerResolution = true
// 		s.Require().NoErrorf(s.testCluster.K8sHelper.Clientset.Update(s.ctx, operatorConfig),
// 			"Failed to set SkipContainerResolution on MondooOperatorConfig for webhook test")

// 		// Enable webhook
// 		s.NoError(s.testCluster.K8sHelper.Clientset.Get(
// 			s.ctx, client.ObjectKeyFromObject(&auditConfig), &auditConfig))

// 		auditConfig.Spec.Nodes.Enable = false
// 		auditConfig.Spec.Webhooks.Enable = true
// 		s.NoErrorf(
// 			s.testCluster.K8sHelper.Clientset.Update(s.ctx, &auditConfig),
// 			"Failed to update MondooAuditConfig.")

// 		// Wait for Ready Pod
// 		webhookLabels := []string{mondoowebhooks.WebhookLabelKey + "=" + mondoowebhooks.WebhookLabelValue}
// 		webhookLabelsString := strings.Join(webhookLabels, ",")
// 		s.Truef(
// 			s.testCluster.K8sHelper.IsPodReady(webhookLabelsString, auditConfig.Namespace),
// 			"Mondoo webhook Pod is not in a Ready state.")

// 		// Change the webhook from Ignore to Fail to prove that the webhook is active
// 		vwc := &webhooksv1.ValidatingWebhookConfiguration{
// 			ObjectMeta: metav1.ObjectMeta{
// 				// namespace-name-mondoo
// 				Name: fmt.Sprintf("%s-%s-mondoo", auditConfig.Namespace, auditConfig.Name),
// 			},
// 		}
// 		s.NoErrorf(
// 			s.testCluster.K8sHelper.Clientset.Get(s.ctx, client.ObjectKeyFromObject(vwc), vwc),
// 			"Failed to retrieve ValidatingWebhookConfiguration")

// 		fail := webhooksv1.Fail
// 		for i := range vwc.Webhooks {
// 			vwc.Webhooks[i].FailurePolicy = &fail
// 		}

// 		s.NoErrorf(
// 			s.testCluster.K8sHelper.Clientset.Update(s.ctx, vwc),
// 			"Failed to change Webhook FailurePolicy to Fail")

// 		// Try and fail to Update() a Deployment

// 		listOpts, err = utils.LabelSelectorListOptions(webhookLabelsString)
// 		s.NoError(err)
// 		listOpts.Namespace = auditConfig.Namespace

// 		deployments = &appsv1.DeploymentList{}
// 		s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, listOpts))

// 		s.Equalf(1, len(deployments.Items), "Deployments count for webhook should be precisely one")

// 		deployments.Items[0].Labels["testLabel"] = "testValue"

// 		s.Errorf(
// 			s.testCluster.K8sHelper.Clientset.Update(s.ctx, &deployments.Items[0]),
// 			"Expected failed updated of Deployment because certificate setup is incomplete")

// 		// Now put the CA data into the webhook
// 		for i := range vwc.Webhooks {
// 			vwc.Webhooks[i].ClientConfig.CABundle = caCert.Bytes()
// 		}

// 		s.NoErrorf(
// 			s.testCluster.K8sHelper.Clientset.Update(s.ctx, vwc),
// 			"Failed to add CA data to Webhook")

// 		// Now the Deployment Update() should work
// 		s.NoErrorf(
// 			s.testCluster.K8sHelper.Clientset.Update(s.ctx, &deployments.Items[0]),
// 			"Expected update of Deployment to succeed after CA data applied to webhook")

// 		// Bring back the default image resolution behavior
// 		operatorConfig.Spec.SkipContainerResolution = false

// 		s.NoErrorf(
// 			s.testCluster.K8sHelper.Clientset.Update(s.ctx, operatorConfig),
// 			"Failed to restore container resolution in MondooOperatorConfig")
// 	}
// }

// func TestAuditConfigNamespaceSuite(t *testing.T) {
// 	s := new(MondooInstallationSuite)
// 	defer func(s *MondooInstallationSuite) {
// 		HandlePanics(recover(), func() {
// 			if err := s.testCluster.UninstallOperator(); err != nil {
// 				zap.S().Errorf("Failed to uninstall Mondoo operator. %v", err)
// 			}
// 		}, s.T)
// 	}(s)
// 	suite.Run(t, s)
// }
