// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nexus

import (
	"go.mondoo.com/cnquery/upstream"
	cnspec "go.mondoo.com/cnspec/policy"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/captain"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/integrations"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/policy"
	"go.mondoo.com/ranger-rpc"
)

type Client struct {
	spaceMrn string

	AssetStore     policy.AssetStore
	PolicyResolver cnspec.PolicyResolver
	Captain        captain.Captain
	Integrations   integrations.IntegrationsManager
}

func NewClient(serviceAccount *upstream.ServiceAccountCredentials) (*Client, error) {
	plugin, err := upstream.NewServiceAccountRangerPlugin(serviceAccount)
	if err != nil {
		return nil, err
	}

	assetStore, err := policy.NewAssetStoreClient(serviceAccount.ApiEndpoint, ranger.DefaultHttpClient(), plugin)
	if err != nil {
		return nil, err
	}

	policyResolver, err := cnspec.NewPolicyResolverClient(serviceAccount.ApiEndpoint, ranger.DefaultHttpClient(), plugin)
	if err != nil {
		return nil, err
	}

	captain, err := captain.NewCaptainClient(serviceAccount.ApiEndpoint, ranger.DefaultHttpClient(), plugin)
	if err != nil {
		return nil, err
	}

	integrations, err := integrations.NewIntegrationsManagerClient(serviceAccount.ApiEndpoint, ranger.DefaultHttpClient(), plugin)
	if err != nil {
		return nil, err
	}

	return &Client{
		spaceMrn:       serviceAccount.ParentMrn,
		AssetStore:     assetStore,
		PolicyResolver: policyResolver,
		Captain:        captain,
		Integrations:   integrations,
	}, nil
}

// TODO: when we support creating spaces this will actually create a space
func (c *Client) GetSpace() *Space {
	return NewSpace(c.spaceMrn, c.AssetStore, c.PolicyResolver, c.Captain, c.Integrations)
}
