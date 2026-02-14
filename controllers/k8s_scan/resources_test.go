// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"go.mondoo.com/cnquery/v12/providers-sdk/v1/inventory"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

const testClusterUID = "abcdefg"

func testAuditConfig() *v1alpha2.MondooAuditConfig {
	return &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Schedule: "0 * * * *",
			},
		},
	}
}

func TestInventory_WithAnnotations(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Annotations: map[string]string{
				"env":  "prod",
				"team": "platform",
			},
		},
	}

	invStr, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err, "unexpected error generating inventory")

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets, "expected at least one asset")

	for _, asset := range inv.Spec.Assets {
		assert.Equal(t, "prod", asset.Annotations["env"], "asset %s missing env annotation", asset.Name)
		assert.Equal(t, "platform", asset.Annotations["team"], "asset %s missing team annotation", asset.Name)
	}
}

func TestExternalClusterInventory_WithAnnotations(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Annotations: map[string]string{
				"env":  "staging",
				"team": "security",
			},
		},
	}

	cluster := v1alpha2.ExternalCluster{
		Name: "remote-cluster",
	}

	invStr, err := ExternalClusterInventory("", testClusterUID, cluster, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err, "unexpected error generating inventory")

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets, "expected at least one asset")

	for _, asset := range inv.Spec.Assets {
		assert.Equal(t, "staging", asset.Annotations["env"], "asset %s missing env annotation", asset.Name)
		assert.Equal(t, "security", asset.Annotations["team"], "asset %s missing team annotation", asset.Name)
	}
}

func TestCronJob_WithProxy(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:  ptr.To("http://proxy:8080"),
			HttpsProxy: ptr.To("https://proxy:8443"),
		},
	}

	cj := CronJob("test-image:latest", m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	cmdStr := strings.Join(container.Command, " ")
	assert.Contains(t, cmdStr, "--api-proxy")
	assert.Contains(t, cmdStr, "https://proxy:8443")

	envMap := envToMap(container.Env)
	assert.Equal(t, "http://proxy:8080", envMap["HTTP_PROXY"])
	assert.Equal(t, "https://proxy:8443", envMap["HTTPS_PROXY"])
}

func TestCronJob_HttpsProxyPreferred(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:  ptr.To("http://proxy:8080"),
			HttpsProxy: ptr.To("https://proxy:8443"),
		},
	}

	cj := CronJob("test-image:latest", m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	cmdStr := strings.Join(container.Command, " ")
	assert.Contains(t, cmdStr, "--api-proxy https://proxy:8443")
	assert.NotContains(t, cmdStr, "http://proxy:8080")
}

func TestCronJob_SkipProxyForCnspec(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:          ptr.To("http://proxy:8080"),
			HttpsProxy:         ptr.To("https://proxy:8443"),
			SkipProxyForCnspec: true,
		},
	}

	cj := CronJob("test-image:latest", m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	cmdStr := strings.Join(container.Command, " ")
	assert.NotContains(t, cmdStr, "--api-proxy")

	envMap := envToMap(container.Env)
	_, hasHTTPProxy := envMap["HTTP_PROXY"]
	_, hasHTTPSProxy := envMap["HTTPS_PROXY"]
	assert.False(t, hasHTTPProxy, "HTTP_PROXY should not be set when SkipProxyForCnspec is true")
	assert.False(t, hasHTTPSProxy, "HTTPS_PROXY should not be set when SkipProxyForCnspec is true")
}

func TestCronJob_WithImagePullSecrets(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "my-registry-secret"},
			},
		},
	}

	cj := CronJob("test-image:latest", m, cfg)
	secrets := cj.Spec.JobTemplate.Spec.Template.Spec.ImagePullSecrets
	require.Len(t, secrets, 1)
	assert.Equal(t, "my-registry-secret", secrets[0].Name)
}

func TestExternalClusterCronJob_WithProxy(t *testing.T) {
	m := testAuditConfig()
	cluster := v1alpha2.ExternalCluster{
		Name: "remote",
		KubeconfigSecretRef: &corev1.LocalObjectReference{
			Name: "kubeconfig-secret",
		},
	}
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:  ptr.To("http://proxy:8080"),
			HttpsProxy: ptr.To("https://proxy:8443"),
		},
	}

	cj := ExternalClusterCronJob("test-image:latest", cluster, m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	cmdStr := strings.Join(container.Command, " ")
	assert.Contains(t, cmdStr, "--api-proxy")
	assert.Contains(t, cmdStr, "https://proxy:8443")

	envMap := envToMap(container.Env)
	assert.Equal(t, "http://proxy:8080", envMap["HTTP_PROXY"])
	assert.Equal(t, "https://proxy:8443", envMap["HTTPS_PROXY"])
}

func TestExternalClusterCronJob_SkipProxy(t *testing.T) {
	m := testAuditConfig()
	cluster := v1alpha2.ExternalCluster{
		Name: "remote",
		KubeconfigSecretRef: &corev1.LocalObjectReference{
			Name: "kubeconfig-secret",
		},
	}
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:          ptr.To("http://proxy:8080"),
			SkipProxyForCnspec: true,
		},
	}

	cj := ExternalClusterCronJob("test-image:latest", cluster, m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	cmdStr := strings.Join(container.Command, " ")
	assert.NotContains(t, cmdStr, "--api-proxy")

	envMap := envToMap(container.Env)
	_, hasHTTPProxy := envMap["HTTP_PROXY"]
	assert.False(t, hasHTTPProxy)
}

func TestExternalClusterCronJob_ImagePullSecrets(t *testing.T) {
	m := testAuditConfig()
	cluster := v1alpha2.ExternalCluster{
		Name: "remote",
		KubeconfigSecretRef: &corev1.LocalObjectReference{
			Name: "kubeconfig-secret",
		},
	}
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "my-registry-secret"},
			},
		},
	}

	cj := ExternalClusterCronJob("test-image:latest", cluster, m, cfg)
	secrets := cj.Spec.JobTemplate.Spec.Template.Spec.ImagePullSecrets
	require.Len(t, secrets, 1)
	assert.Equal(t, "my-registry-secret", secrets[0].Name)
}

func TestInventory_WithContainerProxy(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
	}
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			ContainerProxy: ptr.To("http://container-proxy:3128"),
		},
	}

	invStr, err := Inventory("", testClusterUID, auditConfig, cfg)
	require.NoError(t, err)

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets)

	assert.Equal(t, "http://container-proxy:3128", inv.Spec.Assets[0].Connections[0].Options["container-proxy"])
}

func TestInventory_WithoutContainerProxy(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
	}

	invStr, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets)

	_, hasContainerProxy := inv.Spec.Assets[0].Connections[0].Options["container-proxy"]
	assert.False(t, hasContainerProxy)
}

func TestExternalClusterInventory_WithContainerProxy(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
	}
	cluster := v1alpha2.ExternalCluster{Name: "remote-cluster"}
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			ContainerProxy: ptr.To("http://container-proxy:3128"),
		},
	}

	invStr, err := ExternalClusterInventory("", testClusterUID, cluster, auditConfig, cfg)
	require.NoError(t, err)

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets)

	assert.Equal(t, "http://container-proxy:3128", inv.Spec.Assets[0].Connections[0].Options["container-proxy"])
}

// envToMap converts a slice of EnvVar to a map for easy lookup.
func envToMap(envVars []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envVars))
	for _, e := range envVars {
		m[e.Name] = e.Value
	}
	return m
}
