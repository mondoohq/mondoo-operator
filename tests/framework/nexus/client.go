package nexus

import (
	"context"

	"go.mondoo.com/cnquery/upstream"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/captain"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/integrations"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/policy"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/k8s"
	"go.mondoo.com/ranger-rpc"
)

type Client struct {
	spaceMrn     string
	K8s          *k8s.Client
	AssetStore   policy.AssetStore
	Captain      captain.Captain
	Integrations integrations.IntegrationsManager
}

func NewClient(serviceAccount *upstream.ServiceAccountCredentials) (*Client, error) {
	plugin, err := upstream.NewServiceAccountRangerPlugin(serviceAccount)
	if err != nil {
		return nil, err
	}

	assetStore, err := policy.NewAssetStoreClient(serviceAccount.ApiEndpoint, ranger.DefaultHttpClient(), plugin)
	if err != nil {
		return nil, err
	}

	captain, err := captain.NewCaptainClient(serviceAccount.ApiEndpoint, ranger.DefaultHttpClient(), plugin)
	if err != nil {
		return nil, err
	}

	integrations, err := integrations.NewIntegrationsManagerClient(serviceAccount.ApiEndpoint, ranger.DefaultHttpClient(), plugin)
	if err != nil {
		return nil, err
	}

	return &Client{
		spaceMrn:     serviceAccount.ParentMrn,
		K8s:          k8s.NewClient(integrations),
		AssetStore:   assetStore,
		Captain:      captain,
		Integrations: integrations,
	}, nil
}

func (c *Client) CreateK8sIntegration(ctx context.Context) error {
	c.Integrations.Create(ctx, &integrations.CreateIntegrationRequest{
		Name:     "test-integration2",
		SpaceMrn: c.spaceMrn,
		Type:     integrations.Type_K8S,
		ConfigurationInput: &integrations.ConfigurationInput{
			ConfigOptions: &integrations.ConfigurationInput_K8SOptions{
				K8SOptions: &integrations.K8SConfigurationOptionsInput{
					ScanNodes:     true,
					ScanWorkloads: true,
				},
			},
		},
	})
	return nil
}
