// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"
)

type AuditConfigOOMSuite struct {
	AuditConfigBaseSuite
}

func (s *AuditConfigOOMSuite) TestOOMControllerReporting() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, false, false)
	s.testOOMMondooOperatorController(auditConfig)
}

func (s *AuditConfigOOMSuite) TestOOMScanAPI() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, true, false, false, false)
	s.testOOMScanAPI(auditConfig)
}

func (s *AuditConfigOOMSuite) TestOOMNodeScan() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, true, false)
	s.testOOMNodeScan(auditConfig)
}

func (s *AuditConfigOOMSuite) TearDownSuite() {
	s.AuditConfigBaseSuite.TearDownSuite()
}

func TestAuditConfigOOMSuite(t *testing.T) {
	s := new(AuditConfigOOMSuite)
	defer func(s *AuditConfigOOMSuite) {
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
