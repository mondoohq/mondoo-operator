package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/tests/framework/installer"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MondooInstallationSuite struct {
	suite.Suite
	ctx         context.Context
	testCluster *TestCluster
}

func (s *MondooInstallationSuite) SetupSuite() {
	s.ctx = context.Background()
}

func (s *MondooInstallationSuite) AfterTest(suiteName, testName string) {
	if s.testCluster != nil {
		if !s.T().Failed() {
			s.testCluster.GatherAllMondooLogs(testName, installer.MondooNamespace)
		}
		s.NoError(s.testCluster.UninstallOperator())
	}
}

func (s *MondooInstallationSuite) TestKustomizeInstallation() {
	s.testCluster = StartTestCluster(installer.NewDefaultSettings(), s.T)

	s.testMondooInstallation()
}

func (s *MondooInstallationSuite) TestKustomizeInstallation_NonDefaultNamespace() {
	settings := installer.NewDefaultSettings()
	settings.Namespace = "some-namespace"
	s.testCluster = StartTestCluster(settings, s.T)

	s.testMondooInstallation()
}

func (s *MondooInstallationSuite) testMondooInstallation() {
	zap.S().Info("Create an audit config that enables only workloads scanning.")
	auditConfig := utils.DefaultAuditConfig(s.testCluster.Settings.Namespace, true, false, false)
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	zap.S().Info("Make sure the Mondoo k8s client is ready.")
	workloadsLabels := []string{installer.MondooClientsK8sLabel, installer.MondooClientsLabel}
	workloadsLabelsString := strings.Join(workloadsLabels, ",")
	s.Truef(
		s.testCluster.K8sHelper.IsPodReady(workloadsLabelsString, s.testCluster.Settings.Namespace),
		"Mondoo workloads clients are not in a Ready state.")

	zap.S().Info("Verify the pods are actually created from a Deployment.")
	listOpts, err := utils.LabelSelectorListOptions(workloadsLabelsString)
	s.NoError(err)

	deployments := &appsv1.DeploymentList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, listOpts))

	// Verify there is just 1 deployment that its name matches the name of the CR and that the
	// replica size is 1.
	s.Equalf(1, len(deployments.Items), "Deployments count in Mondoo namespace is incorrect.")
	s.Equalf(auditConfig.Name, deployments.Items[0].Name, "Deployment name does not match audit config name.")
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
		s.testCluster.K8sHelper.IsPodReady(nodesLabelsString, s.testCluster.Settings.Namespace),
		"Mondoo nodes clients are not in a Ready state.")

	zap.S().Info("Verify the pods are actually created from a DaemonSet.")
	listOpts, err = utils.LabelSelectorListOptions(nodesLabelsString)
	s.NoError(err)

	daemonSets := &appsv1.DaemonSetList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, daemonSets, listOpts))

	// Verify there is just 1 daemon set and that its name matches the name of the CR.
	s.Equalf(1, len(daemonSets.Items), "DaemonSets count in Mondoo namespace is incorrect.")
	s.Equalf(auditConfig.Name, daemonSets.Items[0].Name, "DaemonSet name does not match audit config name.")
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
