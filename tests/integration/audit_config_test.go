package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"
	"k8s.io/utils/pointer"
)

type AuditConfigSuite struct {
	AuditConfigBaseSuite
}

func (s *AuditConfigSuite) TestReconcile_AllDisabled() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, false)
	s.testMondooAuditConfigAllDisabled(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_KubernetesResources() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, true, false, false)
	auditConfig.Spec.KubernetesResources.ContainerImageScanning = true
	s.testMondooAuditConfigKubernetesResources(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_Nodes() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, true, false)
	s.testMondooAuditConfigNodes(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_AdmissionPermissive() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, true)
	s.testMondooAuditConfigAdmission(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_AdmissionEnforcing() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, true)
	auditConfig.Spec.Admission.Mode = v1alpha2.Enforcing
	s.testMondooAuditConfigAdmission(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_AdmissionEnforcingScaleDownScanApi() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, true)
	auditConfig.Spec.Admission.Mode = v1alpha2.Enforcing
	auditConfig.Spec.Admission.Replicas = pointer.Int32(1)
	auditConfig.Spec.Scanner.Replicas = pointer.Int32(1)
	s.testMondooAuditConfigAdmissionScaleDownScanApi(auditConfig)
}

func TestAuditConfigSuite(t *testing.T) {
	s := new(AuditConfigSuite)
	defer func(s *AuditConfigSuite) {
		HandlePanics(recover(), func() {
			if err := s.testCluster.UninstallOperator(); err != nil {
				zap.S().Errorf("Failed to uninstall Mondoo operator. %v", err)
			}
		}, s.T)
	}(s)
	suite.Run(t, s)
}
