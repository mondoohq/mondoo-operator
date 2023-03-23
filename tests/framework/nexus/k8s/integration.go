// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"
	"fmt"

	cnspec "go.mondoo.com/cnspec/v9/policy"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/integrations"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/policy"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/assets"
)

type IntegrationBuilder struct {
	integrations   integrations.IntegrationsManager
	assetStore     policy.AssetStore
	policyResolver cnspec.PolicyResolver

	spaceMrn            string
	name                string
	scanNodes           bool
	scanWorkloads       bool
	scanContainerImages bool
	admissionController bool
}

func (i *IntegrationBuilder) EnableNodesScan() *IntegrationBuilder {
	i.scanNodes = true
	return i
}

func (i *IntegrationBuilder) EnableWorkloadsScan() *IntegrationBuilder {
	i.scanWorkloads = true
	return i
}

func (i *IntegrationBuilder) EnableContainerImagesScan() *IntegrationBuilder {
	i.scanContainerImages = true
	return i
}

func (i *IntegrationBuilder) EnableAdmissionController() *IntegrationBuilder {
	i.admissionController = true
	return i
}

func (b *IntegrationBuilder) Run(ctx context.Context) (*Integration, error) {
	resp, err := b.integrations.Create(ctx, &integrations.CreateIntegrationRequest{
		Name:     b.name,
		SpaceMrn: b.spaceMrn,
		Type:     integrations.Type_K8S,
		ConfigurationInput: &integrations.ConfigurationInput{
			ConfigOptions: &integrations.ConfigurationInput_K8SOptions{
				K8SOptions: &integrations.K8SConfigurationOptionsInput{
					ScanNodes:        b.scanNodes,
					ScanWorkloads:    b.scanWorkloads,
					ScanPublicImages: b.scanContainerImages,
					ScanDeploys:      b.admissionController,
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return &Integration{
		integrations:   b.integrations,
		assetStore:     b.assetStore,
		policyResolver: b.policyResolver,
		name:           b.name,
		mrn:            resp.Integration.Mrn,
		token:          resp.Integration.Token,
		spaceMrn:       b.spaceMrn,
	}, nil
}

type Integration struct {
	integrations   integrations.IntegrationsManager
	assetStore     policy.AssetStore
	policyResolver cnspec.PolicyResolver

	name     string
	mrn      string
	spaceMrn string
	token    string
}

func (i *Integration) Mrn() string {
	return i.mrn
}

func (i *Integration) Token() string {
	return i.token
}

func (i *Integration) Delete(ctx context.Context) error {
	_, err := i.integrations.Delete(ctx, &integrations.DeleteIntegrationRequest{Mrn: i.mrn})
	return err
}

func (i *Integration) DeleteCiCdProjectIfExists(ctx context.Context) error {
	p, err := i.GetCiCdProject(ctx)
	if err != nil {
		return nil
	}

	return p.Delete(ctx)
}

func (i *Integration) GetCiCdProject(ctx context.Context) (*CiCdProject, error) {
	resp, err := i.assetStore.ListCicdProjects(ctx, &policy.ListCicdProjectsRequest{SpaceMrn: i.spaceMrn})
	if err != nil {
		return nil, err
	}

	for _, p := range resp.List {
		if p.Labels["mondoo.com/integration-mrn"] == i.mrn {
			return &CiCdProject{assetStore: i.assetStore, policyResolver: i.policyResolver, mrn: p.Mrn, spaceMrn: i.spaceMrn}, nil
		}
	}
	return nil, fmt.Errorf("cannot find CI/CD project for integration %s", i.mrn)
}

type CiCdProject struct {
	assetStore     policy.AssetStore
	policyResolver cnspec.PolicyResolver
	mrn            string
	spaceMrn       string
}

func (p *CiCdProject) Delete(ctx context.Context) error {
	_, err := p.assetStore.DeleteCicdProjects(ctx, &policy.DeleteCicdProjectsRequest{Mrns: []string{p.mrn}})
	return err
}

func (p *CiCdProject) ListAssets(ctx context.Context, assetType string) ([]assets.AssetWithScore, error) {
	return assets.ListAssetsWithScores(ctx, p.spaceMrn, "", p.mrn, assetType, p.assetStore, p.policyResolver)
}
