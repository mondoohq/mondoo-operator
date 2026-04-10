// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
)

func TestConfigSecretRef_NoSpaceID(t *testing.T) {
	m := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			MondooCredsSecretRef: corev1.LocalObjectReference{Name: "my-creds"},
		},
	}
	ref := ConfigSecretRef(m)
	assert.Equal(t, "my-creds", ref.Name)
}

func TestConfigSecretRef_WithSpaceID(t *testing.T) {
	m := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "test-audit"},
		Spec: v1alpha2.MondooAuditConfigSpec{
			MondooCredsSecretRef: corev1.LocalObjectReference{Name: "my-creds"},
			SpaceID:              "abc123",
		},
	}
	ref := ConfigSecretRef(m)
	assert.Equal(t, "test-audit"+ConfigOverrideSecretSuffix, ref.Name)
}

func TestSpaceMrnForAuditConfig(t *testing.T) {
	t.Run("empty when no spaceId", func(t *testing.T) {
		m := v1alpha2.MondooAuditConfig{}
		assert.Equal(t, "", SpaceMrnForAuditConfig(m))
	})

	t.Run("constructs MRN when spaceId set", func(t *testing.T) {
		m := v1alpha2.MondooAuditConfig{
			Spec: v1alpha2.MondooAuditConfigSpec{SpaceID: "abc123"},
		}
		assert.Equal(t, "//captain.api.mondoo.app/spaces/abc123", SpaceMrnForAuditConfig(m))
	})
}

func TestSyncConfigOverrideSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	origConfig := map[string]any{
		"mrn":          "//agents.api.mondoo.app/organizations/org1/serviceaccounts/sa1",
		"private_key":  "test-key",
		"certificate":  "test-cert",
		"api_endpoint": "https://api.mondoo.app",
	}
	origConfigBytes, _ := json.Marshal(origConfig)

	t.Run("no-op when spaceId is empty", func(t *testing.T) {
		m := &v1alpha2.MondooAuditConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			Spec:       v1alpha2.MondooAuditConfigSpec{},
		}
		kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		err := SyncConfigOverrideSecret(context.Background(), kubeClient, m)
		assert.NoError(t, err)
	})

	t.Run("cleans up override secret when spaceId is removed", func(t *testing.T) {
		leftoverSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test" + ConfigOverrideSecretSuffix,
				Namespace: "default",
			},
			Data: map[string][]byte{
				constants.MondooCredsSecretServiceAccountKey: []byte(`{"scope_mrn":"old"}`),
			},
		}

		m := &v1alpha2.MondooAuditConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			Spec:       v1alpha2.MondooAuditConfigSpec{},
		}

		kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(leftoverSecret).Build()
		err := SyncConfigOverrideSecret(context.Background(), kubeClient, m)
		require.NoError(t, err)

		// Verify the override secret was deleted
		deletedSecret := &corev1.Secret{}
		err = kubeClient.Get(context.Background(), client.ObjectKey{
			Name:      "test" + ConfigOverrideSecretSuffix,
			Namespace: "default",
		}, deletedSecret)
		assert.Error(t, err, "override secret should have been deleted")
	})

	t.Run("creates derived secret with scope_mrn", func(t *testing.T) {
		origSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "org-creds", Namespace: "default"},
			Data: map[string][]byte{
				constants.MondooCredsSecretServiceAccountKey: origConfigBytes,
				constants.MondooCredsSecretIntegrationMRNKey: []byte("integration-mrn-value"),
			},
		}

		m := &v1alpha2.MondooAuditConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default", UID: "uid-123"},
			Spec: v1alpha2.MondooAuditConfigSpec{
				MondooCredsSecretRef: corev1.LocalObjectReference{Name: "org-creds"},
				SpaceID:              "target-space",
			},
		}

		kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(origSecret).Build()
		err := SyncConfigOverrideSecret(context.Background(), kubeClient, m)
		require.NoError(t, err)

		// Verify derived secret was created
		derivedSecret := &corev1.Secret{}
		err = kubeClient.Get(context.Background(), client.ObjectKey{
			Name:      "test" + ConfigOverrideSecretSuffix,
			Namespace: "default",
		}, derivedSecret)
		require.NoError(t, err)

		// Verify scope_mrn was injected
		var derivedConfig map[string]any
		err = json.Unmarshal(derivedSecret.Data[constants.MondooCredsSecretServiceAccountKey], &derivedConfig)
		require.NoError(t, err)
		assert.Equal(t, "//captain.api.mondoo.app/spaces/target-space", derivedConfig["scope_mrn"])
		assert.Equal(t, "test-key", derivedConfig["private_key"])
		assert.Equal(t, "https://api.mondoo.app", derivedConfig["api_endpoint"])

		// Verify integration MRN was copied
		assert.Equal(t, []byte("integration-mrn-value"),
			derivedSecret.Data[constants.MondooCredsSecretIntegrationMRNKey])
	})

	t.Run("updates existing derived secret", func(t *testing.T) {
		origSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "org-creds", Namespace: "default"},
			Data: map[string][]byte{
				constants.MondooCredsSecretServiceAccountKey: origConfigBytes,
			},
		}

		existingDerived := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test" + ConfigOverrideSecretSuffix, Namespace: "default"},
			Data: map[string][]byte{
				constants.MondooCredsSecretServiceAccountKey: []byte(`{"scope_mrn":"old-value"}`),
			},
		}

		m := &v1alpha2.MondooAuditConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default", UID: "uid-123"},
			Spec: v1alpha2.MondooAuditConfigSpec{
				MondooCredsSecretRef: corev1.LocalObjectReference{Name: "org-creds"},
				SpaceID:              "new-space",
			},
		}

		kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(origSecret, existingDerived).Build()
		err := SyncConfigOverrideSecret(context.Background(), kubeClient, m)
		require.NoError(t, err)

		derivedSecret := &corev1.Secret{}
		err = kubeClient.Get(context.Background(), client.ObjectKey{
			Name:      "test" + ConfigOverrideSecretSuffix,
			Namespace: "default",
		}, derivedSecret)
		require.NoError(t, err)

		var derivedConfig map[string]any
		err = json.Unmarshal(derivedSecret.Data[constants.MondooCredsSecretServiceAccountKey], &derivedConfig)
		require.NoError(t, err)
		assert.Equal(t, "//captain.api.mondoo.app/spaces/new-space", derivedConfig["scope_mrn"])
	})
}
