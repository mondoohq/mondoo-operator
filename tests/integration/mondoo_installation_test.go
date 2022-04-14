package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mondoov1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
	mondoocontrollers "go.mondoo.com/mondoo-operator/controllers"
	"go.mondoo.com/mondoo-operator/tests/framework/installer"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
)

type MondooInstallationSuite struct {
	suite.Suite
	ctx           context.Context
	testCluster   *TestCluster
	objsToCleanup []client.Object
}

func (s *MondooInstallationSuite) SetupSuite() {
	s.ctx = context.Background()
	s.testCluster = StartTestCluster(installer.NewDefaultSettings(), s.T)
}

func (s *MondooInstallationSuite) TearDownSuite() {
	s.NoError(s.testCluster.UninstallOperator())
}

func (s *MondooInstallationSuite) AfterTest(suiteName, testName string) {
	if s.testCluster != nil {
		s.testCluster.GatherAllMondooLogs(testName, installer.MondooNamespace)
		s.NoError(s.testCluster.CleanupAuditConfigs())

		for _, o := range s.objsToCleanup {
			s.NoError(s.testCluster.K8sHelper.DeleteResourceIfExists(o))
		}
		s.objsToCleanup = make([]client.Object, 0)
	}
}

func (s *MondooInstallationSuite) TestAuditConfigReconcile() {
	auditConfig := utils.DefaultAuditConfig(s.testCluster.Settings.Namespace, true, false, false)
	s.testMondooAuditConfig(auditConfig)
}

func (s *MondooInstallationSuite) TestAuditConfigReconcile_NonDefaultNamespace() {
	ns := &corev1.Namespace{}
	ns.Name = "some-namespace"
	s.Require().NoErrorf(s.testCluster.K8sHelper.Clientset.Create(s.ctx, ns), "Failed to create namespace.")
	s.objsToCleanup = append(s.objsToCleanup, ns)
	zap.S().Info("Created test namespace.")

	s.Require().NoErrorf(s.testCluster.CreateClientSecret(ns.Name), "Failed to create client secret.")
	zap.S().Infof("Created client secret in namespace %q.", ns.Name)

	sa := &corev1.ServiceAccount{}
	sa.Name = "mondoo-sa"
	sa.Namespace = ns.Name
	s.Require().NoErrorf(s.testCluster.K8sHelper.Clientset.Create(s.ctx, sa), "Failed to create service account.")
	s.objsToCleanup = append(s.objsToCleanup, sa)
	zap.S().Infof("Created service account %q in namespace %q.", sa.Name, ns.Name)

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	clusterRoleBinding.Name = "mondoo-operator-workload2"
	clusterRoleBinding.RoleRef.APIGroup = rbacv1.GroupName
	clusterRoleBinding.RoleRef.Kind = "ClusterRole"
	clusterRoleBinding.RoleRef.Name = "mondoo-operator-workload"

	subject := rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: sa.Name, Namespace: sa.Namespace}
	clusterRoleBinding.Subjects = append(clusterRoleBinding.Subjects, subject)
	s.Require().NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, clusterRoleBinding), "Failed to create cluster role binding.")
	s.objsToCleanup = append(s.objsToCleanup, clusterRoleBinding)
	zap.S().Infof("Created cluster role binding %q.", clusterRoleBinding.Name)

	auditConfig := utils.DefaultAuditConfig(ns.Name, true, false, false)
	auditConfig.Spec.Workloads.ServiceAccount = sa.Name

	s.testMondooAuditConfig(auditConfig)
	s.testCluster.GatherAllMondooLogs(s.T().Name(), auditConfig.Namespace) // Gather the logs from the non-default ns
}

func (s *MondooInstallationSuite) testMondooAuditConfig(auditConfig mondoov1.MondooAuditConfig) {
	zap.S().Info("Create an audit config that enables only workloads scanning.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	zap.S().Info("Make sure the Mondoo k8s client is ready.")
	workloadsLabels := []string{installer.MondooClientsK8sLabel, installer.MondooClientsLabel}
	workloadsLabelsString := strings.Join(workloadsLabels, ",")
	s.Truef(
		s.testCluster.K8sHelper.IsPodReady(workloadsLabelsString, auditConfig.Namespace),
		"Mondoo workloads clients are not in a Ready state.")

	zap.S().Info("Verify the pods are actually created from a Deployment.")
	listOpts, err := utils.LabelSelectorListOptions(workloadsLabelsString)
	listOpts.Namespace = auditConfig.Namespace
	s.NoError(err)

	deployments := &appsv1.DeploymentList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, listOpts))

	// Verify there is just 1 deployment that its name matches the name of the CR and that the
	// replica size is 1.
	s.Equalf(1, len(deployments.Items), "Deployments count in Mondoo namespace is incorrect.")
	expectedWorkloadDeploymentName := fmt.Sprintf(mondoocontrollers.WorkloadDeploymentNameTemplate, auditConfig.Name)
	s.Equalf(expectedWorkloadDeploymentName, deployments.Items[0].Name, "Deployment name does not match expected name based from audit config name.")
	s.Equalf(int32(1), *deployments.Items[0].Spec.Replicas, "Deployment does not have 1 replica.")

	zap.S().Info("Enable nodes auditing.")
	// First retrieve the newest version of the audit config, otherwise we might get errors.
	s.NoError(s.testCluster.K8sHelper.Clientset.Get(
		s.ctx, client.ObjectKeyFromObject(&auditConfig), &auditConfig))

	auditConfig.Spec.Nodes.Enable = true
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Update(s.ctx, &auditConfig),
		"Failed to update Mondoo audit config.")

	zap.S().Info("Verify the nodes client is ready.")
	nodesLabels := []string{installer.MondooClientsNodesLabel, installer.MondooClientsLabel}
	nodesLabelsString := strings.Join(nodesLabels, ",")
	s.Truef(
		s.testCluster.K8sHelper.IsPodReady(nodesLabelsString, auditConfig.Namespace),
		"Mondoo nodes clients are not in a Ready state.")

	zap.S().Info("Verify the pods are actually created from a DaemonSet.")
	listOpts, err = utils.LabelSelectorListOptions(nodesLabelsString)
	listOpts.Namespace = auditConfig.Namespace
	s.NoError(err)

	daemonSets := &appsv1.DaemonSetList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, daemonSets, listOpts))

	// Verify there is just 1 daemon set and that its name matches the name of the CR.
	s.Equalf(1, len(daemonSets.Items), "DaemonSets count in Mondoo namespace is incorrect.")
	expectedDaemonSetName := fmt.Sprintf(mondoocontrollers.NodeDaemonSetNameTemplate, auditConfig.Name)
	s.Equalf(expectedDaemonSetName, daemonSets.Items[0].Name, "DaemonSet name does not match expected name based from audit config name.")
}

func TestMondooInstallationSuite(t *testing.T) {
	s := new(MondooInstallationSuite)
	defer func(s *MondooInstallationSuite) {
		HandlePanics(recover(), func() {
			if err := s.testCluster.UninstallOperator(); err != nil {
				zap.S().Errorf("Failed to uninstall Mondoo operator. %v", err)
			}
		}, s.T)
	}(s)
	suite.Run(t, s)
}
