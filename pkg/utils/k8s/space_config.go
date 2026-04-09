// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
)

const (
	// ConfigOverrideSecretSuffix is appended to the MondooAuditConfig name to form the
	// derived config Secret name when spaceId is set.
	ConfigOverrideSecretSuffix = "-config-override"

	// SpaceMrnPrefix is the MRN prefix for Mondoo spaces.
	SpaceMrnPrefix = "//captain.api.mondoo.app/spaces/"
)

// ConfigSecretRef returns the Secret reference to use for mounting the mondoo config.
// When spaceId is set, it returns the derived config override Secret; otherwise it
// returns the original MondooCredsSecretRef.
func ConfigSecretRef(m v1alpha2.MondooAuditConfig) corev1.LocalObjectReference {
	if m.Spec.SpaceID != "" {
		return corev1.LocalObjectReference{Name: m.Name + ConfigOverrideSecretSuffix}
	}
	return m.Spec.MondooCredsSecretRef
}

// SyncConfigOverrideSecret creates or updates a derived Secret that injects scope_mrn
// into the service account config when spaceId is set. Returns nil if spaceId is empty.
func SyncConfigOverrideSecret(
	ctx context.Context,
	kubeClient client.Client,
	m *v1alpha2.MondooAuditConfig,
) error {
	if m.Spec.SpaceID == "" {
		// Clean up any leftover override secret from when spaceId was previously set
		derivedSecret := &corev1.Secret{}
		key := client.ObjectKey{Name: m.Name + ConfigOverrideSecretSuffix, Namespace: m.Namespace}
		if err := kubeClient.Get(ctx, key, derivedSecret); err == nil {
			if err := kubeClient.Delete(ctx, derivedSecret); err != nil {
				return fmt.Errorf("failed to clean up override secret: %w", err)
			}
		}
		return nil
	}

	// Read the original credentials secret
	origSecret, err := GetIntegrationSecretForAuditConfig(ctx, kubeClient, *m)
	if err != nil {
		return fmt.Errorf("failed to get credentials secret: %w", err)
	}

	saData, ok := origSecret.Data[constants.MondooCredsSecretServiceAccountKey]
	if !ok {
		return fmt.Errorf("credentials secret missing key %q", constants.MondooCredsSecretServiceAccountKey)
	}

	// Unmarshal as generic map to preserve all original fields (including any future
	// additions) and inject scope_mrn. Go's encoding/json round-trips map[string]any
	// faithfully for JSON-compatible types.
	var config map[string]any
	if err := json.Unmarshal(saData, &config); err != nil {
		return fmt.Errorf("failed to unmarshal service account config: %w", err)
	}

	config["scope_mrn"] = SpaceMrnPrefix + m.Spec.SpaceID

	modifiedData, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal modified config: %w", err)
	}

	derivedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name + ConfigOverrideSecretSuffix,
			Namespace: m.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, kubeClient, derivedSecret, func() error {
		// Set owner reference for automatic cleanup
		if err := controllerutil.SetControllerReference(m, derivedSecret, kubeClient.Scheme()); err != nil {
			return err
		}

		if derivedSecret.Data == nil {
			derivedSecret.Data = make(map[string][]byte)
		}
		derivedSecret.Data[constants.MondooCredsSecretServiceAccountKey] = modifiedData

		// Copy integration MRN if present
		if integrationMrn, ok := origSecret.Data[constants.MondooCredsSecretIntegrationMRNKey]; ok {
			derivedSecret.Data[constants.MondooCredsSecretIntegrationMRNKey] = integrationMrn
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create/update config override secret: %w", err)
	}

	return nil
}

// SpaceMrnForAuditConfig returns the space MRN for the given MondooAuditConfig.
// If spaceId is set, it constructs the MRN from that. Otherwise returns empty string.
func SpaceMrnForAuditConfig(m v1alpha2.MondooAuditConfig) string {
	if m.Spec.SpaceID != "" {
		return SpaceMrnPrefix + m.Spec.SpaceID
	}
	return ""
}
