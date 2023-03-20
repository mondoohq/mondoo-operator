// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nexus

import (
	"context"

	cnspec "go.mondoo.com/cnspec/v9/policy"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/captain"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/integrations"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/policy"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/assets"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/k8s"
	"go.uber.org/zap"
)

type Space struct {
	spaceMrn string

	AssetStore     policy.AssetStore
	PolicyResolver cnspec.PolicyResolver
	Captain        captain.Captain
	Integrations   integrations.IntegrationsManager

	K8s *k8s.Client
}

func NewSpace(spaceMrn string, assetStore policy.AssetStore, policyResolver cnspec.PolicyResolver, captain captain.Captain, integrations integrations.IntegrationsManager) *Space {
	return &Space{
		spaceMrn:       spaceMrn,
		AssetStore:     assetStore,
		PolicyResolver: policyResolver,
		Captain:        captain,
		Integrations:   integrations,
		K8s:            k8s.NewClient(spaceMrn, integrations, assetStore, policyResolver),
	}
}

func (s *Space) Mrn() string {
	return s.spaceMrn
}

func (s *Space) ListAssetsWithScores(ctx context.Context, integrationMrn, assetType string) ([]assets.AssetWithScore, error) {
	return assets.ListAssetsWithScores(ctx, s.spaceMrn, integrationMrn, "", assetType, s.AssetStore, s.PolicyResolver)
}

func (s *Space) DeleteAssetsManagedBy(ctx context.Context, managedBy string) error {
	res, err := s.AssetStore.DeleteAssets(ctx, &policy.DeleteAssetsRequest{SpaceMrn: s.spaceMrn, ManagedBy: managedBy})
	zap.S().Infof("deleted assets %v", res)
	return err
}
