// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
)

const (
	testSpaceMrn       = "//captain.api.mondoo.app/spaces/test-space"
	testOrgMrn         = "//captain.api.mondoo.app/organizations/test-org"
	testIntegrationMrn = "//integration.api.mondoo.app/spaces/test-space/integrations/abcdefg"
)

func testProvisionerCredential(scopeMrn string) *provisionerCredential {
	sa := mondooclient.ServiceAccountCredentials{
		Mrn:         "//agents.api.mondoo.app/spaces/test-space/serviceaccounts/1234",
		SpaceMrn:    scopeMrn,
		PrivateKey:  "PRIVATE KEY",
		Certificate: "CERT",
		ApiEndpoint: "http://127.0.0.2:8989",
	}
	raw, _ := json.Marshal(sa) //nolint:gosec
	return &provisionerCredential{sa: sa, raw: string(raw)}
}

func TestIsIntegrationMrn(t *testing.T) {
	assert.True(t, IsIntegrationMrn(testIntegrationMrn))
	assert.False(t, IsIntegrationMrn(testSpaceMrn))
	assert.False(t, IsIntegrationMrn(""))
	assert.False(t, IsIntegrationMrn("<nil>"))
}

func TestParseServiceAccountCredential(t *testing.T) {
	valid := testProvisionerCredential(testSpaceMrn)

	cred, ok := parseServiceAccountCredential(valid.raw)
	require.True(t, ok)
	assert.Equal(t, valid.sa, cred.sa)

	_, ok = parseServiceAccountCredential("eyJhbGciOi.some.jwt")
	assert.False(t, ok, "a JWT is not a service account")

	_, ok = parseServiceAccountCredential(`{"mrn": "only-mrn"}`)
	assert.False(t, ok, "incomplete service account JSON must be rejected")

	_, ok = parseServiceAccountCredential("")
	assert.False(t, ok)
}

func TestIntegrationScopeMrn(t *testing.T) {
	m := &v1alpha2.MondooAuditConfig{}

	// space-scoped credential, no spaceId
	scope, err := integrationScopeMrn(m, testProvisionerCredential(testSpaceMrn))
	require.NoError(t, err)
	assert.Equal(t, testSpaceMrn, scope)

	// org-scoped credential without spaceId cannot determine a target space
	_, err = integrationScopeMrn(m, testProvisionerCredential(testOrgMrn))
	assert.Error(t, err)

	// spaceId selects the space for an org-scoped credential
	m.Spec.SpaceID = "test-space"
	scope, err = integrationScopeMrn(m, testProvisionerCredential(testOrgMrn))
	require.NoError(t, err)
	assert.Equal(t, testSpaceMrn, scope)

	// matching space-scoped credential and spaceId
	scope, err = integrationScopeMrn(m, testProvisionerCredential(testSpaceMrn))
	require.NoError(t, err)
	assert.Equal(t, testSpaceMrn, scope)

	// mismatch between the credential's space and spaceId is an error
	m.Spec.SpaceID = "another-space"
	_, err = integrationScopeMrn(m, testProvisionerCredential(testSpaceMrn))
	assert.Error(t, err)
}

func TestIntegrationName(t *testing.T) {
	m := &v1alpha2.MondooAuditConfig{}
	assert.Equal(t, "mondoo-operator-d0d0caca", integrationName(m, "d0d0caca-0000-1111-2222-333344445555"))
	assert.Equal(t, "mondoo-operator-short", integrationName(m, "short"))

	m.Spec.ConsoleIntegration.Name = "my-cluster"
	assert.Equal(t, "my-cluster", integrationName(m, "d0d0caca-0000-1111-2222-333344445555"))
}

func TestAuditConfigIdentifier(t *testing.T) {
	id := AuditConfigIdentifier("cluster-uid", client.ObjectKey{Namespace: "mondoo-operator", Name: "mondoo-config"})
	assert.Equal(t, "k8s-audit-config:cluster-uid/mondoo-operator/mondoo-config", id)
}

func TestK8sConfigurationOptions(t *testing.T) {
	m := &v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			Nodes: v1alpha2.Nodes{
				Enable:   true,
				Style:    v1alpha2.NodeScanStyle_DaemonSet,
				Schedule: "0 * * * *",
			},
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable:   true,
				Schedule: "30 * * * *",
			},
			Containers: v1alpha2.Containers{
				Enable:   true,
				Schedule: "0 6 * * *",
			},
			Filtering: v1alpha2.Filtering{
				Namespaces: v1alpha2.FilteringSpec{
					Include: []string{"prod-*"},
					Exclude: []string{"kube-system"},
				},
			},
		},
	}

	opts := k8sConfigurationOptions(m)
	assert.Equal(t, &mondooclient.K8sConfigurationOptionsInput{
		ScanNodes:          true,
		ScanNodesStyle:     mondooclient.ScanNodesStyleDaemonSet,
		ScanWorkloads:      true,
		ScanPublicImages:   true,
		ScanLocalCluster:   true,
		NamespaceAllowList: []string{"prod-*"},
		NamespaceDenyList:  []string{"kube-system"},
		Schedule:           "30 * * * *",
		NodesSchedule:      "0 * * * *",
		ContainersSchedule: "0 6 * * *",
	}, opts)

	// node style defaults to cronjob when nodes are enabled without a style
	m.Spec.Nodes.Style = ""
	assert.Equal(t, mondooclient.ScanNodesStyleCronJob, k8sConfigurationOptions(m).ScanNodesStyle)

	// no style is sent at all when node scanning is disabled
	m.Spec.Nodes.Enable = false
	assert.Empty(t, k8sConfigurationOptions(m).ScanNodesStyle)
}
