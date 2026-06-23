// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mql/v13/providers-sdk/v1/inventory"
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

func TestExternalClusterInventory_InheritsGlobalNamespaceFiltering(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Filtering: v1alpha2.Filtering{
				Namespaces: v1alpha2.FilteringSpec{
					Include: []string{"production", "shared"},
					Exclude: []string{"kube-system"},
				},
			},
		},
	}
	cluster := v1alpha2.ExternalCluster{Name: "remote-cluster"}

	options := externalClusterInventoryOptions(t, auditConfig, cluster)

	assert.Equal(t, "production,shared", options["namespaces"])
	assert.Equal(t, "kube-system", options["namespaces-exclude"])
}

func TestExternalClusterInventory_UsesClusterNamespaceFiltering(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Filtering: v1alpha2.Filtering{
				Namespaces: v1alpha2.FilteringSpec{
					Include: []string{"production"},
				},
			},
		},
	}
	cluster := v1alpha2.ExternalCluster{
		Name: "remote-cluster",
		Filtering: &v1alpha2.Filtering{
			Namespaces: v1alpha2.FilteringSpec{
				Exclude: []string{"kube-system", "monitoring"},
			},
		},
	}

	options := externalClusterInventoryOptions(t, auditConfig, cluster)

	assert.Empty(t, options["namespaces"])
	assert.Equal(t, "kube-system,monitoring", options["namespaces-exclude"])
}

func TestExternalClusterInventory_EmptyClusterNamespaceFilteringOverridesGlobal(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Filtering: v1alpha2.Filtering{
				Namespaces: v1alpha2.FilteringSpec{
					Include: []string{"production"},
				},
			},
		},
	}
	cluster := v1alpha2.ExternalCluster{
		Name:      "remote-cluster",
		Filtering: &v1alpha2.Filtering{},
	}

	options := externalClusterInventoryOptions(t, auditConfig, cluster)

	assert.Empty(t, options["namespaces"])
	assert.Empty(t, options["namespaces-exclude"])
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

func TestCronJob_HasReportTypeNone(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	cmd := strings.Join(container.Command, " ")
	assert.Contains(t, cmd, "--report-type none")
}

func TestExternalClusterCronJob_HasReportTypeNone(t *testing.T) {
	m := testAuditConfig()
	cluster := v1alpha2.ExternalCluster{
		Name:                "remote",
		KubeconfigSecretRef: &corev1.LocalObjectReference{Name: "kubeconfig-secret"},
	}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := ExternalClusterCronJob("test-image:latest", cluster, m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	cmd := strings.Join(container.Command, " ")
	assert.Contains(t, cmd, "--report-type none")
}

func TestCronJob_HasGOMEMLIMIT(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	envMap := envToMap(container.Env)
	// Default k8s resource scanning memory limit is 1G; 90% = 900000000
	assert.Equal(t, "900000000", envMap["GOMEMLIMIT"])
}

func TestCronJob_GOMEMLIMIT_CustomResources(t *testing.T) {
	m := testAuditConfig()
	m.Spec.Scanner.Resources = corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	envMap := envToMap(container.Env)
	assert.Equal(t, "483183821", envMap["GOMEMLIMIT"])
}

func TestCronJob_ActiveDeadline(t *testing.T) {
	m := testAuditConfig()
	m.Spec.KubernetesResources.ActiveDeadline = &metav1.Duration{Duration: 30 * time.Minute}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", m, cfg)
	require.NotNil(t, cj.Spec.JobTemplate.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, int64(1800), *cj.Spec.JobTemplate.Spec.ActiveDeadlineSeconds)
}

func TestCronJob_ActiveDeadline_Unset(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", m, cfg)
	assert.Nil(t, cj.Spec.JobTemplate.Spec.ActiveDeadlineSeconds)
}

func TestExternalClusterCronJob_HasGOMEMLIMIT(t *testing.T) {
	m := testAuditConfig()
	cluster := v1alpha2.ExternalCluster{
		Name:                "remote",
		KubeconfigSecretRef: &corev1.LocalObjectReference{Name: "kubeconfig-secret"},
	}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := ExternalClusterCronJob("test-image:latest", cluster, m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	envMap := envToMap(container.Env)
	assert.Equal(t, "900000000", envMap["GOMEMLIMIT"])
}

func TestExternalClusterCronJob_ActiveDeadline(t *testing.T) {
	m := testAuditConfig()
	m.Spec.KubernetesResources.ActiveDeadline = &metav1.Duration{Duration: 1 * time.Hour}
	cluster := v1alpha2.ExternalCluster{
		Name:                "remote",
		KubeconfigSecretRef: &corev1.LocalObjectReference{Name: "kubeconfig-secret"},
	}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := ExternalClusterCronJob("test-image:latest", cluster, m, cfg)
	require.NotNil(t, cj.Spec.JobTemplate.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, int64(3600), *cj.Spec.JobTemplate.Spec.ActiveDeadlineSeconds)
}

func TestExternalClusterCronJob_ActiveDeadline_Unset(t *testing.T) {
	m := testAuditConfig()
	cluster := v1alpha2.ExternalCluster{
		Name:                "remote",
		KubeconfigSecretRef: &corev1.LocalObjectReference{Name: "kubeconfig-secret"},
	}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := ExternalClusterCronJob("test-image:latest", cluster, m, cfg)
	assert.Nil(t, cj.Spec.JobTemplate.Spec.ActiveDeadlineSeconds)
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

func TestExternalClusterCronJob_HasMondooTmpDir(t *testing.T) {
	m := testAuditConfig()
	cluster := v1alpha2.ExternalCluster{
		Name: "remote",
		KubeconfigSecretRef: &corev1.LocalObjectReference{
			Name: "kubeconfig-secret",
		},
	}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := ExternalClusterCronJob("test-image:latest", cluster, m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	envMap := envToMap(container.Env)
	assert.Equal(t, "/tmp", envMap["MONDOO_TMP_DIR"])
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

func externalClusterInventoryOptions(t *testing.T, auditConfig v1alpha2.MondooAuditConfig, cluster v1alpha2.ExternalCluster) map[string]string {
	t.Helper()

	invStr, err := ExternalClusterInventory("", testClusterUID, cluster, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets)
	require.NotEmpty(t, inv.Spec.Assets[0].Connections)

	return inv.Spec.Assets[0].Connections[0].Options
}

// envToMap converts a slice of EnvVar to a map for easy lookup.
func envToMap(envVars []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envVars))
	for _, e := range envVars {
		m[e.Name] = e.Value
	}
	return m
}
