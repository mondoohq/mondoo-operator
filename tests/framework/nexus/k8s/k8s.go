// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	cnspec "go.mondoo.com/cnspec/policy"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/integrations"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/policy"
)

type Client struct {
	spaceMrn       string
	integrations   integrations.IntegrationsManager
	assetStore     policy.AssetStore
	policyResolver cnspec.PolicyResolver
}

func NewClient(spaceMrn string, integrations integrations.IntegrationsManager, assetStore policy.AssetStore, policyResolver cnspec.PolicyResolver) *Client {
	return &Client{
		spaceMrn:       spaceMrn,
		integrations:   integrations,
		assetStore:     assetStore,
		policyResolver: policyResolver,
	}
}

func (k *Client) CreateIntegration(name string) *IntegrationBuilder {
	return &IntegrationBuilder{
		spaceMrn:       k.spaceMrn,
		name:           name,
		integrations:   k.integrations,
		assetStore:     k.assetStore,
		policyResolver: k.policyResolver,
	}
}
