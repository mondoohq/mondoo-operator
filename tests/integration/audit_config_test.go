// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"
	"k8s.io/utils/ptr"
)

type AuditConfigSuite struct {
	AuditConfigBaseSuite
}

func (s *AuditConfigSuite) SetupSuite() {
	s.AuditConfigBaseSuite.SetupSuite()
	s.testCluster.MondooInstaller.Settings.SuiteName = "AuditConfigSuite"
}

func (s *AuditConfigSuite) TestReconcile_AllDisabled() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, false, false)
	s.testMondooAuditConfigAllDisabled(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_KubernetesResources() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, true, false, false, false)
	s.testMondooAuditConfigKubernetesResources(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_Containers() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, true, false, false)

	// Ignore the operator namespace because we cannot scan a local image
	// Ignore kube-system to speed up the containers test
	auditConfig.Spec.Filtering.Namespaces.Exclude = []string{s.testCluster.Settings.Namespace, "kube-system"}
	s.testMondooAuditConfigContainers(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_Nodes_CronJobs() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, true, false)
	s.testMondooAuditConfigNodesCronjobs(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_Nodes_DaemonSet() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, true, false)
	auditConfig.Spec.Nodes.Style = v1alpha2.NodeScanStyle_Deployment // TODO: Change to DaemonSet (no effect on reconsile logic)
	auditConfig.Spec.Nodes.IntervalTimer = 1
	s.testMondooAuditConfigNodesDaemonSets(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_AdmissionPermissive() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, false, true)
	s.testMondooAuditConfigAdmission(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_AdmissionEnforcing() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, false, true)
	auditConfig.Spec.Admission.Mode = v1alpha2.Enforcing
	s.testMondooAuditConfigAdmission(auditConfig)
}

func (s *AuditConfigSuite) TestReconcile_AdmissionEnforcingScaleDownScanApi() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, false, true)
	auditConfig.Spec.Admission.Mode = v1alpha2.Enforcing
	auditConfig.Spec.Admission.Replicas = ptr.To(int32(1))
	auditConfig.Spec.Scanner.Replicas = ptr.To(int32(1))
	s.testMondooAuditConfigAdmissionScaleDownScanApi(auditConfig)
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
