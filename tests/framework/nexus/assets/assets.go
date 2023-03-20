package assets

import (
	"context"

	cnspec "go.mondoo.com/cnspec/v9/policy"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/policy"
)

type AssetWithScore struct {
	Asset *policy.Asset
	Score *cnspec.Score
}

func ListAssetsWithScores(
	ctx context.Context,
	spaceMrn,
	integrationMrn,
	ciCdProjectMrn,
	assetType string,
	assetStore policy.AssetStore,
	policyResolver cnspec.PolicyResolver,
) ([]AssetWithScore, error) {
	filter := &policy.AssetSearchFilter{SpaceMrn: spaceMrn}
	if integrationMrn != "" {
		filter.QueryTerms = []string{"{ \"mondoo.com/integration-mrn\": \"" + integrationMrn + "\" }"}
	}

	if ciCdProjectMrn != "" {
		filter.CicdProjectMrn = ciCdProjectMrn
	}

	if assetType != "" {
		filter.AssetTypes = []string{assetType}
	}

	assetsPage, err := assetStore.ListAssets(ctx, filter)
	if err != nil {
		return nil, err
	}

	mrns := make([]string, len(assetsPage.List))
	for i := range assetsPage.List {
		mrns[i] = assetsPage.List[i].Mrn
	}

	assetScores := make([]AssetWithScore, len(assetsPage.List))
	for i := range assetsPage.List {
		asset := assetsPage.List[i]

		// TODO: replace this call with GetScore(ctx, &cnspec.EntityScoreReq{EntityMrn: asset.Mrn, ScoreMrn: asset.Mrn}) once nexus is released
		score, err := policyResolver.GetReport(ctx, &cnspec.EntityScoreReq{EntityMrn: asset.Mrn})
		if err != nil {
			return nil, err
		}
		assetScores[i] = AssetWithScore{Asset: asset, Score: score.Scores[asset.Mrn]}
	}
	return assetScores, nil
}
