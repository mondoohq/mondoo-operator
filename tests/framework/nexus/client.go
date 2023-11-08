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
	MONDOO_ORG_MRN_VAR      = "MONDOO_ORG_MRN"
	MONDOO_GQL_ENDPOINT_VAR = "MONDOO_GQL_ENDPOINT"
)

type Client struct {
	orgMrn string
	Client *mondoogql.Client
}

func NewClient() (*Client, error) {
	orgMrn := os.Getenv(MONDOO_ORG_MRN_VAR)
	if orgMrn == "" {
		return nil, fmt.Errorf("missing environment variable %s", MONDOO_ORG_MRN_VAR)
	}

	gqlEndpoint := os.Getenv(MONDOO_GQL_ENDPOINT_VAR)
	if gqlEndpoint == "" {
		return nil, fmt.Errorf("missing environment variable %s", MONDOO_GQL_ENDPOINT_VAR)
	}

	apiToken := os.Getenv(MONDOO_API_TOKEN_VAR)
	if apiToken == "" {
		return nil, fmt.Errorf("missing environment variable %s", MONDOO_API_TOKEN_VAR)
	}
	fmt.Printf("Using GraphQL endpoint %s\n", gqlEndpoint)
	fmt.Printf("Using org MRN %s\n", orgMrn)
	// Initialize the client
	client, err := mondoogql.NewClient(option.WithEndpoint(gqlEndpoint), option.WithAPIToken(apiToken))
	if err != nil {
		return nil, err
	}

	return &Client{
		orgMrn: orgMrn,
		Client: client,
	}, nil
}

// TODO: when we support creating spaces this will actually create a space
func (c *Client) CreateSpace() (*Space, error) {
	return NewSpace(c.Client, c.orgMrn)
}
