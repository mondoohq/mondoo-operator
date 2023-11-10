// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package assets

import (
	"context"

	mondoogql "go.mondoo.com/mondoo-go"
)

type AssetWithScore struct {
	Mrn      string
	Name     string
	Labels   map[string]string
	Grade    string
	Platform struct {
		Name string
	}
	AssetType string
}

func ListAssetsWithScores(
	ctx context.Context,
	spaceMrn string,
	gqlClient *mondoogql.Client,
) ([]AssetWithScore, error) {
	var q struct {
		AssetsConnection struct {
			Edges []struct {
				Node struct {
					Mrn    string
					Name   string
					Labels []struct {
						Key   string
						Value string
					}
					Score struct {
						Grade string
					}
					Platform struct {
						Name string
					}
					AssetType string `graphql:"asset_type"`
				}
			}
		} `graphql:"assets(spaceMrn: $spaceMrn, first: 100)"`
	}

	err := gqlClient.Query(ctx, &q, map[string]interface{}{"spaceMrn": mondoogql.String(spaceMrn)})
	if err != nil {
		return nil, err
	}

	assetScores := make([]AssetWithScore, len(q.AssetsConnection.Edges))
	for i := range q.AssetsConnection.Edges {
		a := q.AssetsConnection.Edges[i].Node
		assetScores[i] = AssetWithScore{
			Mrn:       a.Mrn,
			Name:      a.Name,
			Labels:    make(map[string]string),
			Grade:     a.Score.Grade,
			Platform:  a.Platform,
			AssetType: a.AssetType,
		}
		for _, l := range a.Labels {
			assetScores[i].Labels[l.Key] = l.Value
		}
	}

	return assetScores, nil
}
