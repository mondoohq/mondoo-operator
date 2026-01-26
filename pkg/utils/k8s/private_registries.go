// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

const (
	// DefaultPrivateRegistriesSecretName is the default name for the private registries secret
	DefaultPrivateRegistriesSecretName = "mondoo-private-registries-secrets"
)

var prLogger = ctrl.Log.WithName("private-registries")

// CollectPrivateRegistrySecretNames collects all private registry secret names from the MondooAuditConfig.
// It checks both the singular PrivateRegistriesPullSecretRef and the plural PrivateRegistriesPullSecretRefs fields,
// as well as the default secret name. Only secrets that actually exist in the cluster are returned.
func CollectPrivateRegistrySecretNames(ctx context.Context, kubeClient client.Client, m *v1alpha2.MondooAuditConfig) ([]string, error) {
	secretNames := make(map[string]struct{})

	// Collect from the plural field first (preferred)
	for _, ref := range m.Spec.Scanner.PrivateRegistriesPullSecretRefs {
		if ref.Name != "" {
			secretNames[ref.Name] = struct{}{}
		}
	}

	// Collect from the singular field (deprecated but still supported)
	if m.Spec.Scanner.PrivateRegistriesPullSecretRef.Name != "" {
		secretNames[m.Spec.Scanner.PrivateRegistriesPullSecretRef.Name] = struct{}{}
	}

	// If no secrets were explicitly specified, check for the default secret
	if len(secretNames) == 0 {
		secretNames[DefaultPrivateRegistriesSecretName] = struct{}{}
	}

	// Filter to only secrets that exist
	var existingSecrets []string
	for name := range secretNames {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: m.Namespace,
			},
		}
		found, err := CheckIfExists(ctx, kubeClient, secret, secret)
		if err != nil {
			return nil, err
		}
		if found {
			existingSecrets = append(existingSecrets, name)
		} else {
			prLogger.Info("private registries pull secret not found",
				"namespace", m.Namespace,
				"secretname", name)
		}
	}

	if len(existingSecrets) == 0 {
		prLogger.Info("no private registries pull secrets found, trying to fetch imagePullSecrets for each discovered image")
	}

	return existingSecrets, nil
}
