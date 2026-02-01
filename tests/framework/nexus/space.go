// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package nexus

import (
	"context"

	mondoogql "go.mondoo.com/mondoo-go"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/assets"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/k8s"
)

type Space struct {
	spaceMrn  string
	gqlClient *mondoogql.Client
	K8s       *k8s.Client
}

func NewSpace(gqlClient *mondoogql.Client, orgMrn string) (*Space, error) {
	var m struct {
		CreateSpace struct {
			Mrn string
		} `graphql:"createSpace(input: $input)"`
	}
	err := gqlClient.Mutate(context.Background(), &m, mondoogql.CreateSpaceInput{Name: "test", OrgMrn: mondoogql.String(orgMrn)}, nil)
	if err != nil {
		return nil, err
	}
	return &Space{
		spaceMrn:  m.CreateSpace.Mrn,
		gqlClient: gqlClient,
		K8s:       k8s.NewClient(m.CreateSpace.Mrn, gqlClient),
	}, nil
}

func (s *Space) Mrn() string {
	return s.spaceMrn
}

func (s *Space) ListAssetsWithScores(ctx context.Context) ([]assets.AssetWithScore, error) {
	return assets.ListAssetsWithScores(ctx, s.spaceMrn, s.gqlClient)
}

func (s *Space) Delete(ctx context.Context) error {
	var m struct {
		DeleteSpace string `graphql:"deleteSpace(spaceMrn: $input)"`
	}
	return s.gqlClient.Mutate(ctx, &m, nil, map[string]interface{}{
		"input": mondoogql.ID(s.spaceMrn),
	})
}

func (s *Space) DeleteAssets(ctx context.Context) error {
	var m struct {
		DeleteAssets struct {
			SpaceMrn string
		} `graphql:"deleteAssets(input: $input)"`
	}

	return s.gqlClient.Mutate(ctx, &m, mondoogql.DeleteAssetsInput{SpaceMrn: mondoogql.String(s.spaceMrn)}, nil)
}
