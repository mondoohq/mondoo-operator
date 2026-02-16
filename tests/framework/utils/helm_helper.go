// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package utils

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
)

const helmCmd = "helm"

// HelmHelper provides utilities for Helm operations in tests
type HelmHelper struct {
	executor *CommandExecutor
}

// NewHelmHelper creates a new HelmHelper instance
func NewHelmHelper() *HelmHelper {
	return &HelmHelper{
		executor: &CommandExecutor{},
	}
}

// Install installs a Helm chart
func (h *HelmHelper) Install(releaseName, chartPath, namespace string, values map[string]string) (string, error) {
	args := []string{
		"install",
		releaseName,
		chartPath,
		"--namespace", namespace,
		"--create-namespace",
		"--atomic",
		"--timeout", "5m",
	}

	for k, v := range values {
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, v))
	}

	zap.S().Infof("Installing Helm chart: %s %s", helmCmd, strings.Join(args, " "))
	return h.executor.ExecuteCommandWithOutput(helmCmd, args...)
}

// Upgrade upgrades a Helm release
func (h *HelmHelper) Upgrade(releaseName, chartPath, namespace string, values map[string]string) (string, error) {
	args := []string{
		"upgrade",
		releaseName,
		chartPath,
		"--namespace", namespace,
		"--wait",
		"--timeout", "5m",
	}

	for k, v := range values {
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, v))
	}

	zap.S().Infof("Upgrading Helm chart: %s %s", helmCmd, strings.Join(args, " "))
	return h.executor.ExecuteCommandWithOutput(helmCmd, args...)
}

// Uninstall uninstalls a Helm release
func (h *HelmHelper) Uninstall(releaseName, namespace string) (string, error) {
	args := []string{
		"uninstall",
		releaseName,
		"--namespace", namespace,
		"--wait",
		"--timeout", "5m",
	}

	zap.S().Infof("Uninstalling Helm release: %s %s", helmCmd, strings.Join(args, " "))
	return h.executor.ExecuteCommandWithOutput(helmCmd, args...)
}

// Template renders Helm templates without installing
func (h *HelmHelper) Template(releaseName, chartPath, namespace string, values map[string]string) (string, error) {
	args := []string{
		"template",
		releaseName,
		chartPath,
		"--namespace", namespace,
		"--include-crds",
	}

	for k, v := range values {
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, v))
	}

	return h.executor.ExecuteCommandWithOutput(helmCmd, args...)
}

// Lint lints a Helm chart
func (h *HelmHelper) Lint(chartPath string) (string, error) {
	args := []string{
		"lint",
		chartPath,
	}

	zap.S().Infof("Linting Helm chart: %s %s", helmCmd, strings.Join(args, " "))
	return h.executor.ExecuteCommandWithOutput(helmCmd, args...)
}

// List lists Helm releases
func (h *HelmHelper) List(namespace string) (string, error) {
	args := []string{
		"list",
		"--namespace", namespace,
		"-o", "json",
	}

	return h.executor.ExecuteCommandWithOutput(helmCmd, args...)
}

// GetReleaseStatus gets the status of a Helm release
func (h *HelmHelper) GetReleaseStatus(releaseName, namespace string) (string, error) {
	args := []string{
		"status",
		releaseName,
		"--namespace", namespace,
		"-o", "json",
	}

	return h.executor.ExecuteCommandWithOutput(helmCmd, args...)
}
