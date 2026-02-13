// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

func TestDeploymentName(t *testing.T) {
	assert.Equal(t, "my-config-resource-watcher", DeploymentName("my-config"))
}

func TestDeploymentLabels(t *testing.T) {
	config := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-config",
		},
	}

	labels := DeploymentLabels(config)
	assert.Equal(t, "mondoo-resource-watcher", labels["app"])
	assert.Equal(t, "my-config", labels["mondoo_cr"])
}

func TestDeployment(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable:           true,
					DebounceInterval: metav1.Duration{Duration: 30 * time.Second},
					ResourceTypes:    []string{"pods", "deployments"},
				},
			},
			Filtering: v1alpha2.Filtering{
				Namespaces: v1alpha2.FilteringSpec{
					Include: []string{"default", "kube-system"},
				},
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)

	assert.Equal(t, "my-config-resource-watcher", deployment.Name)
	assert.Equal(t, "mondoo-operator", deployment.Namespace)
	assert.Equal(t, int32(1), *deployment.Spec.Replicas)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))

	container := deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "mondoo-resource-watcher", container.Name)
	assert.Equal(t, "ghcr.io/mondoohq/cnspec:latest", container.Image)

	// Check command contains expected flags
	cmdStr := ""
	for _, c := range container.Command {
		cmdStr += c + " "
	}
	assert.Contains(t, cmdStr, "resource-watcher")
	assert.Contains(t, cmdStr, "--config")
	assert.Contains(t, cmdStr, "--debounce-interval")
	assert.Contains(t, cmdStr, "30s")
	assert.Contains(t, cmdStr, "--resource-types")
	assert.Contains(t, cmdStr, "pods,deployments")
	assert.Contains(t, cmdStr, "--namespaces")
	assert.Contains(t, cmdStr, "default,kube-system")
}

func TestDeployment_DefaultDebounceInterval(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable: true,
					// No DebounceInterval set - should use default
				},
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)

	container := deployment.Spec.Template.Spec.Containers[0]
	cmdStr := ""
	for _, c := range container.Command {
		cmdStr += c + " "
	}
	// Should contain default debounce interval (10s)
	assert.Contains(t, cmdStr, "--debounce-interval")
	assert.Contains(t, cmdStr, "10s")
}

func TestDeployment_WithHttpProxy(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable: true,
				},
			},
		},
	}

	proxyURL := "http://proxy.example.com:8080"
	operatorConfig := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy: &proxyURL,
		},
	}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)

	container := deployment.Spec.Template.Spec.Containers[0]
	cmdStr := ""
	for _, c := range container.Command {
		cmdStr += c + " "
	}
	assert.Contains(t, cmdStr, "--api-proxy")
	assert.Contains(t, cmdStr, "http://proxy.example.com:8080")
}

func TestDeployment_MinimumScanInterval(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable:              true,
					MinimumScanInterval: metav1.Duration{Duration: 5 * time.Minute},
				},
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)

	container := deployment.Spec.Template.Spec.Containers[0]
	cmdStr := ""
	for _, c := range container.Command {
		cmdStr += c + " "
	}
	assert.Contains(t, cmdStr, "--minimum-scan-interval")
	assert.Contains(t, cmdStr, "5m0s")
}

func TestDeployment_DefaultMinimumScanInterval(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable: true,
					// No MinimumScanInterval set - should use default (2m)
				},
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)

	container := deployment.Spec.Template.Spec.Containers[0]
	cmdStr := ""
	for _, c := range container.Command {
		cmdStr += c + " "
	}
	assert.Contains(t, cmdStr, "--minimum-scan-interval")
	assert.Contains(t, cmdStr, "2m0s")
}

func TestDeployment_WatchAllResources(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable:            true,
					WatchAllResources: true,
				},
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)

	container := deployment.Spec.Template.Spec.Containers[0]
	cmdStr := ""
	for _, c := range container.Command {
		cmdStr += c + " "
	}
	assert.Contains(t, cmdStr, "--watch-all-resources")
}

func TestDeployment_WithAnnotations(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable: true,
				},
			},
			Annotations: map[string]string{
				"env":  "prod",
				"team": "platform",
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)

	container := deployment.Spec.Template.Spec.Containers[0]
	cmd := container.Command

	// Find --annotation flags and collect their values
	annotationArgs := map[string]bool{}
	for i, arg := range cmd {
		if arg == "--annotation" && i+1 < len(cmd) {
			annotationArgs[cmd[i+1]] = true
		}
	}
	assert.True(t, annotationArgs["env=prod"], "expected --annotation env=prod")
	assert.True(t, annotationArgs["team=platform"], "expected --annotation team=platform")
}

func TestDeployment_HighPriorityByDefault(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable: true,
					// WatchAllResources defaults to false
				},
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)

	container := deployment.Spec.Template.Spec.Containers[0]
	cmdStr := ""
	for _, c := range container.Command {
		cmdStr += c + " "
	}
	// Should NOT contain --watch-all-resources when false (default)
	assert.NotContains(t, cmdStr, "--watch-all-resources")
}

func TestDeployment_WithClusterUIDAndIntegrationMRN(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable: true,
				},
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "//integration/mrn/123", "cluster-uid-456", config, operatorConfig)

	container := deployment.Spec.Template.Spec.Containers[0]
	cmdStr := ""
	for _, c := range container.Command {
		cmdStr += c + " "
	}
	assert.Contains(t, cmdStr, "--cluster-uid")
	assert.Contains(t, cmdStr, "cluster-uid-456")
	assert.Contains(t, cmdStr, "--integration-mrn")
	assert.Contains(t, cmdStr, "//integration/mrn/123")
}

func TestDeployment_EmptyClusterUIDAndIntegrationMRN(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable: true,
				},
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)

	container := deployment.Spec.Template.Spec.Containers[0]
	cmdStr := ""
	for _, c := range container.Command {
		cmdStr += c + " "
	}
	// Should NOT contain flags when empty
	assert.NotContains(t, cmdStr, "--cluster-uid")
	assert.NotContains(t, cmdStr, "--integration-mrn")
}

func TestDeployment_HttpsProxyPreferred(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable: true,
				},
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:  ptr.To("http://proxy:8080"),
			HttpsProxy: ptr.To("https://proxy:8443"),
		},
	}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)
	container := deployment.Spec.Template.Spec.Containers[0]

	cmdStr := strings.Join(container.Command, " ")
	assert.Contains(t, cmdStr, "--api-proxy https://proxy:8443")
	assert.NotContains(t, cmdStr, "http://proxy:8080")
}

func TestDeployment_SkipProxyForCnspec(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable: true,
				},
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:          ptr.To("http://proxy:8080"),
			HttpsProxy:         ptr.To("https://proxy:8443"),
			SkipProxyForCnspec: true,
		},
	}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)
	container := deployment.Spec.Template.Spec.Containers[0]

	cmdStr := strings.Join(container.Command, " ")
	assert.NotContains(t, cmdStr, "--api-proxy")

	envMap := envToMap(container.Env)
	_, hasHTTPProxy := envMap["HTTP_PROXY"]
	_, hasHTTPSProxy := envMap["HTTPS_PROXY"]
	assert.False(t, hasHTTPProxy, "HTTP_PROXY should not be set when SkipProxyForCnspec is true")
	assert.False(t, hasHTTPSProxy, "HTTPS_PROXY should not be set when SkipProxyForCnspec is true")
}

func TestDeployment_ProxyEnvVars(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable: true,
				},
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:  ptr.To("http://proxy:8080"),
			HttpsProxy: ptr.To("https://proxy:8443"),
			NoProxy:    ptr.To("localhost,10.0.0.0/8"),
		},
	}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)
	container := deployment.Spec.Template.Spec.Containers[0]

	envMap := envToMap(container.Env)
	assert.Equal(t, "http://proxy:8080", envMap["HTTP_PROXY"])
	assert.Equal(t, "http://proxy:8080", envMap["http_proxy"])
	assert.Equal(t, "https://proxy:8443", envMap["HTTPS_PROXY"])
	assert.Equal(t, "https://proxy:8443", envMap["https_proxy"])
	assert.Equal(t, "localhost,10.0.0.0/8", envMap["NO_PROXY"])
	assert.Equal(t, "localhost,10.0.0.0/8", envMap["no_proxy"])
}

func TestDeployment_WithImagePullSecrets(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			KubernetesResources: v1alpha2.KubernetesResources{
				Enable: true,
				ResourceWatcher: v1alpha2.ResourceWatcherSpec{
					Enable: true,
				},
			},
		},
	}

	operatorConfig := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "my-registry-secret"},
			},
		},
	}

	deployment := Deployment("ghcr.io/mondoohq/cnspec:latest", "", "", config, operatorConfig)
	secrets := deployment.Spec.Template.Spec.ImagePullSecrets
	require.Len(t, secrets, 1)
	assert.Equal(t, "my-registry-secret", secrets[0].Name)
}

// envToMap converts a slice of EnvVar to a map for easy lookup.
func envToMap(envVars []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envVars))
	for _, e := range envVars {
		m[e.Name] = e.Value
	}
	return m
}
