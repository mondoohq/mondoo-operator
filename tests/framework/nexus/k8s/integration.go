// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"

	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/integrations"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/policy"
)

type IntegrationBuilder struct {
	integrations integrations.IntegrationsManager
	assetStore   policy.AssetStore

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
		integrations: b.integrations,
		assetStore:   b.assetStore,
		name:         b.name,
		mrn:          resp.Integration.Mrn,
		spaceMrn:     b.spaceMrn,
	}, nil
}

type Integration struct {
	integrations integrations.IntegrationsManager
	assetStore   policy.AssetStore

	name     string
	mrn      string
	spaceMrn string
}

func (i *Integration) Mrn() string {
	return i.mrn
}

func (i *Integration) GetRegistrationToken(ctx context.Context) (string, error) {
	resp, err := i.integrations.GetTokenForIntegration(
		ctx, &integrations.GetTokenForIntegrationRequest{Mrn: i.mrn, LongLivedToken: false})
	if err != nil {
		return "", err
	}
	return resp.Token, nil
}

func (i *Integration) Delete(ctx context.Context) error {
	_, err := i.integrations.Delete(ctx, &integrations.DeleteIntegrationRequest{Mrn: i.mrn})
	return err
}

func (i *Integration) DeleteCiCdProjectIfExists(ctx context.Context) error {
	resp, err := i.assetStore.ListCicdProjects(ctx, &policy.ListCicdProjectsRequest{SpaceMrn: i.spaceMrn})
	if err != nil {
		return err
	}

	for _, p := range resp.List {
		if p.Labels["mondoo.com/integration-mrn"] == i.mrn {
			_, err := i.assetStore.DeleteCicdProjects(ctx, &policy.DeleteCicdProjectsRequest{Mrns: []string{p.Mrn}})
			return err
		}
	}
	return nil
}
