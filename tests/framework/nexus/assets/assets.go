// Copyright Mondoo, Inc. 2026
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
		Name    string
		Runtime string
	}
	PolicyScores []PolicyScore
	AssetType    string
}

type PolicyScore struct {
	Mrn   string
	Grade string
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
						Name    string
						Runtime string
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

	var assetQ struct {
		Asset struct {
			ListPolicies struct {
				Edges []struct {
					Node struct {
						Mrn   string
						Score struct {
							Grade string
						}
					}
				}
			} `graphql:"listPolicies"`
		} `graphql:"asset(mrn: $mrn)"`
	}

	assetScores := make([]AssetWithScore, len(q.AssetsConnection.Edges))
	for i := range q.AssetsConnection.Edges {
		a := q.AssetsConnection.Edges[i].Node
		err := gqlClient.Query(ctx, &assetQ, map[string]interface{}{"mrn": mondoogql.String(a.Mrn)})
		if err != nil {
			return nil, err
		}

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

		for _, p := range assetQ.Asset.ListPolicies.Edges {
			assetScores[i].PolicyScores = append(assetScores[i].PolicyScores, PolicyScore{
				Mrn:   p.Node.Mrn,
				Grade: p.Node.Score.Grade,
			})
		}
	}

	return assetScores, nil
}
