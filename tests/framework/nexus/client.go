// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nexus

import (
	"go.mondoo.com/cnquery/v9/providers-sdk/v1/upstream"

	mondoogql "go.mondoo.com/mondoo-go"
	"go.mondoo.com/mondoo-go/option"
)

type Client struct {
	spaceMrn string

	Client *mondoogql.Client
}

func NewClient(serviceAccount *upstream.ServiceAccountCredentials) (*Client, error) {
	// Initialize the client
	client, err := mondoogql.NewClient(option.WithEndpoint("https://api.edge.mondoo.com/query"), option.WithAPIToken(""))
	if err != nil {
		return nil, err
	}

	return &Client{
		spaceMrn: serviceAccount.ParentMrn,
		Client:   client,
	}, nil
}

// TODO: when we support creating spaces this will actually create a space
func (c *Client) GetSpace() *Space {
	return NewSpace(c.Client)
}
