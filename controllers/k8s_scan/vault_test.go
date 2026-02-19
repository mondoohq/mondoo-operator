// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"strings"
	"testing"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVaultKubeconfigSecretName(t *testing.T) {
	tests := []struct {
		prefix      string
		clusterName string
		expected    string
	}{
		{"mondoo-client", "production", "mondoo-client-vault-kubeconfig-production"},
		{"my-config", "staging", "my-config-vault-kubeconfig-staging"},
	}

	for _, tt := range tests {
		t.Run(tt.prefix+"-"+tt.clusterName, func(t *testing.T) {
			got := VaultKubeconfigSecretName(tt.prefix, tt.clusterName)
			if got != tt.expected {
				t.Errorf("VaultKubeconfigSecretName(%q, %q) = %q, want %q", tt.prefix, tt.clusterName, got, tt.expected)
			}
		})
	}
}

func TestBuildVaultKubeconfig(t *testing.T) {
	tests := []struct {
		name         string
		server       string
		token        string
		targetCACert []byte
		expectCA     bool
		expectSkip   bool
	}{
		{
			name:         "without target CA cert",
			server:       "https://target.example.com:6443",
			token:        "my-token-123",
			targetCACert: nil,
			expectCA:     false,
			expectSkip:   true,
		},
		{
			name:         "with target CA cert",
			server:       "https://target.example.com:6443",
			token:        "my-token-456",
			targetCACert: []byte("-----BEGIN CERTIFICATE-----\ntest-ca\n-----END CERTIFICATE-----"),
			expectCA:     true,
			expectSkip:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeconfig := buildVaultKubeconfig(tt.server, tt.token, tt.targetCACert)

			if !strings.Contains(kubeconfig, tt.server) {
				t.Errorf("expected kubeconfig to contain server %q", tt.server)
			}
			if !strings.Contains(kubeconfig, tt.token) {
				t.Errorf("expected kubeconfig to contain token %q", tt.token)
			}

			if tt.expectCA {
				if !strings.Contains(kubeconfig, "certificate-authority-data:") {
					t.Error("expected kubeconfig to contain certificate-authority-data")
				}
				if strings.Contains(kubeconfig, "insecure-skip-tls-verify") {
					t.Error("expected kubeconfig to NOT contain insecure-skip-tls-verify when CA is provided")
				}
			}

			if tt.expectSkip {
				if !strings.Contains(kubeconfig, "insecure-skip-tls-verify: true") {
					t.Error("expected kubeconfig to contain insecure-skip-tls-verify: true")
				}
				if strings.Contains(kubeconfig, "certificate-authority-data") {
					t.Error("expected kubeconfig to NOT contain certificate-authority-data when no CA")
				}
			}

			// Verify basic kubeconfig structure
			if !strings.Contains(kubeconfig, "apiVersion: v1") {
				t.Error("expected kubeconfig to contain apiVersion")
			}
			if !strings.Contains(kubeconfig, "kind: Config") {
				t.Error("expected kubeconfig to contain kind: Config")
			}
			if !strings.Contains(kubeconfig, "current-context: default") {
				t.Error("expected kubeconfig to contain current-context")
			}
		})
	}
}

func TestVaultKubeconfigSecret(t *testing.T) {
	m := &mondoov1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "mondoo-operator",
		},
	}

	kubeconfig := "apiVersion: v1\nkind: Config\ntest: data"
	secret := VaultKubeconfigSecret(m, "vault-cluster", kubeconfig)

	// Verify name
	expectedName := VaultKubeconfigSecretName("test-config", "vault-cluster")
	if secret.Name != expectedName {
		t.Errorf("expected name %q, got %q", expectedName, secret.Name)
	}

	// Verify namespace
	if secret.Namespace != "mondoo-operator" {
		t.Errorf("expected namespace %q, got %q", "mondoo-operator", secret.Namespace)
	}

	// Verify labels
	if secret.Labels["cluster_name"] != "vault-cluster" {
		t.Errorf("expected cluster_name label %q, got %q", "vault-cluster", secret.Labels["cluster_name"])
	}
	if secret.Labels["app"] != "mondoo-k8s-scan" {
		t.Errorf("expected app label %q, got %q", "mondoo-k8s-scan", secret.Labels["app"])
	}

	// Verify data
	data, ok := secret.Data["kubeconfig"]
	if !ok {
		t.Fatal("expected secret to have 'kubeconfig' key")
	}
	if string(data) != kubeconfig {
		t.Errorf("expected kubeconfig data %q, got %q", kubeconfig, string(data))
	}
}
