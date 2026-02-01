// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	mondoogql "go.mondoo.com/mondoo-go"
)

type Client struct {
	spaceMrn string
	client   *mondoogql.Client
}

func NewClient(spaceMrn string, gqlClient *mondoogql.Client) *Client {
	return &Client{
		spaceMrn: spaceMrn,
		client:   gqlClient,
	}
}

func (k *Client) CreateIntegration(name string) *IntegrationBuilder {
	return &IntegrationBuilder{
		spaceMrn: k.spaceMrn,
		name:     name,
		client:   k.client,
	}
}
