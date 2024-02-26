// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	mondoov2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers/nodes"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AuditConfigOOMSuite struct {
	AuditConfigBaseSuite
}

func (s *AuditConfigOOMSuite) TestOOMControllerReporting() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, false, false)
	s.auditConfig = auditConfig

	// Disable container image resolution to be able to run the k8s resources scan CronJob with a local image.
	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	zap.S().Info("Create an audit config that enables nothing.")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	s.Require().True(s.testCluster.K8sHelper.WaitUntilMondooClientSecretExists(s.ctx, s.auditConfig.Namespace), "Mondoo SA not created")

	deployments := &appsv1.DeploymentList{}
	listOpts := &client.ListOptions{
		Namespace: auditConfig.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app.kubernetes.io/name": "mondoo-operator",
		}),
	}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, listOpts))
	s.Equalf(1, len(deployments.Items), "mondoo-operator deployment not found")

	operatorDeployment := deployments.Items[0]
	operatorDeployment.Spec.Template.Spec.Containers[0].Resources.Requests = corev1.ResourceList{}
	operatorDeployment.Spec.Template.Spec.Containers[0].Resources.Limits = corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("15Mi"), // this should be low enough to trigger an OOMkilled
	}

	zap.S().Info("Reducing memory limit to trigger OOM.")
	s.NoError(s.testCluster.K8sHelper.Clientset.Update(s.ctx, &operatorDeployment))

	// This will take some time, because:
	// a new replicaset should be created
	// the first Pod tries to start and gets killed
	// on the 2nd start we should get an OOMkilled status update
	err := s.testCluster.K8sHelper.CheckForDegradedCondition(&auditConfig, mondoov2.MondooOperatorDegraded, corev1.ConditionTrue)
	s.Require().NoError(err, "Failed to find degraded condition")

	foundMondooAuditConfig, err := s.testCluster.K8sHelper.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
	s.NoError(err, "Failed to find MondooAuditConfig")
	cond := mondoo.FindMondooAuditConditions(foundMondooAuditConfig.Status.Conditions, mondoov2.MondooOperatorDegraded)
	s.Require().NotNil(cond)
	s.Containsf(cond.Message, "OOM", "Failed to find OOMKilled message in degraded condition")
	s.Len(cond.AffectedPods, 1, "Failed to find only one pod in degraded condition")

	// Give the integration a chance to update
	err = s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		status, err := s.integration.GetStatus(s.ctx)
		if err != nil {
			return false, err
		}
		return status == "ERROR", nil
	})
	s.NoErrorf(err, "Failed to check for ERROR status")

	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, deployments, listOpts))
	s.Equalf(1, len(deployments.Items), "mondoo-operator deployment not found")

	operatorDeployment = deployments.Items[0]
	operatorDeployment.Spec.Template.Spec.Containers[0].Resources.Limits = corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("100Mi"), // this should be enough to get the operator running again
	}

	zap.S().Info("Increasing memory limit to get controller running again.")
	s.NoError(s.testCluster.K8sHelper.Clientset.Update(s.ctx, &operatorDeployment))

	err = s.testCluster.K8sHelper.CheckForDegradedCondition(&auditConfig, mondoov2.MondooOperatorDegraded, corev1.ConditionFalse)
	s.Require().NoError(err, "Failed to find degraded condition")
	foundMondooAuditConfig, err = s.testCluster.K8sHelper.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
	s.NoError(err, "Failed to find MondooAuditConfig")
	cond = mondoo.FindMondooAuditConditions(foundMondooAuditConfig.Status.Conditions, mondoov2.MondooOperatorDegraded)
	s.Require().NotNil(cond)
	s.NotContains(cond.Message, "OOM", "Found OOMKilled message in condition")
	s.Len(cond.AffectedPods, 0, "Found a pod in condition")

	err = s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		status, err := s.integration.GetStatus(s.ctx)
		if err != nil {
			return false, err
		}
		return status == "ACTIVE", nil
	})
	s.NoErrorf(err, "Failed to check for ACTIVE status")
}

func (s *AuditConfigOOMSuite) TestOOMScanAPI() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, true, false, false, false)
	s.auditConfig = auditConfig

	auditConfig.Spec.Scanner.Resources.Limits = corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("10Mi"), // this should be low enough to trigger an OOMkilled
	}

	// Move CronJob into the future to avoid interference with tests.
	cronStart := time.Now().Add(30 * time.Minute)
	auditConfig.Spec.KubernetesResources.Schedule = fmt.Sprintf("%d * * * *", cronStart.Minute())

	// Disable container image resolution to be able to run the k8s resources scan CronJob with a local image.
	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	zap.S().Info("Create an audit config that enables only workloads scanning. (with reduced memory limit)")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	s.Require().True(s.testCluster.K8sHelper.WaitUntilMondooClientSecretExists(s.ctx, s.auditConfig.Namespace), "Mondoo SA not created")

	// This will take some time, because:
	// reconcile needs to happen
	err := s.testCluster.K8sHelper.CheckForDegradedCondition(&auditConfig, mondoov2.ScanAPIDegraded, corev1.ConditionTrue)
	s.Require().NoError(err, "Failed to find degraded condition")

	foundMondooAuditConfig, err := s.testCluster.K8sHelper.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
	s.NoError(err, "Failed to find MondooAuditConfig")

	cond := mondoo.FindMondooAuditConditions(foundMondooAuditConfig.Status.Conditions, mondoov2.ScanAPIDegraded)
	s.Require().NotNil(cond)
	s.Containsf(cond.Message, "OOM", "Failed to find OOMKilled message in degraded condition")
	s.Len(cond.AffectedPods, 1, "Failed to find only one pod in degraded condition")

	// Give the integration a chance to update
	time.Sleep(2 * time.Second)

	status, err := s.integration.GetStatus(s.ctx)
	s.NoError(err, "Failed to get status")
	s.Equal("ERROR", status)

	err = s.testCluster.K8sHelper.UpdateAuditConfigWithRetries(auditConfig.Name, auditConfig.Namespace, func(config *mondoov2.MondooAuditConfig) {
		config.Spec.Scanner.Resources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("200Mi"), // this should be enough to get the ScanAPI running again
		}
	})
	s.Require().NoError(err)

	err = s.testCluster.K8sHelper.CheckForDegradedCondition(&auditConfig, mondoov2.ScanAPIDegraded, corev1.ConditionFalse)
	s.Require().NoError(err, "Failed to find degraded condition")
	foundMondooAuditConfig, err = s.testCluster.K8sHelper.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
	s.NoError(err, "Failed to find MondooAuditConfig")

	cond = mondoo.FindMondooAuditConditions(foundMondooAuditConfig.Status.Conditions, mondoov2.ScanAPIDegraded)
	s.Require().NotNil(cond)
	s.NotContains(cond.Message, "OOM", "Found OOMKilled message in condition")
	s.Len(cond.AffectedPods, 0, "Found a pod in condition")

	// Give the integration a chance to update
	time.Sleep(2 * time.Second)

	status, err = s.integration.GetStatus(s.ctx)
	s.NoError(err, "Failed to get status")
	s.Equal("ACTIVE", status)
}

func (s *AuditConfigOOMSuite) TestOOMNodeScan() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, false, false, true, false)
	s.auditConfig = auditConfig

	auditConfig.Spec.Nodes.Resources.Limits = corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("10Mi"), // this should be low enough to trigger an OOMkilled
	}

	// Disable container image resolution to be able to run the k8s resources scan CronJob with a local image.
	cleanup := s.disableContainerImageResolution()
	defer cleanup()

	zap.S().Info("Create an audit config that enables only nodes scanning. (with reduced memory limit)")
	s.NoErrorf(
		s.testCluster.K8sHelper.Clientset.Create(s.ctx, &auditConfig),
		"Failed to create Mondoo audit config.")

	s.Require().True(s.testCluster.K8sHelper.WaitUntilMondooClientSecretExists(s.ctx, s.auditConfig.Namespace), "Mondoo SA not created")

	cronJobs := &batchv1.CronJobList{}
	cronJobLabels := nodes.CronJobLabels(auditConfig)

	// List only the CronJobs in the namespace of the MondooAuditConfig and only the ones that exactly match our labels.
	listOpts := &client.ListOptions{Namespace: auditConfig.Namespace, LabelSelector: labels.SelectorFromSet(cronJobLabels)}

	nodeList := &corev1.NodeList{}
	s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, nodeList))

	// Verify the amount of CronJobs created is equal to the amount of nodes
	err := s.testCluster.K8sHelper.ExecuteWithRetries(func() (bool, error) {
		s.NoError(s.testCluster.K8sHelper.Clientset.List(s.ctx, cronJobs, listOpts))
		if len(nodeList.Items) == len(cronJobs.Items) {
			return true, nil
		}
		return false, nil
	})
	s.NoErrorf(
		err,
		"The amount of node scanning CronJobs is not equal to the amount of cluster nodes. expected: %d; actual: %d",
		len(nodeList.Items), len(cronJobs.Items))

	// This will take some time, because:
	// reconcile needs to happen
	// a new replicaset should be created
	// the first Pod tries to start and gets killed
	// on the 2nd start we should get an OOMkilled status update
	err = s.testCluster.K8sHelper.CheckForDegradedCondition(&auditConfig, mondoov2.NodeScanningDegraded, corev1.ConditionTrue)
	s.Require().NoError(err, "Failed to find degraded condition")

	foundMondooAuditConfig, err := s.testCluster.K8sHelper.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
	s.NoError(err, "Failed to find MondooAuditConfig")
	cond := mondoo.FindMondooAuditConditions(foundMondooAuditConfig.Status.Conditions, mondoov2.NodeScanningDegraded)
	s.Require().NotNil(cond)
	s.Containsf(cond.Message, "OOM", "Failed to find OOMKilled message in degraded condition")
	s.Len(cond.AffectedPods, 1, "Failed to find only one pod in degraded condition")

	// Give the integration a chance to update
	time.Sleep(2 * time.Second)

	status, err := s.integration.GetStatus(s.ctx)
	s.NoError(err, "Failed to get status")
	s.Equal("ERROR", status)

	zap.S().Info("Increasing memory limit to get node Scans running again.")
	err = s.testCluster.K8sHelper.UpdateAuditConfigWithRetries(auditConfig.Name, auditConfig.Namespace, func(config *mondoov2.MondooAuditConfig) {
		config.Spec.Nodes.Resources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("200Mi"), // this should be enough to get the ScanAPI running again
		}
		foundMondooAuditConfig.Spec.Nodes.Schedule = "*/1 * * * *"
	})
	s.Require().NoError(err)

	// Wait for the next run of the CronJob
	time.Sleep(30 * time.Second)

	err = s.testCluster.K8sHelper.CheckForDegradedCondition(&auditConfig, mondoov2.NodeScanningDegraded, corev1.ConditionFalse)
	s.Require().NoError(err, "Failed to find degraded condition")
	foundMondooAuditConfig, err = s.testCluster.K8sHelper.GetMondooAuditConfigFromCluster(auditConfig.Name, auditConfig.Namespace)
	s.NoError(err, "Failed to find MondooAuditConfig")
	cond = mondoo.FindMondooAuditConditions(foundMondooAuditConfig.Status.Conditions, mondoov2.ScanAPIDegraded)
	s.Require().NotNil(cond)
	s.NotContains(cond.Message, "OOM", "Found OOMKilled message in condition")
	s.Len(cond.AffectedPods, 0, "Found a pod in condition")

	// Give the integration a chance to update
	time.Sleep(2 * time.Second)

	status, err = s.integration.GetStatus(s.ctx)
	s.NoError(err, "Failed to get status")
	s.Equal("ACTIVE", status)
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
