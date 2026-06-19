// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mql/v13/providers-sdk/v1/inventory"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

func TestInventoryNetworkInventoryDisabledByDefault(t *testing.T) {
	options, targets := inventoryConnection(t, v1alpha2.MondooAuditConfig{})
	_, ok := options[NetworkInventoryOption]
	assert.False(t, ok)
	assertNoNetworkInventoryDiscoveryTargets(t, targets)
}

func TestInventoryNetworkInventoryOption(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				NetworkInventory: v1alpha2.NetworkInventorySpec{
					Enable: true,
					Classifications: v1alpha2.NetworkInventoryClassifications{
						PublicCIDRs:        []string{"203.0.113.0/24", "198.51.100.0/24"},
						PrivateCIDRs:       []string{"10.0.0.0/8"},
						TrustedEgressCIDRs: []string{"192.0.2.0/24"},
					},
				},
			},
		},
	}

	config := networkInventoryOptionFromInventory(t, auditConfig)

	assert.True(t, config.HBN.Enabled)
	assert.True(t, config.HBN.IncludeLegacyResources)
	assert.Equal(t, []string{"network-connector.sylvaproject.org", "network.t-caas.telekom.com"}, config.HBN.APIGroups)
	assert.True(t, config.MultiNetworkPolicy.Enabled)
	assert.Equal(t, "k8s.cni.cncf.io", config.MultiNetworkPolicy.APIGroup)
	assert.Equal(t, []string{"network-attachment-definitions", "multi-networkpolicies"}, config.MultiNetworkPolicy.Resources)
	assert.Equal(t, []string{"198.51.100.0/24", "203.0.113.0/24"}, config.Classifications.PublicCIDRs)
	assert.Equal(t, []string{"10.0.0.0/8"}, config.Classifications.PrivateCIDRs)
	assert.Equal(t, []string{"192.0.2.0/24"}, config.Classifications.TrustedEgressCIDRs)
	assert.False(t, config.ObservedFlows.Enabled)
	assert.Equal(t, 1000, config.ObservedFlows.MaxRecords)
	assert.Equal(t, "5m0s", config.ObservedFlows.Lookback)
	assert.Equal(t, "10s", config.ObservedFlows.Timeout)
}

func TestInventoryNetworkInventoryCanDisableOptionalSources(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				NetworkInventory: v1alpha2.NetworkInventorySpec{
					Enable: true,
					HBN: v1alpha2.HBNNetworkInventorySpec{
						Enable:                 ptr.To(false),
						IncludeLegacyResources: ptr.To(false),
					},
					MultiNetworkPolicy: v1alpha2.MultiNetworkPolicyInventorySpec{
						Enable: ptr.To(false),
					},
				},
			},
		},
	}

	config := networkInventoryOptionFromInventory(t, auditConfig)

	assert.False(t, config.HBN.Enabled)
	assert.False(t, config.HBN.IncludeLegacyResources)
	assert.Equal(t, []string{"network-connector.sylvaproject.org"}, config.HBN.APIGroups)
	assert.False(t, config.MultiNetworkPolicy.Enabled)
	assert.False(t, config.MultiNetworkPolicy.NetworkAttachmentConfig)
}

func TestInventoryNetworkInventoryObservedFlows(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				NetworkInventory: v1alpha2.NetworkInventorySpec{
					Enable: true,
					ObservedFlows: v1alpha2.ObservedFlowsSpec{
						Enable:     true,
						MaxRecords: 250,
						Lookback:   metav1.Duration{Duration: 2 * time.Minute},
						Timeout:    metav1.Duration{Duration: 15 * time.Second},
						CalicoWhisker: v1alpha2.FlowEndpointSpec{
							Enable:      true,
							Namespace:   "tigera-operator",
							ServiceName: "whisker-api",
						},
						CiliumHubble: v1alpha2.FlowEndpointSpec{
							Enable: true,
						},
					},
				},
			},
		},
	}

	config := networkInventoryOptionFromInventory(t, auditConfig)

	assert.True(t, config.ObservedFlows.Enabled)
	assert.Equal(t, 250, config.ObservedFlows.MaxRecords)
	assert.Equal(t, "2m0s", config.ObservedFlows.Lookback)
	assert.Equal(t, "15s", config.ObservedFlows.Timeout)
	assert.Equal(t, networkInventoryFlowEndpointConfig{
		Enabled:     true,
		Namespace:   "tigera-operator",
		ServiceName: "whisker-api",
	}, config.ObservedFlows.CalicoWhisker)
	assert.Equal(t, networkInventoryFlowEndpointConfig{
		Enabled:     true,
		Namespace:   "kube-system",
		ServiceName: "hubble-relay",
	}, config.ObservedFlows.CiliumHubble)
}

func TestInventoryNetworkInventoryObservedFlowBackendEnablesParent(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				NetworkInventory: v1alpha2.NetworkInventorySpec{
					Enable: true,
					ObservedFlows: v1alpha2.ObservedFlowsSpec{
						CiliumHubble: v1alpha2.FlowEndpointSpec{
							Enable: true,
						},
					},
				},
			},
		},
	}

	config := networkInventoryOptionFromInventory(t, auditConfig)

	assert.True(t, config.ObservedFlows.Enabled)
	assert.False(t, config.ObservedFlows.CalicoWhisker.Enabled)
	assert.True(t, config.ObservedFlows.CiliumHubble.Enabled)
	assert.Equal(t, "hubble-relay", config.ObservedFlows.CiliumHubble.ServiceName)
}

func TestInventoryNetworkInventoryObservedFlowsParentOnly(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				NetworkInventory: v1alpha2.NetworkInventorySpec{
					Enable: true,
					ObservedFlows: v1alpha2.ObservedFlowsSpec{
						Enable: true,
					},
				},
			},
		},
	}

	config := networkInventoryOptionFromInventory(t, auditConfig)

	assert.True(t, config.ObservedFlows.Enabled)
	assert.False(t, config.ObservedFlows.CalicoWhisker.Enabled)
	assert.False(t, config.ObservedFlows.CiliumHubble.Enabled)
}

func TestInventoryNetworkInventoryAllowsRFC1123ServiceNames(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				NetworkInventory: v1alpha2.NetworkInventorySpec{
					Enable: true,
					ObservedFlows: v1alpha2.ObservedFlowsSpec{
						CiliumHubble: v1alpha2.FlowEndpointSpec{
							Enable:      true,
							ServiceName: "3scale-hubble-relay",
						},
					},
				},
			},
		},
	}

	config := networkInventoryOptionFromInventory(t, auditConfig)

	assert.Equal(t, "3scale-hubble-relay", config.ObservedFlows.CiliumHubble.ServiceName)
}

func TestInventoryNetworkInventoryRejectsInvalidCIDR(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				NetworkInventory: v1alpha2.NetworkInventorySpec{
					Enable: true,
					Classifications: v1alpha2.NetworkInventoryClassifications{
						PublicCIDRs: []string{"not-a-cidr"},
					},
				},
			},
		},
	}

	_, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "publicCidrs")
	var validationErr networkInventoryValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, networkInventoryInvalidCIDRReason, validationErr.reason)
}

func TestInventoryNetworkInventoryRejectsInvalidObservedFlowConfig(t *testing.T) {
	tests := []struct {
		name        string
		flows       v1alpha2.ObservedFlowsSpec
		expectedMsg string
	}{
		{
			name: "negative max records",
			flows: v1alpha2.ObservedFlowsSpec{
				MaxRecords: -1,
			},
			expectedMsg: "maxRecords",
		},
		{
			name: "negative lookback",
			flows: v1alpha2.ObservedFlowsSpec{
				Lookback: metav1.Duration{Duration: -1 * time.Second},
			},
			expectedMsg: "lookback",
		},
		{
			name: "negative timeout",
			flows: v1alpha2.ObservedFlowsSpec{
				Timeout: metav1.Duration{Duration: -1 * time.Second},
			},
			expectedMsg: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditConfig := v1alpha2.MondooAuditConfig{
				Spec: v1alpha2.MondooAuditConfigSpec{
					KubernetesResources: v1alpha2.KubernetesResources{
						NetworkInventory: v1alpha2.NetworkInventorySpec{
							Enable:        true,
							ObservedFlows: tt.flows,
						},
					},
				},
			}

			_, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedMsg)
			var validationErr networkInventoryValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.Equal(t, networkInventoryInvalidObservedFlowReason, validationErr.reason)
		})
	}
}

func TestInventoryNetworkInventoryRejectsInvalidFlowEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint v1alpha2.FlowEndpointSpec
	}{
		{
			name: "enabled endpoint",
			endpoint: v1alpha2.FlowEndpointSpec{
				Enable:    true,
				Namespace: "not_valid",
			},
		},
		{
			name: "disabled endpoint with explicit invalid name",
			endpoint: v1alpha2.FlowEndpointSpec{
				ServiceName: "not_valid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditConfig := v1alpha2.MondooAuditConfig{
				Spec: v1alpha2.MondooAuditConfigSpec{
					KubernetesResources: v1alpha2.KubernetesResources{
						NetworkInventory: v1alpha2.NetworkInventorySpec{
							Enable: true,
							ObservedFlows: v1alpha2.ObservedFlowsSpec{
								CalicoWhisker: tt.endpoint,
							},
						},
					},
				},
			}

			_, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "calicoWhisker.")
			var validationErr networkInventoryValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.Equal(t, networkInventoryInvalidFlowEndpointReason, validationErr.reason)
		})
	}
}

func TestInventoryNetworkInventoryKeepsDiscoveryTargetsProviderSupported(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				NetworkInventory: v1alpha2.NetworkInventorySpec{Enable: true},
			},
		},
	}

	_, targets := inventoryConnection(t, auditConfig)

	assert.Contains(t, targets, "clusters")
	assert.Contains(t, targets, "services")
	assert.Contains(t, targets, "ingresses")
	assertNoNetworkInventoryDiscoveryTargets(t, targets)
}

func TestDiscoveryTargetsNetworkInventoryForcesClusterTarget(t *testing.T) {
	originalTargets := K8sDiscoveryTargets
	t.Cleanup(func() {
		K8sDiscoveryTargets = originalTargets
	})
	K8sDiscoveryTargets = []string{"pods"}

	targets := discoveryTargets(v1alpha2.NetworkInventorySpec{Enable: true}, false)

	assert.Contains(t, targets, "clusters")
	assert.Contains(t, targets, "pods")
}

func TestExternalClusterInventoryIncludesNetworkInventoryOption(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				NetworkInventory: v1alpha2.NetworkInventorySpec{Enable: true},
			},
		},
	}
	cluster := v1alpha2.ExternalCluster{Name: "remote"}

	options, targets := externalClusterInventoryConnection(t, auditConfig, cluster)

	assert.NotEmpty(t, options[NetworkInventoryOption])
	assert.Equal(t, "false", options["disable-cache"])
	assert.Contains(t, targets, "clusters")
	assertNoNetworkInventoryDiscoveryTargets(t, targets)
}

func TestExternalClusterInventoryHonorsDisabledNetworkInventorySources(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				NetworkInventory: v1alpha2.NetworkInventorySpec{
					Enable: true,
					HBN: v1alpha2.HBNNetworkInventorySpec{
						Enable: ptr.To(false),
					},
					MultiNetworkPolicy: v1alpha2.MultiNetworkPolicyInventorySpec{
						Enable: ptr.To(false),
					},
				},
			},
		},
	}
	cluster := v1alpha2.ExternalCluster{Name: "remote"}

	_, targets := externalClusterInventoryConnection(t, auditConfig, cluster)

	assertNoNetworkInventoryDiscoveryTargets(t, targets)
}

func TestExternalClusterInventoryKeepsContainerImageTarget(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				NetworkInventory: v1alpha2.NetworkInventorySpec{Enable: true},
			},
		},
	}
	cluster := v1alpha2.ExternalCluster{Name: "remote", ContainerImageScanning: true}

	_, targets := externalClusterInventoryConnection(t, auditConfig, cluster)

	assert.Contains(t, targets, "container-images")
	assertNoNetworkInventoryDiscoveryTargets(t, targets)
}

func assertNoNetworkInventoryDiscoveryTargets(t *testing.T, targets []string) {
	t.Helper()

	for _, target := range []string{
		"adminnetworkpolicies",
		"baselineadminnetworkpolicies",
		"calico-globalnetworkpolicies",
		"calico-networkpolicies",
		"ciliumclusterwidenetworkpolicies",
		"ciliumnetworkpolicies",
		"gatewayclasses",
		"gateways",
		"grpcroutes",
		"httproutes",
		"networkpolicies",
		"referencegrants",
		"tcproutes",
		"tlsroutes",
		"udproutes",
		"hbn-network",
		"multi-networkpolicies",
		"multinetworkpolicies",
		"network-attachment-definitions",
	} {
		assert.NotContains(t, targets, target)
	}
}

func networkInventoryOptionFromInventory(t *testing.T, auditConfig v1alpha2.MondooAuditConfig) networkInventoryOptionFile {
	t.Helper()

	options := inventoryOptions(t, auditConfig)
	raw, ok := options[NetworkInventoryOption]
	require.True(t, ok)

	var config networkInventoryOptionFile
	require.NoError(t, yaml.Unmarshal([]byte(raw), &config))
	return config
}

func inventoryOptions(t *testing.T, auditConfig v1alpha2.MondooAuditConfig) map[string]string {
	t.Helper()

	options, _ := inventoryConnection(t, auditConfig)
	return options
}

func inventoryConnection(t *testing.T, auditConfig v1alpha2.MondooAuditConfig) (map[string]string, []string) {
	t.Helper()

	invStr, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets)
	require.NotEmpty(t, inv.Spec.Assets[0].Connections)

	conn := inv.Spec.Assets[0].Connections[0]
	return conn.Options, conn.Discover.Targets
}

func externalClusterInventoryConnection(t *testing.T, auditConfig v1alpha2.MondooAuditConfig, cluster v1alpha2.ExternalCluster) (map[string]string, []string) {
	t.Helper()

	invStr, err := ExternalClusterInventory("", testClusterUID, cluster, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets)
	require.NotEmpty(t, inv.Spec.Assets[0].Connections)

	conn := inv.Spec.Assets[0].Connections[0]
	return conn.Options, conn.Discover.Targets
}
