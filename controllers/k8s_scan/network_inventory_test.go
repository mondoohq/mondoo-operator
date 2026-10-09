// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mql/v13/providers-sdk/v1/inventory"
)

func TestNetworkInventoryOption(t *testing.T) {
	tests := []struct {
		name     string
		spec     v1alpha2.NetworkInventorySpec
		expected string
	}{
		{
			name: "unset",
		},
		{
			name: "toggles only",
			spec: v1alpha2.NetworkInventorySpec{
				HBN:                v1alpha2.HBNNetworkInventorySpec{IncludeLegacyResources: ptr.To(false)},
				MultiNetworkPolicy: v1alpha2.MultiNetworkPolicyInventorySpec{Enable: ptr.To(false)},
			},
			expected: "hbn:\n  includeLegacyResources: false\nmultiNetworkPolicy:\n  enabled: false\n",
		},
		{
			name: "classifications only",
			spec: v1alpha2.NetworkInventorySpec{
				Classifications: v1alpha2.NetworkInventoryClassifications{
					PublicCIDRs:        []string{"203.0.113.0/24", "2001:db8::/32"},
					PrivateCIDRs:       []string{"10.0.0.0/8"},
					TrustedEgressCIDRs: []string{"192.0.2.0/24"},
				},
			},
			expected: "classifications:\n  privateCidrs:\n  - 10.0.0.0/8\n  publicCidrs:\n  - 203.0.113.0/24\n  - 2001:db8::/32\n  trustedEgressCidrs:\n  - 192.0.2.0/24\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditConfig := v1alpha2.MondooAuditConfig{
				Spec: v1alpha2.MondooAuditConfigSpec{
					KubernetesResources: v1alpha2.KubernetesResources{NetworkInventory: tt.spec},
				},
			}

			local := inventoryOptions(t, auditConfig)
			external := externalClusterInventoryOptions(t, auditConfig, v1alpha2.ExternalCluster{Name: "remote"})

			if tt.expected == "" {
				assert.NotContains(t, local, NetworkInventoryOption)
				assert.NotContains(t, external, NetworkInventoryOption)
				return
			}
			assert.Equal(t, tt.expected, local[NetworkInventoryOption])
			assert.Equal(t, tt.expected, external[NetworkInventoryOption])
		})
	}
}

func TestNetworkInventoryOption_InvalidCIDR(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				NetworkInventory: v1alpha2.NetworkInventorySpec{
					Classifications: v1alpha2.NetworkInventoryClassifications{
						TrustedEgressCIDRs: []string{"10.0.0.0/8", "not-a-cidr"},
					},
				},
			},
		},
	}

	_, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `trustedEgressCidrs contains invalid CIDR "not-a-cidr"`)

	_, err = ExternalClusterInventory("", testClusterUID, v1alpha2.ExternalCluster{Name: "remote"}, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `trustedEgressCidrs contains invalid CIDR "not-a-cidr"`)
}

func inventoryOptions(t *testing.T, auditConfig v1alpha2.MondooAuditConfig) map[string]string {
	t.Helper()

	invStr, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets)
	require.NotEmpty(t, inv.Spec.Assets[0].Connections)

	return inv.Spec.Assets[0].Connections[0].Options
}
