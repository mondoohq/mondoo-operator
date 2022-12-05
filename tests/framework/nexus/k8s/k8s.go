package k8s

import (
	"go.mondoo.com/mondoo-operator/tests/framework/nexus/api/integrations"
)

type Client struct {
	spaceMrn     string
	integrations integrations.IntegrationsManager
}

func NewClient(spaceMrn string, integrations integrations.IntegrationsManager) *Client {
	return &Client{
		spaceMrn:     spaceMrn,
		integrations: integrations,
	}
}

func (k *Client) CreateIntegration(name string) *IntegrationBuilder {
	return &IntegrationBuilder{
		spaceMrn:     k.spaceMrn,
		name:         name,
		integrations: k.integrations,
	}
}
