package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/tests/framework/utils"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

type AuditConfigCustomNamespaceSuite struct {
	AuditConfigBaseSuite
	objsToCleanup []client.Object
	ns            *corev1.Namespace
	sa            *corev1.ServiceAccount
}

func (s *AuditConfigCustomNamespaceSuite) SetupSuite() {
	s.AuditConfigBaseSuite.SetupSuite()

	s.ns = &corev1.Namespace{}
	s.ns.Name = "some-namespace"
	s.Require().NoErrorf(s.testCluster.K8sHelper.Clientset.Create(s.ctx, s.ns), "Failed to create namespace.")
	s.objsToCleanup = append(s.objsToCleanup, s.ns)
	zap.S().Info("Created test namespace.")

	s.Require().NoErrorf(s.testCluster.CreateClientSecret(s.ns.Name), "Failed to create client secret.")
	zap.S().Infof("Created client secret in namespace %q.", s.ns.Name)

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

func (s *AuditConfigCustomNamespaceSuite) TestReconcile_KubernetesResources() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.ns.Name, true, false, false)
	auditConfig.Spec.Scanner.ServiceAccountName = s.sa.Name
	s.testMondooAuditConfigKubernetesResources(auditConfig)
}

func (s *AuditConfigCustomNamespaceSuite) TestReconcile_Nodes() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.ns.Name, false, true, false)
	auditConfig.Spec.Scanner.ServiceAccountName = s.sa.Name
	s.testMondooAuditConfigNodes(auditConfig)
}

func (s *AuditConfigCustomNamespaceSuite) TestReconcile_Admission() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.ns.Name, false, false, true)
	auditConfig.Spec.Scanner.ServiceAccountName = s.sa.Name
	s.testMondooAuditConfigAdmission(auditConfig)
}

func (s *AuditConfigCustomNamespaceSuite) TestReconcile_00_AdmissionMissingSA() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.ns.Name, false, false, true)
	auditConfig.Spec.Scanner.ServiceAccountName = "missing-serviceaccount"
	s.testMondooAuditConfigAdmissionMissingSA(auditConfig)
}

func TestAuditConfigCustomNamespaceSuite(t *testing.T) {
	s := new(AuditConfigCustomNamespaceSuite)
	defer func(s *AuditConfigCustomNamespaceSuite) {
		HandlePanics(recover(), func() {
			if err := s.testCluster.UninstallOperator(); err != nil {
				zap.S().Errorf("Failed to uninstall Mondoo operator. %v", err)
			}
		}, s.T)
	}(s)
	suite.Run(t, s)
}
