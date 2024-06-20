// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

type AuditConfigCustomNamespaceSuite struct {
	AuditConfigBaseSuite
	objsToCleanup         []client.Object
	ns                    *corev1.Namespace
	sa                    *corev1.ServiceAccount
	webhookServiceAccount *corev1.ServiceAccount
}

func (s *AuditConfigCustomNamespaceSuite) SetupSuite() {
	s.AuditConfigBaseSuite.SetupSuite()
	s.testCluster.MondooInstaller.Settings.SuiteName = "AuditConfigCustomNamespaceSuite"

	s.ns = &corev1.Namespace{}
	s.ns.Name = "some-namespace"
	s.Require().NoErrorf(s.testCluster.K8sHelper.Clientset.Create(s.ctx, s.ns), "Failed to create namespace.")
	s.objsToCleanup = append(s.objsToCleanup, s.ns)
	zap.S().Info("Created test namespace.")

	s.Require().NoErrorf(s.testCluster.CreateClientSecret(s.ns.Name), "Failed to create client secret.")
	zap.S().Infof("Created client secret in namespace %q.", s.ns.Name)

	s.webhookServiceAccount = &corev1.ServiceAccount{}
	s.webhookServiceAccount.Name = "webhook-sa"
	s.webhookServiceAccount.Namespace = s.ns.Name
	s.Require().NoErrorf(s.testCluster.K8sHelper.Clientset.Create(s.ctx, s.webhookServiceAccount), "Failed to create webhook ServiceAccount")
	s.objsToCleanup = append(s.objsToCleanup, s.webhookServiceAccount)
	zap.S().Infof("Created webhook ServiceAccount %q in namespace %q.", s.webhookServiceAccount.Name, s.webhookServiceAccount.Namespace)

	s.sa = &corev1.ServiceAccount{}
	s.sa.Name = "mondoo-sa"
	s.sa.Namespace = s.ns.Name
	s.Require().NoErrorf(s.testCluster.K8sHelper.Clientset.Create(s.ctx, s.sa), "Failed to create service account.")
	s.objsToCleanup = append(s.objsToCleanup, s.sa)
	zap.S().Infof("Created service account %q in namespace %q.", s.sa.Name, s.ns.Name)

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	clusterRoleBinding.Name = "mondoo-operator-k8s-resources-scanning2"
	clusterRoleBinding.RoleRef.APIGroup = rbacv1.GroupName
	clusterRoleBinding.RoleRef.Kind = "ClusterRole"
	clusterRoleBinding.RoleRef.Name = "mondoo-operator-k8s-resources-scanning"

	subject := rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: s.sa.Name, Namespace: s.sa.Namespace}
	clusterRoleBinding.Subjects = append(clusterRoleBinding.Subjects, subject)
	s.Require().NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, clusterRoleBinding), "Failed to create cluster role binding.")
	s.objsToCleanup = append(s.objsToCleanup, clusterRoleBinding)
	zap.S().Infof("Created cluster role binding %q.", clusterRoleBinding.Name)
}

func (s *AuditConfigCustomNamespaceSuite) AfterTest(suiteName, testName string) {
	if s.testCluster != nil {
		s.testCluster.GatherAllMondooLogs(testName, s.ns.Name) // Gather logs from the custom namespace too.
	}
	s.AuditConfigBaseSuite.AfterTest(suiteName, testName)
}

func (s *AuditConfigCustomNamespaceSuite) TearDownSuite() {
	for _, o := range s.objsToCleanup {
		s.NoError(s.testCluster.K8sHelper.DeleteResourceIfExists(o))
	}
	s.AuditConfigBaseSuite.TearDownSuite()
}

func (s *AuditConfigCustomNamespaceSuite) TestReconcile_KubernetesResources2() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.ns.Name, true, false, false, false)
	auditConfig.Spec.Scanner.ServiceAccountName = s.sa.Name
	s.testMondooAuditConfigKubernetesResources(auditConfig)
}

func (s *AuditConfigCustomNamespaceSuite) TestReconcile_Containers() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.ns.Name, false, true, false, false)
	auditConfig.Spec.Scanner.ServiceAccountName = s.sa.Name

	// Ignore the operator namespace and the scanner namespace because we cannot scan a local image
	// Ignore kube-system to speed up the containers test
	auditConfig.Spec.Filtering.Namespaces.Exclude = []string{s.ns.Name, s.testCluster.Settings.Namespace, "kube-system"}
	s.testMondooAuditConfigContainers(auditConfig)
}

func (s *AuditConfigCustomNamespaceSuite) TestReconcile_Nodes_CronJobs() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.ns.Name, false, false, true, false)
	auditConfig.Spec.Scanner.ServiceAccountName = s.sa.Name
	s.testMondooAuditConfigNodesCronjobs(auditConfig)
}

func (s *AuditConfigCustomNamespaceSuite) TestReconcile_Nodes_Deployments() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.ns.Name, false, false, true, false)
	auditConfig.Spec.Nodes.Style = v1alpha2.NodeScanStyle_Deployment
	auditConfig.Spec.Nodes.IntervalTimer = 1
	auditConfig.Spec.Scanner.ServiceAccountName = s.sa.Name
	s.testMondooAuditConfigNodesDeployments(auditConfig)
}

func (s *AuditConfigCustomNamespaceSuite) TestReconcile_Admission() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.ns.Name, false, false, false, true)
	auditConfig.Spec.Scanner.ServiceAccountName = s.sa.Name
	auditConfig.Spec.Admission.ServiceAccountName = s.webhookServiceAccount.Name
	s.testMondooAuditConfigAdmission(auditConfig)
}

func (s *AuditConfigCustomNamespaceSuite) TestReconcile_AdmissionMissingSA() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.ns.Name, false, false, false, true)
	auditConfig.Spec.Scanner.ServiceAccountName = "missing-serviceaccount"
	auditConfig.Spec.Admission.ServiceAccountName = s.webhookServiceAccount.Name
	s.testMondooAuditConfigAdmissionMissingSA(auditConfig)
}

func TestAuditConfigCustomNamespaceSuite(t *testing.T) {
	s := new(AuditConfigCustomNamespaceSuite)
	defer func(s *AuditConfigCustomNamespaceSuite) {
		HandlePanics(recover(), func() {
			if err := s.testCluster.UninstallOperator(); err != nil {
				zap.S().Errorf("Failed to uninstall Mondoo operator. %v", err)
			}
			if s.spaceClient != nil {
				if err := s.spaceClient.Delete(s.ctx); err != nil {
					zap.S().Errorf("Failed to delete Mondoo space. %v", err)
				}
			}
		}, s.T)
	}(s)
	suite.Run(t, s)
}
