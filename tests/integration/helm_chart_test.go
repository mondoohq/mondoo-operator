// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"go.mondoo.com/mondoo-operator/tests/framework/utils"
)

const (
	helmReleaseName = "mondoo-operator"
	helmChartPath   = "charts/mondoo-operator"
)

// HelmChartSuite tests the Helm chart installation and functionality
type HelmChartSuite struct {
	suite.Suite
	ctx        context.Context
	k8sHelper  *utils.K8sHelper
	helmHelper *utils.HelmHelper
	namespace  string
	rootFolder string
}

func (s *HelmChartSuite) SetupSuite() {
	s.ctx = context.Background()

	var err error
	s.k8sHelper, err = utils.CreateK8sHelper()
	s.Require().NoError(err, "Failed to create K8s helper")

	s.helmHelper = utils.NewHelmHelper()

	s.rootFolder, err = utils.FindRootFolder()
	s.Require().NoError(err, "Failed to find root folder")

	// Use a unique namespace for Helm tests
	s.namespace = fmt.Sprintf("mondoo-helm-test-%d", time.Now().Unix())

	zap.S().Infof("Starting Helm chart test suite in namespace %s", s.namespace)
}

func (s *HelmChartSuite) TearDownSuite() {
	zap.S().Info("Tearing down Helm chart test suite")

	// Uninstall the Helm release if it exists
	_, err := s.helmHelper.Uninstall(helmReleaseName, s.namespace)
	if err != nil {
		zap.S().Warnf("Failed to uninstall Helm release (may not exist): %v", err)
	}

	// Delete the test namespace
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: s.namespace}}
	if err := s.k8sHelper.DeleteResourceIfExists(ns); err != nil {
		zap.S().Warnf("Failed to delete namespace %s: %v", s.namespace, err)
	}
}

func (s *HelmChartSuite) TestHelmLint() {
	// Test that the Helm chart passes linting
	chartPath := filepath.Join(s.rootFolder, helmChartPath)
	output, err := s.helmHelper.Lint(chartPath)
	s.Require().NoError(err, "Helm lint failed: %s", output)
	zap.S().Infof("Helm lint output: %s", output)
	zap.S().Info("Helm lint passed successfully")
}

func (s *HelmChartSuite) TestHelmTemplate() {
	// Test that the Helm chart templates render correctly
	chartPath := filepath.Join(s.rootFolder, helmChartPath)
	output, err := s.helmHelper.Template("test-release", chartPath, "default", nil)
	s.Require().NoError(err, "Helm template failed: %s", output)

	// Verify key resources are present in the rendered output
	s.Contains(output, "kind: Deployment", "Should contain Deployment")
	s.Contains(output, "kind: ServiceAccount", "Should contain ServiceAccount")
	s.Contains(output, "kind: CustomResourceDefinition", "Should contain CRD")
	s.Contains(output, "mondooauditconfigs.k8s.mondoo.com", "Should contain MondooAuditConfig CRD")
	zap.S().Info("Helm template rendering passed successfully")
}

func (s *HelmChartSuite) TestHelmInstallAndUninstall() {
	chartPath := filepath.Join(s.rootFolder, helmChartPath)

	imageRepo := os.Getenv("MONDOO_OPERATOR_IMAGE_REPO")
	if imageRepo == "" {
		imageRepo = "ghcr.io/mondoohq/mondoo-operator"
	}
	imageTag := os.Getenv("MONDOO_OPERATOR_IMAGE_TAG")
	if imageTag == "" {
		imageTag = "latest"
	}
	values := map[string]string{
		"controllerManager.manager.image.repository": imageRepo,
		"controllerManager.manager.image.tag":        imageTag,
	}

	// Install the Helm chart
	zap.S().Infof("Installing Helm chart from %s", chartPath)
	output, err := s.helmHelper.Install(helmReleaseName, chartPath, s.namespace, values)
	s.Require().NoError(err, "Helm install failed: %s", output)
	zap.S().Info("Helm install completed successfully")

	// Verify the operator deployment is ready
	s.Eventually(func() bool {
		return s.k8sHelper.IsPodReady("app.kubernetes.io/name=mondoo-operator", s.namespace)
	}, 3*time.Minute, 5*time.Second, "Operator pod should become ready")
	zap.S().Info("Operator pod is ready")

	// Verify key resources were created
	s.verifyHelmResources()

	// Note: We skip creating a MondooAuditConfig in this basic test because:
	// 1. It requires a valid mondoo-client secret with credentials
	// 2. The operator reconciliation can have finalizer issues without valid creds
	// A full end-to-end test with MondooAuditConfig should be in a separate test
	// that sets up proper credentials.

	// Uninstall the Helm chart
	output, err = s.helmHelper.Uninstall(helmReleaseName, s.namespace)
	s.Require().NoError(err, "Helm uninstall failed: %s", output)
	zap.S().Info("Helm uninstall completed successfully")

	// Verify the operator deployment is gone
	s.Eventually(func() bool {
		pods, err := s.k8sHelper.ListPods(s.namespace, "app.kubernetes.io/name=mondoo-operator")
		return err == nil && len(pods.Items) == 0
	}, 2*time.Minute, 5*time.Second, "Operator pods should be removed after uninstall")
	zap.S().Info("Operator pods removed after uninstall")
}

func (s *HelmChartSuite) TestHelmPreDeleteHook() {
	chartPath := filepath.Join(s.rootFolder, helmChartPath)

	// Render the chart and verify pre-delete hook is present
	output, err := s.helmHelper.Template("test-release", chartPath, "default", map[string]string{
		"cleanup.enabled": "true",
	})
	s.Require().NoError(err, "Helm template failed")

	// Verify pre-delete hook resources are present
	// Note: YAML renders annotation keys with quotes when they contain special characters
	s.Contains(output, `"helm.sh/hook": pre-delete`, "Should contain pre-delete hook annotation")
	s.Contains(output, "mondoo-operator-cleanup", "Should contain cleanup job")
	s.Contains(output, "- cleanup", "Cleanup job should use cleanup command")
	zap.S().Info("Pre-delete hook template verification passed")
}

func (s *HelmChartSuite) TestHelmCleanupDisabled() {
	chartPath := filepath.Join(s.rootFolder, helmChartPath)

	// Render the chart with cleanup disabled
	output, err := s.helmHelper.Template("test-release", chartPath, "default", map[string]string{
		"cleanup.enabled": "false",
	})
	s.Require().NoError(err, "Helm template failed")

	// Verify pre-delete hook resources are NOT present
	s.NotContains(output, "mondoo-operator-cleanup", "Should not contain cleanup job when disabled")
	zap.S().Info("Cleanup disabled template verification passed")
}

func (s *HelmChartSuite) verifyHelmResources() {
	// Verify ServiceAccount exists
	sa := &corev1.ServiceAccount{}
	err := s.k8sHelper.Clientset.Get(s.ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-controller-manager", helmReleaseName),
		Namespace: s.namespace,
	}, sa)
	s.Require().NoError(err, "Controller manager ServiceAccount should exist")

	// Verify scanning ServiceAccount exists
	err = s.k8sHelper.Clientset.Get(s.ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-k8s-resources-scanning", helmReleaseName),
		Namespace: s.namespace,
	}, sa)
	s.Require().NoError(err, "K8s resources scanning ServiceAccount should exist")

	zap.S().Info("All expected Helm resources verified")
}

func TestHelmChartSuite(t *testing.T) {
	s := new(HelmChartSuite)
	defer func() {
		if r := recover(); r != nil {
			zap.S().Errorf("Test suite panicked: %v", r)
			// Attempt cleanup
			if s.helmHelper != nil && s.namespace != "" {
				_, _ = s.helmHelper.Uninstall(helmReleaseName, s.namespace)
			}
		}
	}()
	suite.Run(t, s)
}
