package k8s

import (
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/integrations"
)

type Client struct {
	integrations integrations.IntegrationsManager
}

func NewClient(integrations integrations.IntegrationsManager) *Client {
	return &Client{
		integrations: integrations,
	}
}

func (k *Client) CreateIntegration(name string) *IntegrationBuilder {
	return &IntegrationBuilder{
		name:         name,
		integrations: k.integrations,
	}
}
