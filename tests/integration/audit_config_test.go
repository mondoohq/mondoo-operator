package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	"go.mondoo.com/mondoo-operator/tests/framework/utils"
)

type AuditConfigSuite struct {
	AuditConfigBaseSuite
}

func (s *AuditConfigSuite) TestReconcile_KubernetesResources() {
	auditConfig := utils.DefaultAuditConfig(s.testCluster.Settings.Namespace, true, false, false)
	s.testMondooAuditConfigWorkloads(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_Nodes() {
	auditConfig := utils.DefaultAuditConfig(s.testCluster.Settings.Namespace, false, true, false)
	s.testMondooAuditConfigNodes(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_Admission() {
	auditConfig := utils.DefaultAuditConfig(s.testCluster.Settings.Namespace, false, false, true)
	s.testMondooAuditConfigAdmission(auditConfig)
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
