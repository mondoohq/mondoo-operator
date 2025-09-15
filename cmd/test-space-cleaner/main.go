// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	mondoogql "go.mondoo.com/mondoo-go"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus"
)

func main() {
	nexusClient, err := nexus.NewClient()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create nexus client")
	}

	ctx := context.Background()
	spaces, err := ListSpaces(ctx, nexusClient.Client, "//captain.api.mondoo.app/organizations/mondoo-operator-testing")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to list spaces")

	}
	for _, space := range spaces.Edges {
		if space.Node.Name == "mondoo-operator-tests" {
			continue
		}
		err := Delete(ctx, nexusClient.Client, space.Node.Mrn)
		if err != nil {
			log.Warn().Err(err).Str("mrn", space.Node.Mrn).Msg("failed to delete space")
			continue
		}
		fmt.Println("Deleted space:", space.Node.Name, "with MRN:", space.Node.Mrn)
		log.Info().Str("name", space.Node.Name).Str("mrn", space.Node.Mrn).Msg("deleted space")
	}
}

func Delete(ctx context.Context, gqlClient *mondoogql.Client, mrn string) error {
	var m struct {
		DeleteSpace string `graphql:"deleteSpace(spaceMrn: $input)"`
	}
	return gqlClient.Mutate(ctx, &m, nil, map[string]interface{}{
		"input": mondoogql.ID(mrn),
	})
}

type SpaceConnection struct {
	Edges []struct {
		Node struct {
			Name string
			Mrn  string
		}
	}
}

type OrgWithListSpacesQuery struct {
	Id          string
	Mrn         string
	Name        string
	Description string
	// TODO: handle pagination
	SpaceList SpaceConnection `graphql:"spacesList"`
}

// TODO: output is not great yet, lets focus on spaces
func ListSpaces(ctx context.Context, gqlClient *mondoogql.Client, orgMrn string) (SpaceConnection, error) {
	var q struct {
		Organization OrgWithListSpacesQuery `graphql:"organization(mrn: $mrn)"`
	}
	variables := map[string]interface{}{
		"mrn": mondoogql.String(orgMrn),
	}

	err := gqlClient.Query(ctx, &q, variables)
	if err != nil {
		return SpaceConnection{}, err
	}

	return q.Organization.SpaceList, nil
}
