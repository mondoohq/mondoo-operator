package nexus

import (
	"context"

	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/captain"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/integrations"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/policy"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/k8s"
)

type Space struct {
	spaceMrn string

	AssetStore   policy.AssetStore
	Captain      captain.Captain
	Integrations integrations.IntegrationsManager

	K8s *k8s.Client
}

func NewSpace(spaceMrn string, assetStore policy.AssetStore, captain captain.Captain, integrations integrations.IntegrationsManager) *Space {
	return &Space{
		spaceMrn:     spaceMrn,
		AssetStore:   assetStore,
		Captain:      captain,
		Integrations: integrations,
		K8s:          k8s.NewClient(spaceMrn, integrations),
	}
}

func (s *Space) ListAssets(ctx context.Context) ([]*policy.Asset, error) {
	assetsPage, err := s.AssetStore.ListAssets(ctx, &policy.AssetSearchFilter{SpaceMrn: s.spaceMrn})
	if err != nil {
		return nil, err
	}
	return assetsPage.List, nil
}
