// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.mondoo.com/cnquery/v12/providers-sdk/v1/inventory"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/yaml"

	"go.mondoo.com/mondoo-operator/pkg/annotations"
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
	// Namespaces to include in scanning. Empty means all namespaces.
	Namespaces []string
	// NamespacesExclude are namespaces to exclude from scanning.
	NamespacesExclude []string
	// ClusterUID is the unique identifier of the cluster.
	ClusterUID string
	// IntegrationMRN is the integration MRN for asset labeling.
	IntegrationMRN string
}

// Scanner executes cnspec scans on K8s resources.
type Scanner struct {
	config ScannerConfig
}

// NewScanner creates a new Scanner with the given configuration.
func NewScanner(config ScannerConfig) *Scanner {
	return &Scanner{config: config}
}

// K8sResourceIdentifier identifies a specific K8s resource.
type K8sResourceIdentifier struct {
	Type      string // e.g., "deployment", "pod"
	Namespace string // empty for cluster-scoped resources
	Name      string
}

// String returns the resource identifier in the format expected by cnspec's k8s-resources option.
// Format: type:namespace:name for namespaced, type:name for cluster-scoped
func (r K8sResourceIdentifier) String() string {
	if r.Namespace == "" {
		return fmt.Sprintf("%s:%s", r.Type, r.Name)
	}
	return fmt.Sprintf("%s:%s:%s", r.Type, r.Namespace, r.Name)
}

// ScanResources scans specific K8s resources using the K8s API connection.
// resources is a list of specific resource identifiers to scan.
func (s *Scanner) ScanResources(ctx context.Context, resources []K8sResourceIdentifier) error {
	if len(resources) == 0 {
		scannerLogger.V(1).Info("No resources to scan, skipping")
		return nil
	}

	// Generate inventory file
	inv, err := s.generateInventory(resources)
	if err != nil {
		return fmt.Errorf("failed to generate inventory: %w", err)
	}

	// Create temp file for inventory
	tempDir := os.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "mondoo-resource-watcher-inventory-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file for inventory: %w", err)
	}
	tempPath := tempFile.Name()

	// Ensure cleanup
	defer func() {
		if removeErr := os.Remove(tempPath); removeErr != nil {
			scannerLogger.V(1).Info("Failed to remove temp file", "path", tempPath, "error", removeErr)
		}
	}()

	// Write inventory to temp file
	if _, err := tempFile.Write(inv); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("failed to write inventory to temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	scannerLogger.Info("Scanning resources via K8s API", "resourceCount", len(resources), "inventoryPath", tempPath)

	// Build cnspec command using inventory file
	cnspecArgs := []string{
		"scan", "k8s",
		"--config", s.config.ConfigPath,
		"--inventory-file", tempPath,
		"--score-threshold", "0",
	}
	if s.config.APIProxy != "" {
		cnspecArgs = append(cnspecArgs, "--api-proxy", s.config.APIProxy)
	}
	// Add annotations as command-line arguments (sorted for deterministic ordering)
	cnspecArgs = append(cnspecArgs, annotations.AnnotationArgs(s.config.Annotations)...)

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

// generateInventory creates an inventory YAML for scanning specific resources via K8s API.
func (s *Scanner) generateInventory(resources []K8sResourceIdentifier) ([]byte, error) {
	// Build resource filter string for k8s-resources option
	// Format: type:namespace:name,type:namespace:name,...
	resourceFilters := make([]string, 0, len(resources))
	for _, r := range resources {
		resourceFilters = append(resourceFilters, r.String())
	}

	// Extract unique resource types for discovery targets
	typeSet := make(map[string]struct{})
	for _, r := range resources {
		typeSet[r.Type+"s"] = struct{}{} // cnspec uses plural form (e.g., "deployments")
	}
	targets := make([]string, 0, len(typeSet))
	for t := range typeSet {
		targets = append(targets, t)
	}

	inv := &inventory.Inventory{
		Metadata: &inventory.ObjectMeta{
			Name: "mondoo-resource-watcher-inventory",
		},
		Spec: &inventory.InventorySpec{
			Assets: []*inventory.Asset{
				{
					Connections: []*inventory.Config{
						{
							Type: "k8s",
							Options: map[string]string{
								"namespaces":         strings.Join(s.config.Namespaces, ","),
								"namespaces-exclude": strings.Join(s.config.NamespacesExclude, ","),
								"k8s-resources":      strings.Join(resourceFilters, ","),
							},
							Discover: &inventory.Discovery{
								Targets: targets,
							},
						},
					},
					Labels: map[string]string{
						"k8s.mondoo.com/kind": "cluster",
					},
					ManagedBy: "mondoo-operator-" + s.config.ClusterUID,
				},
			},
		},
	}

	if s.config.IntegrationMRN != "" {
		for i := range inv.Spec.Assets {
			inv.Spec.Assets[i].Labels["mondoo.com/integration-mrn"] = s.config.IntegrationMRN
		}
	}

	return yaml.Marshal(inv)
}

// ScanResourcesFunc returns a function suitable for use with the Debouncer.
func (s *Scanner) ScanResourcesFunc() func(ctx context.Context, resources []K8sResourceIdentifier) error {
	return s.ScanResources
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
