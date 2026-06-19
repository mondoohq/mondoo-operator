// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"time"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"
)

const NetworkInventoryOption = "kubernetesNetworkInventory"

const (
	networkInventoryInvalidCIDRReason         = "InvalidCIDRClassification"
	networkInventoryInvalidObservedFlowReason = "InvalidObservedFlowConfig"
	networkInventoryInvalidFlowEndpointReason = "InvalidObservedFlowEndpoint"
)

var (
	hbnAPIGroups       = []string{"network-connector.sylvaproject.org"}
	legacyHBNAPIGroups = []string{"network.t-caas.telekom.com"}
)

type networkInventoryValidationError struct {
	reason string
	err    error
}

func (e networkInventoryValidationError) Error() string {
	return e.err.Error()
}

func (e networkInventoryValidationError) Unwrap() error {
	return e.err
}

func newNetworkInventoryValidationError(reason string, err error) error {
	return networkInventoryValidationError{
		reason: reason,
		err:    err,
	}
}

type networkInventoryOptionFile struct {
	HBN                networkInventoryHBNConfig                `json:"hbn"`
	MultiNetworkPolicy networkInventoryMultiNetworkPolicyConfig `json:"multiNetworkPolicy"`
	Classifications    networkInventoryClassificationsConfig    `json:"classifications"`
	ObservedFlows      networkInventoryObservedFlowsConfig      `json:"observedFlows"`
}

type networkInventoryHBNConfig struct {
	Enabled                bool     `json:"enabled"`
	IncludeLegacyResources bool     `json:"includeLegacyResources"`
	APIGroups              []string `json:"apiGroups"`
}

type networkInventoryMultiNetworkPolicyConfig struct {
	Enabled                 bool     `json:"enabled"`
	APIGroup                string   `json:"apiGroup"`
	Resources               []string `json:"resources"`
	NetworkAttachmentConfig bool     `json:"networkAttachmentConfig"`
}

type networkInventoryClassificationsConfig struct {
	PublicCIDRs        []string `json:"publicCidrs,omitempty"`
	PrivateCIDRs       []string `json:"privateCidrs,omitempty"`
	TrustedEgressCIDRs []string `json:"trustedEgressCidrs,omitempty"`
}

type networkInventoryObservedFlowsConfig struct {
	Enabled       bool                               `json:"enabled"`
	MaxRecords    int                                `json:"maxRecords"`
	Lookback      string                             `json:"lookback"`
	Timeout       string                             `json:"timeout"`
	CalicoWhisker networkInventoryFlowEndpointConfig `json:"calicoWhisker"`
	CiliumHubble  networkInventoryFlowEndpointConfig `json:"ciliumHubble"`
}

type networkInventoryFlowEndpointConfig struct {
	Enabled     bool   `json:"enabled"`
	Namespace   string `json:"namespace"`
	ServiceName string `json:"serviceName"`
}

func addNetworkInventoryOptions(options map[string]string, spec v1alpha2.NetworkInventorySpec) error {
	if !spec.Enable {
		return nil
	}

	out, err := networkInventoryOption(spec)
	if err != nil {
		return err
	}
	options[NetworkInventoryOption] = out
	return nil
}

func networkInventoryOption(spec v1alpha2.NetworkInventorySpec) (string, error) {
	config, err := networkInventoryConfig(spec)
	if err != nil {
		return "", err
	}

	out, err := yaml.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func networkInventoryConfig(spec v1alpha2.NetworkInventorySpec) (networkInventoryOptionFile, error) {
	hbnEnabled := derefBool(spec.HBN.Enable, true)
	includeLegacyHBN := derefBool(spec.HBN.IncludeLegacyResources, true)
	multiNetworkPolicyEnabled := derefBool(spec.MultiNetworkPolicy.Enable, true)

	publicCIDRs, err := validateAndSortCIDRs("publicCidrs", spec.Classifications.PublicCIDRs)
	if err != nil {
		return networkInventoryOptionFile{}, err
	}
	privateCIDRs, err := validateAndSortCIDRs("privateCidrs", spec.Classifications.PrivateCIDRs)
	if err != nil {
		return networkInventoryOptionFile{}, err
	}
	trustedEgressCIDRs, err := validateAndSortCIDRs("trustedEgressCidrs", spec.Classifications.TrustedEgressCIDRs)
	if err != nil {
		return networkInventoryOptionFile{}, err
	}

	flowConfig, err := observedFlowsConfig(spec.ObservedFlows)
	if err != nil {
		return networkInventoryOptionFile{}, err
	}

	apiGroups := slices.Clone(hbnAPIGroups)
	if includeLegacyHBN {
		apiGroups = append(apiGroups, legacyHBNAPIGroups...)
	}
	sort.Strings(apiGroups)

	return networkInventoryOptionFile{
		HBN: networkInventoryHBNConfig{
			Enabled:                hbnEnabled,
			IncludeLegacyResources: includeLegacyHBN,
			APIGroups:              apiGroups,
		},
		MultiNetworkPolicy: networkInventoryMultiNetworkPolicyConfig{
			Enabled:                 multiNetworkPolicyEnabled,
			APIGroup:                "k8s.cni.cncf.io",
			Resources:               []string{"network-attachment-definitions", "multi-networkpolicies"},
			NetworkAttachmentConfig: multiNetworkPolicyEnabled,
		},
		Classifications: networkInventoryClassificationsConfig{
			PublicCIDRs:        publicCIDRs,
			PrivateCIDRs:       privateCIDRs,
			TrustedEgressCIDRs: trustedEgressCIDRs,
		},
		ObservedFlows: flowConfig,
	}, nil
}

func observedFlowsConfig(spec v1alpha2.ObservedFlowsSpec) (networkInventoryObservedFlowsConfig, error) {
	maxRecords := spec.MaxRecords
	if maxRecords == 0 {
		// A zero value means the field was omitted; the rendered scanner config uses the API default.
		maxRecords = 1000
	}
	if maxRecords < 1 {
		return networkInventoryObservedFlowsConfig{}, newNetworkInventoryValidationError(
			networkInventoryInvalidObservedFlowReason,
			fmt.Errorf("kubernetesResources.networkInventory.observedFlows.maxRecords must be at least 1"),
		)
	}

	lookback := spec.Lookback.Duration
	if lookback < 0 {
		return networkInventoryObservedFlowsConfig{}, newNetworkInventoryValidationError(
			networkInventoryInvalidObservedFlowReason,
			fmt.Errorf("kubernetesResources.networkInventory.observedFlows.lookback must not be negative"),
		)
	}
	if lookback == 0 {
		lookback = 5 * time.Minute
	}
	timeout := spec.Timeout.Duration
	if timeout < 0 {
		return networkInventoryObservedFlowsConfig{}, newNetworkInventoryValidationError(
			networkInventoryInvalidObservedFlowReason,
			fmt.Errorf("kubernetesResources.networkInventory.observedFlows.timeout must not be negative"),
		)
	}
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	calico, err := flowEndpointConfig("calicoWhisker", spec.CalicoWhisker, "calico-system", "whisker")
	if err != nil {
		return networkInventoryObservedFlowsConfig{}, err
	}
	cilium, err := flowEndpointConfig("ciliumHubble", spec.CiliumHubble, "kube-system", "hubble-relay")
	if err != nil {
		return networkInventoryObservedFlowsConfig{}, err
	}

	// The parent switch is allowed without an endpoint source so users can stage the
	// top-level observed-flow config separately from backend-specific credentials.
	enabled := spec.Enable || spec.CalicoWhisker.Enable || spec.CiliumHubble.Enable

	return networkInventoryObservedFlowsConfig{
		Enabled:       enabled,
		MaxRecords:    maxRecords,
		Lookback:      lookback.String(),
		Timeout:       timeout.String(),
		CalicoWhisker: calico,
		CiliumHubble:  cilium,
	}, nil
}

func flowEndpointConfig(field string, spec v1alpha2.FlowEndpointSpec, defaultNamespace, defaultServiceName string) (networkInventoryFlowEndpointConfig, error) {
	namespace := spec.Namespace
	if namespace == "" {
		namespace = defaultNamespace
	}
	serviceName := spec.ServiceName
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	if errs := validation.IsDNS1123Label(namespace); len(errs) > 0 {
		return networkInventoryFlowEndpointConfig{}, newNetworkInventoryValidationError(
			networkInventoryInvalidFlowEndpointReason,
			fmt.Errorf("kubernetesResources.networkInventory.observedFlows.%s.namespace is invalid: %s", field, strings.Join(errs, "; ")),
		)
	}
	if errs := validation.IsDNS1123Label(serviceName); len(errs) > 0 {
		return networkInventoryFlowEndpointConfig{}, newNetworkInventoryValidationError(
			networkInventoryInvalidFlowEndpointReason,
			fmt.Errorf("kubernetesResources.networkInventory.observedFlows.%s.serviceName is invalid: %s", field, strings.Join(errs, "; ")),
		)
	}

	return networkInventoryFlowEndpointConfig{
		Enabled:     spec.Enable,
		Namespace:   namespace,
		ServiceName: serviceName,
	}, nil
}

func validateAndSortCIDRs(field string, in []string) ([]string, error) {
	out := slices.Clone(in)
	for _, cidr := range out {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			return nil, newNetworkInventoryValidationError(
				networkInventoryInvalidCIDRReason,
				fmt.Errorf("kubernetesResources.networkInventory.classifications.%s contains invalid CIDR %q: %w", field, cidr, err),
			)
		}
	}
	sort.Strings(out)
	return out, nil
}

func derefBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
