// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package spire

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultNamespace is the default namespace for SPIRE installation
	DefaultNamespace = "spire"

	// DefaultTrustDomain is the default SPIFFE trust domain for tests
	DefaultTrustDomain = "test.mondoo.local"

	// DefaultSocketPath is the default path to the SPIRE agent socket
	DefaultSocketPath = "/run/spire/sockets/agent.sock"

	// HelmReleaseName is the name of the SPIRE Helm release
	HelmReleaseName = "spire"

	// HelmChartRepo is the SPIFFE Helm chart repository URL
	HelmChartRepo = "https://spiffe.github.io/helm-charts-hardened"

	// HelmChartName is the name of the SPIRE chart
	HelmChartName = "spire"
)

// Installer handles SPIRE installation and configuration for tests
type Installer struct {
	client      client.Client
	namespace   string
	trustDomain string
	isInstalled bool
}

// InstallerOption is a functional option for configuring the SPIRE installer
type InstallerOption func(*Installer)

// WithNamespace sets the namespace for SPIRE installation
func WithNamespace(ns string) InstallerOption {
	return func(i *Installer) {
		i.namespace = ns
	}
}

// WithTrustDomain sets the SPIFFE trust domain
func WithTrustDomain(domain string) InstallerOption {
	return func(i *Installer) {
		i.trustDomain = domain
	}
}

// NewInstaller creates a new SPIRE installer
func NewInstaller(kubeClient client.Client, opts ...InstallerOption) *Installer {
	i := &Installer{
		client:      kubeClient,
		namespace:   DefaultNamespace,
		trustDomain: DefaultTrustDomain,
	}

	for _, opt := range opts {
		opt(i)
	}

	return i
}

// Install deploys SPIRE server and agent using Helm
func (i *Installer) Install(ctx context.Context) error {
	zap.S().Info("Installing SPIRE via Helm...")

	// Add Helm repo
	if err := i.addHelmRepo(); err != nil {
		return fmt.Errorf("failed to add SPIRE Helm repo: %w", err)
	}

	// Create namespace
	if err := i.ensureNamespace(ctx); err != nil {
		return fmt.Errorf("failed to create SPIRE namespace: %w", err)
	}

	// Install SPIRE using Helm with custom values
	if err := i.installHelmChart(); err != nil {
		return fmt.Errorf("failed to install SPIRE Helm chart: %w", err)
	}

	i.isInstalled = true

	// Wait for components to be ready
	if err := i.WaitForAgentReady(ctx); err != nil {
		return fmt.Errorf("SPIRE agent failed to become ready: %w", err)
	}

	zap.S().Info("SPIRE installation completed successfully")
	return nil
}

// Uninstall removes SPIRE from the cluster
func (i *Installer) Uninstall(ctx context.Context) error {
	if !i.isInstalled {
		return nil
	}

	zap.S().Info("Uninstalling SPIRE...")

	// Uninstall main SPIRE chart
	cmd := exec.Command("helm", "uninstall", HelmReleaseName, "-n", i.namespace, "--wait") // #nosec G204
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore if release not found
		if !strings.Contains(string(output), "not found") {
			zap.S().Warnf("Failed to uninstall SPIRE: %v, output: %s", err, string(output))
		}
	}

	// Uninstall SPIRE CRDs chart
	cmd = exec.Command("helm", "uninstall", HelmReleaseName+"-crds", "-n", i.namespace, "--wait") // #nosec G204
	output, err = cmd.CombinedOutput()
	if err != nil {
		// Ignore if release not found
		if !strings.Contains(string(output), "not found") {
			zap.S().Warnf("Failed to uninstall SPIRE CRDs: %v, output: %s", err, string(output))
		}
	}

	// Delete namespace
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: i.namespace}}
	if err := i.client.Delete(ctx, ns); err != nil && !errors.IsNotFound(err) {
		zap.S().Warnf("Failed to delete SPIRE namespace: %v", err)
	}

	i.isInstalled = false
	zap.S().Info("SPIRE uninstalled")
	return nil
}

// RegisterWorkload creates a workload entry in SPIRE for the given service account
func (i *Installer) RegisterWorkload(ctx context.Context, spiffeID, namespace, serviceAccount string) error {
	zap.S().Infof("Registering SPIRE workload: %s for %s/%s", spiffeID, namespace, serviceAccount)

	// Get SPIRE server pod name
	serverPod, err := i.getSpireServerPod(ctx)
	if err != nil {
		return fmt.Errorf("failed to get SPIRE server pod: %w", err)
	}

	// Get the node attestor parent ID
	// For Kubernetes workloads, the parent ID is typically the node's SPIFFE ID
	parentID := fmt.Sprintf("spiffe://%s/spire/agent/k8s_psat/default", i.trustDomain)

	// Create workload entry using kubectl exec
	cmd := exec.Command("kubectl", "exec", "-n", i.namespace, serverPod, "-c", "spire-server", "--", // #nosec G204
		"/opt/spire/bin/spire-server", "entry", "create",
		"-spiffeID", spiffeID,
		"-parentID", parentID,
		"-selector", fmt.Sprintf("k8s:ns:%s", namespace),
		"-selector", fmt.Sprintf("k8s:sa:%s", serviceAccount),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if entry already exists
		if strings.Contains(string(output), "similar entry already exists") {
			zap.S().Infof("Workload entry already exists: %s", spiffeID)
			return nil
		}
		return fmt.Errorf("failed to register workload: %v, output: %s", err, string(output))
	}

	zap.S().Infof("Workload registered successfully: %s", spiffeID)
	return nil
}

// WaitForAgentReady waits for the SPIRE agent DaemonSet to be ready
func (i *Installer) WaitForAgentReady(ctx context.Context) error {
	zap.S().Info("Waiting for SPIRE agent to be ready...")

	timeout := time.After(3 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for SPIRE agent to be ready")
		case <-ticker.C:
			ds := &appsv1.DaemonSet{}
			// The SPIRE chart creates the agent DaemonSet with name spire-agent
			if err := i.client.Get(ctx, types.NamespacedName{
				Name:      "spire-agent",
				Namespace: i.namespace,
			}, ds); err != nil {
				if errors.IsNotFound(err) {
					zap.S().Debug("SPIRE agent DaemonSet not found yet")
					continue
				}
				return err
			}

			if ds.Status.NumberReady > 0 && ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
				zap.S().Infof("SPIRE agent is ready (%d/%d)", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
				return nil
			}
			zap.S().Debugf("SPIRE agent not ready: %d/%d", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
		}
	}
}

// GetAgentSocketPath returns the path to the SPIRE agent socket
func (i *Installer) GetAgentSocketPath() string {
	return DefaultSocketPath
}

// GetTrustDomain returns the configured trust domain
func (i *Installer) GetTrustDomain() string {
	return i.trustDomain
}

// GetNamespace returns the SPIRE namespace
func (i *Installer) GetNamespace() string {
	return i.namespace
}

// GetSPIFFEID generates a SPIFFE ID for the given namespace and service account
func (i *Installer) GetSPIFFEID(namespace, serviceAccount string) string {
	return fmt.Sprintf("spiffe://%s/ns/%s/sa/%s", i.trustDomain, namespace, serviceAccount)
}

func (i *Installer) addHelmRepo() error {
	cmd := exec.Command("helm", "repo", "add", "spiffe", HelmChartRepo)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if !strings.Contains(string(output), "already exists") {
			return fmt.Errorf("failed to add Helm repo: %v, output: %s", err, string(output))
		}
	}

	cmd = exec.Command("helm", "repo", "update", "spiffe")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update Helm repo: %v, output: %s", err, string(output))
	}

	return nil
}

func (i *Installer) ensureNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: i.namespace,
		},
	}

	if err := i.client.Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (i *Installer) installHelmChart() error {
	// First, install the SPIRE CRDs chart (required before main chart)
	zap.S().Info("Installing SPIRE CRDs...")
	crdArgs := []string{
		"upgrade", "--install", HelmReleaseName + "-crds",
		"spiffe/spire-crds",
		"-n", i.namespace,
		"--create-namespace",
		"--wait",
		"--timeout", "2m",
	}

	cmd := exec.Command("helm", crdArgs...) // #nosec G204
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install SPIRE CRDs chart: %v, output: %s", err, string(output))
	}
	zap.S().Info("SPIRE CRDs installed successfully")

	// Install SPIRE with configuration suitable for testing
	// Using the spiffe/spire chart which includes both server and agent
	args := []string{
		"upgrade", "--install", HelmReleaseName,
		"spiffe/spire",
		"-n", i.namespace,
		"--create-namespace",
		"--wait",
		"--timeout", "5m",
		// Set trust domain (global.spire.trustDomain is the correct path)
		"--set", fmt.Sprintf("global.spire.trustDomain=%s", i.trustDomain),
		// Enable SPIRE server
		"--set", "spire-server.enabled=true",
		// Enable SPIRE agent
		"--set", "spire-agent.enabled=true",
		// Configure for k3d testing
		"--set", "spire-agent.hostPathSocket=/run/spire/sockets",
		// Use NodePort for simplicity in testing
		"--set", "spire-server.service.type=ClusterIP",
		// Disable federation for simplicity
		"--set", "spire-server.federation.enabled=false",
		// Disable tornjak (UI) for simpler setup
		"--set", "tornjak-frontend.enabled=false",
		// Set reasonable resource limits for testing
		"--set", "spire-server.resources.requests.cpu=100m",
		"--set", "spire-server.resources.requests.memory=128Mi",
		"--set", "spire-agent.resources.requests.cpu=50m",
		"--set", "spire-agent.resources.requests.memory=64Mi",
	}

	cmd = exec.Command("helm", args...) // #nosec G204
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install SPIRE chart: %v, output: %s", err, string(output))
	}

	zap.S().Infof("SPIRE Helm chart installed successfully")
	return nil
}

func (i *Installer) getSpireServerPod(ctx context.Context) (string, error) {
	pods := &corev1.PodList{}
	if err := i.client.List(ctx, pods, client.InNamespace(i.namespace), client.MatchingLabels{
		"app.kubernetes.io/name": "server",
	}); err != nil {
		return "", err
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return pod.Name, nil
		}
	}

	// Try alternative label selector for different chart versions
	if err := i.client.List(ctx, pods, client.InNamespace(i.namespace), client.MatchingLabels{
		"app": "spire-server",
	}); err != nil {
		return "", err
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return pod.Name, nil
		}
	}

	return "", fmt.Errorf("no running SPIRE server pod found")
}

// CreateSPIFFERBAC creates RBAC resources on the target cluster to allow SPIFFE-authenticated users
func CreateSPIFFERBAC(ctx context.Context, kubeClient client.Client, spiffeID, roleName string) error {
	// Create ClusterRoleBinding that allows the SPIFFE identity to view cluster resources
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "spiffe-" + roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "User",
				Name: spiffeID,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "view", // Use the built-in view ClusterRole
		},
	}

	if err := kubeClient.Create(ctx, crb); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create SPIFFE ClusterRoleBinding: %w", err)
	}

	zap.S().Infof("Created SPIFFE RBAC for identity: %s", spiffeID)
	return nil
}

// CreateTrustBundleSecret creates a secret containing the CA certificate for the target cluster
func CreateTrustBundleSecret(ctx context.Context, kubeClient client.Client, secretName, namespace string, caCert []byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt": caCert,
		},
	}

	if err := kubeClient.Create(ctx, secret); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create trust bundle secret: %w", err)
	}

	zap.S().Infof("Created trust bundle secret: %s/%s", namespace, secretName)
	return nil
}
