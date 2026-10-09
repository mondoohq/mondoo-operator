// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"fmt"
	"net/netip"

	"sigs.k8s.io/yaml"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

// NetworkInventoryOption is the k8s provider inventory option that tunes network posture collection.
const NetworkInventoryOption = "kubernetesNetworkInventory"

// networkInventoryOption mirrors the kubernetesNetworkInventory option parsed by the k8s provider.
// Unset fields are omitted so the provider applies its own defaults.
type networkInventoryOption struct {
	HBN struct {
		Enabled                *bool `json:"enabled,omitempty"`
		IncludeLegacyResources *bool `json:"includeLegacyResources,omitempty"`
	} `json:"hbn,omitzero"`
	MultiNetworkPolicy struct {
		Enabled *bool `json:"enabled,omitempty"`
	} `json:"multiNetworkPolicy,omitzero"`
	Classifications struct {
		PublicCIDRs        []string `json:"publicCidrs,omitempty"`
		PrivateCIDRs       []string `json:"privateCidrs,omitempty"`
		TrustedEgressCIDRs []string `json:"trustedEgressCidrs,omitempty"`
	} `json:"classifications,omitzero"`
}

// addNetworkInventoryOptions adds the kubernetesNetworkInventory option when any network inventory
// setting is configured. It returns an error if a CIDR classification is invalid.
func addNetworkInventoryOptions(options map[string]string, spec v1alpha2.NetworkInventorySpec) error {
	if !networkInventoryConfigured(spec) {
		return nil
	}

	c := spec.Classifications
	for _, f := range []struct {
		name  string
		cidrs []string
	}{
		{"publicCidrs", c.PublicCIDRs},
		{"privateCidrs", c.PrivateCIDRs},
		{"trustedEgressCidrs", c.TrustedEgressCIDRs},
	} {
		for _, cidr := range f.cidrs {
			if _, err := netip.ParsePrefix(cidr); err != nil {
				return fmt.Errorf("kubernetesResources.networkInventory.classifications.%s contains invalid CIDR %q: %w", f.name, cidr, err)
			}
		}
	}

	var opt networkInventoryOption
	opt.HBN.Enabled = spec.HBN.Enable
	opt.HBN.IncludeLegacyResources = spec.HBN.IncludeLegacyResources
	opt.MultiNetworkPolicy.Enabled = spec.MultiNetworkPolicy.Enable
	opt.Classifications.PublicCIDRs = c.PublicCIDRs
	opt.Classifications.PrivateCIDRs = c.PrivateCIDRs
	opt.Classifications.TrustedEgressCIDRs = c.TrustedEgressCIDRs

	out, err := yaml.Marshal(opt)
	if err != nil {
		return err
	}
	options[NetworkInventoryOption] = string(out)
	return nil
}

func networkInventoryConfigured(spec v1alpha2.NetworkInventorySpec) bool {
	c := spec.Classifications
	return spec.HBN != (v1alpha2.HBNNetworkInventorySpec{}) ||
		spec.MultiNetworkPolicy != (v1alpha2.MultiNetworkPolicyInventorySpec{}) ||
		len(c.PublicCIDRs) > 0 || len(c.PrivateCIDRs) > 0 || len(c.TrustedEgressCIDRs) > 0
}
