// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

var scannerLogger = ctrl.Log.WithName("resource-watcher-scanner")

// ScannerConfig holds configuration for the cnspec scanner.
type ScannerConfig struct {
	// ConfigPath is the path to the mondoo.yml config file containing service account credentials.
	ConfigPath string
	// APIProxy is the HTTP proxy to use for API requests (optional).
	APIProxy string
	// Timeout is the timeout for scan operations.
	Timeout time.Duration
	// Annotations are key-value pairs to attach to all scanned assets.
	Annotations map[string]string
}

// Scanner executes cnspec scans on K8s manifests.
type Scanner struct {
	config ScannerConfig
}

// NewScanner creates a new Scanner with the given configuration.
func NewScanner(config ScannerConfig) *Scanner {
	return &Scanner{config: config}
}

// ScanManifests writes the given manifests to a temporary file and executes cnspec scan.
func (s *Scanner) ScanManifests(ctx context.Context, manifests []byte) error {
	if len(manifests) == 0 {
		scannerLogger.V(1).Info("No manifests to scan, skipping")
		return nil
	}

	// Create temp file for manifests
	tempDir := os.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "mondoo-resource-watcher-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file for manifests: %w", err)
	}
	tempPath := tempFile.Name()

	// Ensure cleanup
	defer func() {
		if removeErr := os.Remove(tempPath); removeErr != nil {
			scannerLogger.V(1).Info("Failed to remove temp file", "path", tempPath, "error", removeErr)
		}
	}()

	// Write manifests to temp file
	if _, err := tempFile.Write(manifests); err != nil {
		_ = tempFile.Close() // Ignore close error since we're already returning an error
		return fmt.Errorf("failed to write manifests to temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	scannerLogger.Info("Scanning manifests", "path", tempPath, "size", len(manifests))

	// Build cnspec command
	// cnspec scan k8s <manifest-file> --config <config-path>
	cnspecArgs := []string{
		"scan", "k8s", tempPath,
		"--config", s.config.ConfigPath,
		"--score-threshold", "0",
	}
	if s.config.APIProxy != "" {
		cnspecArgs = append(cnspecArgs, "--api-proxy", s.config.APIProxy)
	}
	// Add annotations as command-line arguments (sorted for deterministic ordering)
	annotationKeys := make([]string, 0, len(s.config.Annotations))
	for k := range s.config.Annotations {
		annotationKeys = append(annotationKeys, k)
	}
	sort.Strings(annotationKeys)
	for _, key := range annotationKeys {
		cnspecArgs = append(cnspecArgs, "--annotation", fmt.Sprintf("%s=%s", key, s.config.Annotations[key]))
	}

	// Create context with timeout
	scanCtx := ctx
	if s.config.Timeout > 0 {
		var cancel context.CancelFunc
		scanCtx, cancel = context.WithTimeout(ctx, s.config.Timeout)
		defer cancel()
	}

	// Execute cnspec
	cmd := exec.CommandContext(scanCtx, "cnspec", cnspecArgs...) //nolint:gosec // cnspec is a trusted binary
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "MONDOO_AUTO_UPDATE=false")

	scannerLogger.V(1).Info("Executing cnspec scan", "args", cnspecArgs)

	if err := cmd.Run(); err != nil {
		// Check if it was a timeout
		if scanCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("cnspec scan timed out after %v: %w", s.config.Timeout, err)
		}
		return fmt.Errorf("cnspec scan failed: %w", err)
	}

	scannerLogger.Info("Scan completed successfully")
	return nil
}

// ScanManifestsFunc returns a function suitable for use with the Debouncer.
func (s *Scanner) ScanManifestsFunc() func(ctx context.Context, manifests []byte) error {
	return s.ScanManifests
}

// ValidateConfig checks if the scanner configuration is valid.
func (s *Scanner) ValidateConfig() error {
	if s.config.ConfigPath == "" {
		return fmt.Errorf("config path is required")
	}

	// Check if config file exists
	if _, err := os.Stat(s.config.ConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", s.config.ConfigPath)
	}

	// Check if cnspec is available
	if _, err := exec.LookPath("cnspec"); err != nil {
		return fmt.Errorf("cnspec not found in PATH: %w", err)
	}

	return nil
}

// GetConfigPath returns the configured path to mondoo.yml.
func (s *Scanner) GetConfigPath() string {
	return s.config.ConfigPath
}
