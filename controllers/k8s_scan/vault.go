// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	vault "github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VaultTokenFetcher is an injectable function that fetches a service account token
// from Vault's Kubernetes secrets engine. It follows the same pattern as MondooClientBuilder
// for testability.
type VaultTokenFetcher func(ctx context.Context, saToken string, config v1alpha2.VaultAuthConfig, vaultCACert []byte) (string, error)

// DefaultVaultTokenFetcher is the production implementation that uses vault-client-go.
func DefaultVaultTokenFetcher(ctx context.Context, saToken string, config v1alpha2.VaultAuthConfig, vaultCACert []byte) (string, error) {
	// Build client options
	opts := []vault.ClientOption{
		vault.WithAddress(config.VaultAddr),
	}
	if len(vaultCACert) > 0 {
		opts = append(opts, vault.WithTLS(vault.TLSConfiguration{
			ServerCertificate: vault.ServerCertificateEntry{
				FromBytes: vaultCACert,
			},
		}))
	}

	client, err := vault.New(opts...)
	if err != nil {
		return "", fmt.Errorf("failed to create Vault client: %w", err)
	}

	// Authenticate to Vault using Kubernetes auth
	authPath := config.AuthPath
	if authPath == "" {
		authPath = "auth/kubernetes"
	}
	// The library prepends "auth/" automatically, so strip it if present
	authMount := strings.TrimPrefix(authPath, "auth/")

	loginResp, err := client.Auth.KubernetesLogin(ctx, schema.KubernetesLoginRequest{
		Jwt:  saToken,
		Role: config.AuthRole,
	}, vault.WithMountPath(authMount))
	if err != nil {
		return "", fmt.Errorf("vault Kubernetes auth login failed: %w", err)
	}
	if loginResp == nil || loginResp.Auth == nil {
		return "", fmt.Errorf("vault Kubernetes auth login returned empty response")
	}

	if err := client.SetToken(loginResp.Auth.ClientToken); err != nil {
		return "", fmt.Errorf("failed to set Vault token: %w", err)
	}

	// Request credentials from Vault's Kubernetes secrets engine
	secretsPath := config.SecretsPath
	if secretsPath == "" {
		secretsPath = "kubernetes"
	}

	credsReq := schema.KubernetesGenerateCredentialsRequest{
		KubernetesNamespace: config.KubernetesNamespace,
	}
	if config.TTL != "" {
		credsReq.Ttl = config.TTL
	}

	credResp, err := client.Secrets.KubernetesGenerateCredentials(ctx, config.CredsRole, credsReq, vault.WithMountPath(secretsPath))
	if err != nil {
		return "", fmt.Errorf("failed to generate Vault Kubernetes credentials: %w", err)
	}
	if credResp == nil || credResp.Data == nil {
		return "", fmt.Errorf("vault returned empty credentials response")
	}

	token, ok := credResp.Data["service_account_token"].(string)
	if !ok || token == "" {
		return "", fmt.Errorf("vault response missing service_account_token")
	}

	return token, nil
}

// VaultKubeconfigSecretName returns the name for the Vault kubeconfig Secret.
func VaultKubeconfigSecretName(prefix, clusterName string) string {
	return fmt.Sprintf("%s-vault-kubeconfig-%s", prefix, clusterName)
}

// buildVaultKubeconfig generates a kubeconfig YAML given a server URL, bearer token,
// and optional target cluster CA certificate bytes.
func buildVaultKubeconfig(server, token string, targetCACert []byte) string {
	var clusterConfig string
	if len(targetCACert) > 0 {
		encoded := base64.StdEncoding.EncodeToString(targetCACert)
		clusterConfig = fmt.Sprintf(`    certificate-authority-data: %s
    server: %s`, encoded, server)
	} else {
		clusterConfig = fmt.Sprintf(`    insecure-skip-tls-verify: true
    server: %s`, server)
	}

	return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
%s
  name: external
contexts:
- context:
    cluster: external
    user: vault
  name: default
current-context: default
users:
- name: vault
  user:
    token: %s
`, clusterConfig, token)
}

// VaultKubeconfigSecret builds a Secret containing the generated kubeconfig for Vault auth.
func VaultKubeconfigSecret(m *v1alpha2.MondooAuditConfig, clusterName, kubeconfig string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      VaultKubeconfigSecretName(m.Name, clusterName),
			Namespace: m.Namespace,
			Labels:    ExternalClusterCronJobLabels(*m, clusterName),
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(kubeconfig),
		},
	}
}
