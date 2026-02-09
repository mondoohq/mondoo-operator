// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"go.mondoo.com/cnquery/v12/providers-sdk/v1/inventory"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

const testClusterUID = "abcdefg"

func TestInventory_WithAnnotations(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Annotations: map[string]string{
				"env":  "prod",
				"team": "platform",
			},
		},
	}

	invStr, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err, "unexpected error generating inventory")

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets, "expected at least one asset")

	for _, asset := range inv.Spec.Assets {
		assert.Equal(t, "prod", asset.Annotations["env"], "asset %s missing env annotation", asset.Name)
		assert.Equal(t, "platform", asset.Annotations["team"], "asset %s missing team annotation", asset.Name)
	}
}

func TestExternalClusterInventory_WithAnnotations(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Annotations: map[string]string{
				"env":  "staging",
				"team": "security",
			},
		},
	}

	cluster := v1alpha2.ExternalCluster{
		Name: "remote-cluster",
	}

	invStr, err := ExternalClusterInventory("", testClusterUID, cluster, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err, "unexpected error generating inventory")

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets, "expected at least one asset")

	for _, asset := range inv.Spec.Assets {
		assert.Equal(t, "staging", asset.Annotations["env"], "asset %s missing env annotation", asset.Name)
		assert.Equal(t, "security", asset.Annotations["team"], "asset %s missing team annotation", asset.Name)
	}
}
