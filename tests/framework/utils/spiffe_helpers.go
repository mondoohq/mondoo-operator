// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package utils

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	mondoov2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultSPIFFETrustDomain is the default trust domain for testing
	DefaultSPIFFETrustDomain = "test.mondoo.local"
)

// CreateTrustBundleSecretFromCluster extracts the CA certificate from a target cluster
// and creates a secret in the management cluster
func CreateTrustBundleSecretFromCluster(ctx context.Context, kubeClient client.Client, targetClusterName, secretName, namespace string) error {
	// Get the CA certificate from the target cluster's kubeconfig
	cmd := exec.Command("k3d", "kubeconfig", "get", targetClusterName) // #nosec G204
	kubeconfigBytes, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get target cluster kubeconfig: %w", err)
	}

	// Extract CA from kubeconfig using yq
	cmd = exec.Command("yq", "-r", ".clusters[0].cluster.certificate-authority-data") // #nosec G204
	cmd.Stdin = strings.NewReader(string(kubeconfigBytes))
	caBase64, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to extract CA from kubeconfig: %w", err)
	}

	// Decode base64 CA
	cmd = exec.Command("base64", "-d")
	cmd.Stdin = strings.NewReader(strings.TrimSpace(string(caBase64)))
	caCert, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to decode CA certificate: %w", err)
	}

	// Create the secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt": caCert,
		},
	}

	existing := &corev1.Secret{}
	if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(secret), existing); err != nil {
		if errors.IsNotFound(err) {
			if err := kubeClient.Create(ctx, secret); err != nil {
				return fmt.Errorf("failed to create trust bundle secret: %w", err)
			}
			zap.S().Infof("Created trust bundle secret: %s/%s", namespace, secretName)
			return nil
		}
		return err
	}

	// Update if exists
	existing.Data = secret.Data
	if err := kubeClient.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update trust bundle secret: %w", err)
	}

	zap.S().Infof("Updated trust bundle secret: %s/%s", namespace, secretName)
	return nil
}

// CreateSPIFFERBACOnTargetCluster creates RBAC on the target cluster to allow SPIFFE-authenticated users
func CreateSPIFFERBACOnTargetCluster(targetClusterContext, spiffeID string) error {
	zap.S().Infof("Creating SPIFFE RBAC on target cluster for identity: %s", spiffeID)

	// Create ClusterRoleBinding YAML
	crbYAML := fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: spiffe-mondoo-scanner
subjects:
- kind: User
  name: "%s"
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: view
  apiGroup: rbac.authorization.k8s.io
`, spiffeID)

	// Apply using kubectl with the target cluster context
	cmd := exec.Command("kubectl", "--context", targetClusterContext, "apply", "-f", "-") // #nosec G204
	cmd.Stdin = strings.NewReader(crbYAML)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create SPIFFE RBAC: %v, output: %s", err, string(output))
	}

	zap.S().Infof("Created SPIFFE RBAC on target cluster")
	return nil
}

// DefaultAuditConfigWithSPIFFE creates a MondooAuditConfig with SPIFFE authentication for external cluster scanning
func DefaultAuditConfigWithSPIFFE(ns string, clusterName, serverURL, trustBundleSecretName, socketPath string) mondoov2.MondooAuditConfig {
	now := time.Now()
	startScan := now.Add(time.Minute).Add(time.Second * 15)
	schedule := fmt.Sprintf("%d * * * *", startScan.Minute())

	if socketPath == "" {
		socketPath = "/run/spire/sockets/agent.sock"
	}

	return mondoov2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: ns,
		},
		Spec: mondoov2.MondooAuditConfigSpec{
			ConsoleIntegration:   mondoov2.ConsoleIntegration{Enable: true},
			MondooCredsSecretRef: corev1.LocalObjectReference{Name: MondooClientSecret},
			MondooTokenSecretRef: corev1.LocalObjectReference{Name: MondooTokenSecret},
			KubernetesResources: mondoov2.KubernetesResources{
				Enable:   false, // Disable local cluster scanning
				Schedule: schedule,
				ExternalClusters: []mondoov2.ExternalCluster{
					{
						Name:     clusterName,
						Schedule: schedule,
						SPIFFEAuth: &mondoov2.SPIFFEAuthConfig{
							Server:               serverURL,
							SocketPath:           socketPath,
							TrustBundleSecretRef: corev1.LocalObjectReference{Name: trustBundleSecretName},
						},
					},
				},
			},
		},
	}
}

// CreateSPIFFERBACOnCluster creates RBAC resources on a cluster to allow SPIFFE-authenticated users
func CreateSPIFFERBACOnCluster(ctx context.Context, kubeClient client.Client, spiffeID string) error {
	// Create ClusterRoleBinding that allows the SPIFFE identity to view cluster resources
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "spiffe-mondoo-scanner",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				Name:     spiffeID,
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "view",
		},
	}

	if err := kubeClient.Create(ctx, crb); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create SPIFFE ClusterRoleBinding: %w", err)
	}

	zap.S().Infof("Created SPIFFE RBAC for identity: %s", spiffeID)
	return nil
}

// WaitForSPIRESocket waits for the SPIRE agent socket to be available on the node
func WaitForSPIRESocket(ctx context.Context, kubeClient client.Client, namespace, socketPath string) error {
	zap.S().Info("Waiting for SPIRE agent socket to be available...")

	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for SPIRE agent socket")
		case <-ticker.C:
			// Create a test pod that checks if the socket exists
			if checkSPIRESocket(namespace, socketPath) {
				zap.S().Info("SPIRE agent socket is available")
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func checkSPIRESocket(namespace, socketPath string) bool {
	// Use a simple kubectl exec to check if socket is available
	// This is a basic check; in production you might want a more robust verification
	cmd := exec.Command("kubectl", "run", "socket-check", "-n", namespace, // #nosec G204
		"--rm", "-i", "--restart=Never",
		"--image=busybox:1.36",
		"--", "ls", "-la", socketPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		zap.S().Debugf("SPIRE socket check failed: %v, output: %s", err, string(output))
		return false
	}
	return true
}

// GetTargetClusterCAFromK3d extracts the CA certificate from a k3d cluster
func GetTargetClusterCAFromK3d(clusterName string) ([]byte, error) {
	cmd := exec.Command("k3d", "kubeconfig", "get", clusterName) // #nosec G204
	kubeconfigBytes, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get k3d kubeconfig: %w", err)
	}

	// Extract CA using yq
	cmd = exec.Command("yq", "-r", ".clusters[0].cluster.certificate-authority-data") // #nosec G204
	cmd.Stdin = strings.NewReader(string(kubeconfigBytes))
	caBase64, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to extract CA: %w", err)
	}

	// Decode base64
	cmd = exec.Command("base64", "-d")
	cmd.Stdin = strings.NewReader(strings.TrimSpace(string(caBase64)))
	caCert, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to decode CA: %w", err)
	}

	return caCert, nil
}
