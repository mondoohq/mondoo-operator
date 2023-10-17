// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/tests/framework/installer"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"
)

type AuditConfigUpgradeSuite struct {
	AuditConfigBaseSuite
}

func (s *AuditConfigUpgradeSuite) AfterTest(suiteName, testName string) {
	if s.testCluster != nil {
		s.testCluster.GatherAllMondooLogs(testName, installer.MondooNamespace)
	}
}

func (s *AuditConfigUpgradeSuite) TearDownSuite() {
	s.NoError(s.testCluster.UninstallOperator())
	s.NoError(s.spaceClient.Delete(s.ctx))
}

func (s *AuditConfigUpgradeSuite) TestUpgradePreviousReleaseToLatest() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, true, false, true, false)
	s.testUpgradePreviousReleaseToLatest(auditConfig)
}

func TestAuditConfigUpgradeSuite(t *testing.T) {
	s := new(AuditConfigUpgradeSuite)
	s.installRelease = true
	defer func(s *AuditConfigUpgradeSuite) {
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
