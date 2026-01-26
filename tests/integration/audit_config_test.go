// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"
)

type AuditConfigSuite struct {
	AuditConfigBaseSuite
}

func (s *AuditConfigSuite) SetupSuite() {
	s.AuditConfigBaseSuite.SetupSuite()
	s.testCluster.Settings.SuiteName = "AuditConfigSuite"
}

func (s *AuditConfigSuite) TestReconcile_AllDisabled() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, false)
	s.testMondooAuditConfigAllDisabled(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_KubernetesResources() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, true, false, false)
	s.testMondooAuditConfigKubernetesResources(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_Containers() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, true, false)

	// Ignore the operator namespace because we cannot scan a local image
	// Ignore kube-system to speed up the containers test
	auditConfig.Spec.Filtering.Namespaces.Exclude = []string{s.testCluster.Settings.Namespace, "kube-system"}
	s.testMondooAuditConfigContainers(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_Nodes_CronJobs() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, true)
	s.testMondooAuditConfigNodesCronjobs(auditConfig)
}

func (s *AuditConfigSuite) TearDownSuite() {
	s.AuditConfigBaseSuite.TearDownSuite()
}

func TestAuditConfigSuite(t *testing.T) {
	s := new(AuditConfigSuite)
	defer func(s *AuditConfigSuite) {
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
