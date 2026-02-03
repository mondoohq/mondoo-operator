// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

const (
	// DefaultPrivateRegistriesSecretName is the default name for the private registries secret
	DefaultPrivateRegistriesSecretName = "mondoo-private-registries-secrets"
	// MergedPrivateRegistriesSecretSuffix is appended to create the merged secret name
	MergedPrivateRegistriesSecretSuffix = "-private-registries-merged"
)

var prLogger = ctrl.Log.WithName("private-registries")

// DockerConfigJSON represents the structure of a Docker config.json file
type DockerConfigJSON struct {
	Auths map[string]DockerAuthConfig `json:"auths"`
}

// DockerAuthConfig represents auth config for a single registry
type DockerAuthConfig struct {
	Auth          string `json:"auth,omitempty"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Email         string `json:"email,omitempty"`
	ServerAddress string `json:"serveraddress,omitempty"`
	IdentityToken string `json:"identitytoken,omitempty"`
	RegistryToken string `json:"registrytoken,omitempty"`
}

// MergedSecretName returns the name of the merged private registries secret for a MondooAuditConfig
func MergedSecretName(m *v1alpha2.MondooAuditConfig) string {
	return m.Name + MergedPrivateRegistriesSecretSuffix
}

// ReconcilePrivateRegistriesSecret reconciles the private registry secrets.
// If multiple secrets are configured, it merges them into a single managed secret.
// Returns the name of the secret to use (either the single source secret or the merged secret),
// or empty string if no secrets are configured/found.
func ReconcilePrivateRegistriesSecret(ctx context.Context, kubeClient client.Client, m *v1alpha2.MondooAuditConfig) (string, error) {
	secretNames := collectSecretNames(m)

	// If no secrets are explicitly specified, check for the default secret
	if len(secretNames) == 0 {
		secretNames = []string{DefaultPrivateRegistriesSecretName}
	}

	// Filter to only secrets that exist and collect their data
	existingSecrets := make(map[string]*corev1.Secret)
	for _, name := range secretNames {
		secret := &corev1.Secret{}
		err := kubeClient.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: name}, secret)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				return "", err
			}
			prLogger.Info("private registries pull secret not found",
				"namespace", m.Namespace,
				"secretname", name)
			continue
		}
		existingSecrets[name] = secret
	}

	if len(existingSecrets) == 0 {
		prLogger.Info("no private registries pull secrets found, trying to fetch imagePullSecrets for each discovered image")
		// Clean up any existing merged secret
		if err := DeleteIfExists(ctx, kubeClient, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      MergedSecretName(m),
				Namespace: m.Namespace,
			},
		}); err != nil {
			return "", err
		}
		return "", nil
	}

	// If only one secret exists, use it directly (no need to merge)
	if len(existingSecrets) == 1 {
		for name := range existingSecrets {
			// Clean up any existing merged secret since we don't need it
			if err := DeleteIfExists(ctx, kubeClient, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MergedSecretName(m),
					Namespace: m.Namespace,
				},
			}); err != nil {
				return "", err
			}
			return name, nil
		}
	}

	// Multiple secrets exist - merge them
	mergedConfig, err := mergeDockerConfigs(existingSecrets)
	if err != nil {
		return "", fmt.Errorf("failed to merge docker configs: %w", err)
	}

	mergedSecretName := MergedSecretName(m)
	mergedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mergedSecretName,
			Namespace: m.Namespace,
		},
	}

	_, err = CreateOrUpdate(ctx, kubeClient, mergedSecret, m, prLogger, func() error {
		mergedSecret.Labels = map[string]string{
			"app.kubernetes.io/managed-by": "mondoo-operator",
			"mondoo_cr":                    m.Name,
		}
		mergedSecret.Type = corev1.SecretTypeDockerConfigJson
		mergedSecret.Data = map[string][]byte{
			".dockerconfigjson": mergedConfig,
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to create/update merged secret: %w", err)
	}

	prLogger.Info("merged private registry secrets",
		"namespace", m.Namespace,
		"mergedSecret", mergedSecretName,
		"sourceSecrets", len(existingSecrets))

	return mergedSecretName, nil
}

// collectSecretNames collects all configured private registry secret names from the MondooAuditConfig
func collectSecretNames(m *v1alpha2.MondooAuditConfig) []string {
	seen := make(map[string]struct{})
	var names []string

	// Collect from the plural field first (preferred)
	for _, ref := range m.Spec.Scanner.PrivateRegistriesPullSecretRefs {
		if ref.Name != "" {
			if _, exists := seen[ref.Name]; !exists {
				seen[ref.Name] = struct{}{}
				names = append(names, ref.Name)
			}
		}
	}

	// Collect from the singular field (deprecated but still supported)
	if m.Spec.Scanner.PrivateRegistriesPullSecretRef.Name != "" {
		name := m.Spec.Scanner.PrivateRegistriesPullSecretRef.Name
		if _, exists := seen[name]; !exists {
			names = append(names, name)
		}
	}

	return names
}

// mergeDockerConfigs merges multiple Docker config secrets into a single config
func mergeDockerConfigs(secrets map[string]*corev1.Secret) ([]byte, error) {
	merged := DockerConfigJSON{
		Auths: make(map[string]DockerAuthConfig),
	}

	for name, secret := range secrets {
		data, ok := secret.Data[".dockerconfigjson"]
		if !ok {
			prLogger.Info("secret missing .dockerconfigjson key, skipping",
				"secret", name)
			continue
		}

		var config DockerConfigJSON
		if err := json.Unmarshal(data, &config); err != nil {
			prLogger.Error(err, "failed to parse docker config, skipping",
				"secret", name)
			continue
		}

		// Merge auths - later secrets override earlier ones for the same registry
		for registry, auth := range config.Auths {
			merged.Auths[registry] = auth
		}
	}

	return json.Marshal(merged)
}
