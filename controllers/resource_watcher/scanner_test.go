// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mondoo.com/mql/v13/providers-sdk/v1/inventory"
	"sigs.k8s.io/yaml"

	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

func TestScannerGenerateInventory_LabelSelectors(t *testing.T) {
	resources := []K8sResourceIdentifier{{Type: "deployments", Namespace: "default", Name: "nginx"}}

	tests := []struct {
		name              string
		config            ScannerConfig
		expectedNamespace string
		expectedObject    string
	}{
		{
			name: "selectors set",
			config: ScannerConfig{
				NamespaceLabelSelector: "tenant=team-a",
				ObjectLabelSelector:    "scan notin (disabled)",
			},
			expectedNamespace: "tenant=team-a",
			expectedObject:    "scan notin (disabled)",
		},
		{
			name:   "selectors unset",
			config: ScannerConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invBytes, err := NewScanner(tt.config).generateInventory(resources)
			require.NoError(t, err)

			var inv inventory.Inventory
			require.NoError(t, yaml.Unmarshal(invBytes, &inv))
			opts := inv.Spec.Assets[0].Connections[0].Options

			if tt.expectedNamespace == "" {
				assert.NotContains(t, opts, k8s.NamespaceLabelSelectorOption)
			} else {
				assert.Equal(t, tt.expectedNamespace, opts[k8s.NamespaceLabelSelectorOption])
			}
			if tt.expectedObject == "" {
				assert.NotContains(t, opts, k8s.ObjectLabelSelectorOption)
			} else {
				assert.Equal(t, tt.expectedObject, opts[k8s.ObjectLabelSelectorOption])
			}
		})
	}
}
