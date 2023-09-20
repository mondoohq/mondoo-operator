// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nexus

import (
	"context"

	cnspec "go.mondoo.com/cnspec/policy"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/captain"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/integrations"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/policy"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/k8s"
)

type Space struct {
	spaceMrn string

	AssetStore     policy.AssetStore
	PolicyResolver cnspec.PolicyResolver
	Captain        captain.Captain
	Integrations   integrations.IntegrationsManager

	K8s *k8s.Client
}

type AssetWithScore struct {
	Asset *policy.Asset
	Score *cnspec.Score
}

func NewSpace(spaceMrn string, assetStore policy.AssetStore, policyResolver cnspec.PolicyResolver, captain captain.Captain, integrations integrations.IntegrationsManager) *Space {
	return &Space{
		spaceMrn:       spaceMrn,
		AssetStore:     assetStore,
		PolicyResolver: policyResolver,
		Captain:        captain,
		Integrations:   integrations,
		K8s:            k8s.NewClient(spaceMrn, integrations, assetStore),
	}
}

func (s *Space) ListAssetsWithScores(ctx context.Context, integrationMrn string) ([]AssetWithScore, error) {
	filter := &policy.AssetSearchFilter{SpaceMrn: s.spaceMrn}
	if integrationMrn != "" {
		filter.QueryTerms = []string{"{ \"mondoo.com/integration-mrn\": \"" + integrationMrn + "\" }"}
	}

	assetsPage, err := s.AssetStore.ListAssets(ctx, filter)
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
		score, err := s.PolicyResolver.GetReport(ctx, &cnspec.EntityScoreReq{EntityMrn: asset.Mrn})
		if err != nil {
			return nil, err
		}
		assetScores[i] = AssetWithScore{Asset: asset, Score: score.Scores[asset.Mrn]}
	}

	return assetScores, nil
}

func (s *Space) DeleteAssetsManagedBy(ctx context.Context, managedBy string) error {
	_, err := s.AssetStore.DeleteAssets(ctx, &policy.DeleteAssetsRequest{SpaceMrn: s.spaceMrn, ManagedBy: managedBy})
	return err
}
