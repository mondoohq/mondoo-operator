// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"
	"fmt"

	mondoogql "go.mondoo.com/mondoo-go"
)

type IntegrationBuilder struct {
	client              *mondoogql.Client
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

func (i *IntegrationBuilder) EnableContainersScan() *IntegrationBuilder {
	i.scanContainerImages = true
	return i
}

func (i *IntegrationBuilder) EnableAdmissionController() *IntegrationBuilder {
	i.admissionController = true
	return i
}

func (b *IntegrationBuilder) Run(ctx context.Context) (*Integration, error) {
	var m struct {
		CreateIntegration struct {
			Integration struct {
				Mrn   string
				Token string
			}
		} `graphql:"createClientIntegration(input: $input)"`
	}
	err := b.client.Mutate(ctx, &m, mondoogql.CreateClientIntegrationInput{
		SpaceMrn: mondoogql.String(b.spaceMrn),
		Name:     mondoogql.String(b.name),
		Type:     mondoogql.ClientIntegrationTypeK8s,
		ConfigurationOptions: mondoogql.ClientIntegrationConfigurationInput{
			K8sConfigurationOptions: &mondoogql.K8sConfigurationOptionsInput{
				ScanNodes:        mondoogql.Boolean(b.scanNodes),
				ScanWorkloads:    mondoogql.Boolean(b.scanWorkloads),
				ScanPublicImages: mondoogql.NewBooleanPtr(mondoogql.Boolean(b.scanContainerImages)),
				ScanDeploys:      mondoogql.Boolean(b.admissionController),
			},
		},
	}, nil)
	if err != nil {
		return nil, err
	}

	return &Integration{
		gqlClient: b.client,
		name:      b.name,
		mrn:       m.CreateIntegration.Integration.Mrn,
		token:     m.CreateIntegration.Integration.Token,
		spaceMrn:  b.spaceMrn,
	}, nil
}

type Integration struct {
	gqlClient *mondoogql.Client

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
	var m struct {
		DeleteIntegration struct {
			Mrn string
		} `graphql:"deleteClientIntegration(input: $input)"`
	}
	return i.gqlClient.Mutate(ctx, &m, mondoogql.DeleteClientIntegrationInput{Mrn: mondoogql.String(i.mrn)}, nil)
}

func (i *Integration) DeleteCiCdProjectIfExists(ctx context.Context) error {
	p, err := i.GetCiCdProject(ctx)
	if err != nil {
		return nil
	}

	return p.Delete(ctx)
}

func (i *Integration) GetCiCdProject(ctx context.Context) (*CiCdProject, error) {
	var q struct {
		CiCdProjects struct {
			Projects struct {
				Projects struct {
					Edges []struct {
						Node struct {
							Mrn    string
							Labels []struct {
								Key   string
								Value string
							}
						}
					}
				} `graphql:"projects(first: $first)"`
			} `graphql:"... on CicdProjects"`
		} `graphql:"cicdProjects(input: $input)"`
	}

	err := i.gqlClient.Query(ctx, &q, map[string]interface{}{"input": mondoogql.CicdProjectsInput{SpaceMrn: i.spaceMrn}, "first": mondoogql.Int(100)})
	if err != nil {
		return nil, err
	}

	for _, p := range q.CiCdProjects.Projects.Projects.Edges {
		for _, l := range p.Node.Labels {
			if l.Key == "mondoo.com/integration-mrn" && l.Value == i.mrn {
				return &CiCdProject{gqlClient: i.gqlClient, mrn: p.Node.Mrn, spaceMrn: i.spaceMrn}, nil
			}
		}
	}
	return nil, fmt.Errorf("cannot find CI/CD project for integration %s", i.mrn)
}

type CiCdProject struct {
	gqlClient *mondoogql.Client
	mrn       string
	spaceMrn  string
}

func (p *CiCdProject) Delete(ctx context.Context) error {
	var m struct {
		DeleteCicdProject struct {
			Mrns []string
		} `graphql:"deleteCicdProjects(input: $input)"`
	}
	return p.gqlClient.Mutate(ctx, &m, mondoogql.DeleteProjectsInput{Mrns: []mondoogql.String{mondoogql.String(p.mrn)}}, nil)
}

type CiCdAsset struct {
	Mrn   string
	Name  string
	Grade string
}

func (p *CiCdProject) ListAssets(ctx context.Context, assetType string) ([]CiCdAsset, error) {
	var q struct {
		CicdProjectJobs struct {
			Jobs struct {
				Jobs struct {
					Edges []struct {
						Node struct {
							Job struct {
								Mrn   string
								Name  string
								Grade string
							} `graphql:"... on KubernetesJob"`
						}
					}
				} `graphql:"jobs(first:$first)"`
			} `graphql:"... on CicdProjectJobs"`
		} `graphql:"cicdProjectJobs(input: $input)"`
	}
	err := p.gqlClient.Query(ctx, &q, map[string]interface{}{
		"input": mondoogql.CicdProjectJobsInput{SpaceMrn: p.spaceMrn, ProjectID: p.mrn},
		"first": mondoogql.Int(100),
	})
	if err != nil {
		return nil, err
	}
	assets := make([]CiCdAsset, 0, len(q.CicdProjectJobs.Jobs.Jobs.Edges))
	for _, e := range q.CicdProjectJobs.Jobs.Jobs.Edges {
		assets = append(assets, CiCdAsset{
			Mrn:   e.Node.Job.Mrn,
			Name:  e.Node.Job.Name,
			Grade: e.Node.Job.Grade,
		})
	}
	return assets, nil
}
