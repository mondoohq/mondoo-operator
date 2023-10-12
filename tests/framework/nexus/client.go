// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nexus

import (
	"fmt"
	"os"

	mondoogql "go.mondoo.com/mondoo-go"
	"go.mondoo.com/mondoo-go/option"
)

const (
	MONDOO_API_TOKEN_VAR    = "MONDOO_API_TOKEN"
	MONDOO_GQL_ENDPOINT_VAR = "MONDOO_GQL_ENDPOINT"
)

type Client struct {
	Client *mondoogql.Client
}

func NewClient() (*Client, error) {
	gqlEndpoint := os.Getenv(MONDOO_GQL_ENDPOINT_VAR)
	if gqlEndpoint == "" {
		return nil, fmt.Errorf("missing environment variable %s", MONDOO_GQL_ENDPOINT_VAR)
	}

	apiToken := os.Getenv(MONDOO_API_TOKEN_VAR)
	if apiToken == "" {
		return nil, fmt.Errorf("missing environment variable %s", MONDOO_API_TOKEN_VAR)
	}
	// Initialize the client
	client, err := mondoogql.NewClient(option.WithEndpoint(gqlEndpoint), option.WithAPIToken(apiToken))
	if err != nil {
		return nil, err
	}

	return &Client{
		Client: client,
	}, nil
}

// TODO: when we support creating spaces this will actually create a space
func (c *Client) GetSpace() *Space {
	return NewSpace(c.Client)
}
