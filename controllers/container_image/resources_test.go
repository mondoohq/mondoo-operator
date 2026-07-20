// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package container_image

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
			Containers: v1alpha2.Containers{
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

func TestCronJob_WithProxy(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			HttpProxy:  ptr.To("http://proxy:8080"),
			HttpsProxy: ptr.To("https://proxy:8443"),
		},
	}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	cmdStr := strings.Join(container.Command, " ")
	assert.Contains(t, cmdStr, "--api-proxy")
	assert.Contains(t, cmdStr, "https://proxy:8443")

	envMap := envToMap(container.Env)
	assert.Equal(t, "http://proxy:8080", envMap["HTTP_PROXY"])
	assert.Equal(t, "https://proxy:8443", envMap["HTTPS_PROXY"])
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

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
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

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
	secrets := cj.Spec.JobTemplate.Spec.Template.Spec.ImagePullSecrets
	require.Len(t, secrets, 1)
	assert.Equal(t, "my-registry-secret", secrets[0].Name)
}

func TestCronJob_WithScheduling(t *testing.T) {
	m := testAuditConfig()
	m.Spec.Containers.Scheduling = v1alpha2.PodScheduling{
		NodeSelector: map[string]string{
			"nodepool": "scanners",
		},
		Tolerations: []corev1.Toleration{
			{
				Key:      "sriov",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
		},
	}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, v1alpha2.MondooOperatorConfig{})
	podSpec := cj.Spec.JobTemplate.Spec.Template.Spec

	assert.Equal(t, map[string]string{"nodepool": "scanners"}, podSpec.NodeSelector)
	assert.Equal(t, m.Spec.Containers.Scheduling.Tolerations, podSpec.Tolerations)
}

func TestCronJob_ImagePullSecrets_AppendsMultiple(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{
		Spec: v1alpha2.MondooOperatorConfigSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "secret-one"},
				{Name: "secret-two"},
			},
		},
	}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
	secrets := cj.Spec.JobTemplate.Spec.Template.Spec.ImagePullSecrets

	require.Len(t, secrets, 2)
	assert.Equal(t, "secret-one", secrets[0].Name)
	assert.Equal(t, "secret-two", secrets[1].Name)
}

func TestCronJob_PrivateRegistrySecretMountsDockerConfig(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", "", testClusterUID, "private-registry-secret", m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	// Private registry secret should be mounted as a Docker config volume, not as ImagePullSecrets
	envMap := envToMap(container.Env)
	assert.Equal(t, "/etc/opt/mondoo/docker", envMap["DOCKER_CONFIG"])

	found := false
	for _, vm := range container.VolumeMounts {
		if vm.Name == "pull-secrets" {
			found = true
			assert.Equal(t, "/etc/opt/mondoo/docker", vm.MountPath)
		}
	}
	assert.True(t, found, "pull-secrets volume mount should be present")
}

func TestCronJob_HasGOMEMLIMIT(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	envMap := envToMap(container.Env)
	assert.Equal(t, "900000000", envMap["GOMEMLIMIT"])
}

func TestCronJob_GOMEMLIMIT_CustomResources(t *testing.T) {
	m := testAuditConfig()
	m.Spec.Containers.Resources = corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	envMap := envToMap(container.Env)
	// 512Mi = 536870912 bytes, 90% = 483183820.8 → "483183821"
	assert.Equal(t, "483183821", envMap["GOMEMLIMIT"])
}

func TestCronJob_ActiveDeadline(t *testing.T) {
	m := testAuditConfig()
	m.Spec.Containers.ActiveDeadline = &metav1.Duration{Duration: 30 * time.Minute}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
	require.NotNil(t, cj.Spec.JobTemplate.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, int64(1800), *cj.Spec.JobTemplate.Spec.ActiveDeadlineSeconds)
}

func TestCronJob_ActiveDeadline_Unset(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
	assert.Nil(t, cj.Spec.JobTemplate.Spec.ActiveDeadlineSeconds)
}

func TestCronJob_HasMaxProviderConnections(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	envMap := envToMap(container.Env)
	assert.Equal(t, defaultMaxProviderConnections, envMap["MONDOO_MAX_PROVIDER_CONNECTIONS"])
}

func TestCronJob_HasReportTypeNone(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
	container := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	cmd := strings.Join(container.Command, " ")
	assert.Contains(t, cmd, "--report-type none")
}

func TestCronJob_HasMondooTmpDir(t *testing.T) {
	m := testAuditConfig()
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
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

func TestCronJob_WIF_GKE(t *testing.T) {
	m := testAuditConfig()
	m.Spec.Containers.WorkloadIdentity = &v1alpha2.WorkloadIdentityConfig{
		Provider: v1alpha2.CloudProviderGKE,
		GKE: &v1alpha2.GKEWorkloadIdentity{
			ProjectID:            "my-project",
			GoogleServiceAccount: "scanner@my-project.iam.gserviceaccount.com",
		},
	}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
	podSpec := cj.Spec.JobTemplate.Spec.Template.Spec

	// Should use WIF ServiceAccount
	assert.Equal(t, WIFServiceAccountName(m.Name), podSpec.ServiceAccountName)
	assert.True(t, *podSpec.AutomountServiceAccountToken)

	// Should have init container for registry creds
	require.Len(t, podSpec.InitContainers, 1)
	assert.Equal(t, "generate-registry-creds", podSpec.InitContainers[0].Name)
	assert.Contains(t, podSpec.InitContainers[0].Image, "google-cloud-cli")

	// Should have docker config volume
	found := false
	for _, v := range podSpec.Volumes {
		if v.Name == "docker-config" {
			found = true
			assert.NotNil(t, v.EmptyDir)
		}
	}
	assert.True(t, found, "docker-config volume should be present")

	// Main container should have DOCKER_CONFIG env
	envMap := envToMap(podSpec.Containers[0].Env)
	assert.Equal(t, "/etc/opt/mondoo/docker", envMap["DOCKER_CONFIG"])

	// Should NOT have pull-secrets volume (WIF replaces static secrets)
	for _, v := range podSpec.Volumes {
		assert.NotEqual(t, "pull-secrets", v.Name, "should not have pull-secrets volume when WIF is enabled")
	}
}

func TestCronJob_WIF_EKS(t *testing.T) {
	m := testAuditConfig()
	m.Spec.Containers.WorkloadIdentity = &v1alpha2.WorkloadIdentityConfig{
		Provider: v1alpha2.CloudProviderEKS,
		EKS: &v1alpha2.EKSWorkloadIdentity{
			Region:  "us-east-1",
			RoleARN: "arn:aws:iam::123456789012:role/ecr-reader",
		},
	}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
	podSpec := cj.Spec.JobTemplate.Spec.Template.Spec

	assert.Equal(t, WIFServiceAccountName(m.Name), podSpec.ServiceAccountName)
	require.Len(t, podSpec.InitContainers, 1)
	assert.Contains(t, podSpec.InitContainers[0].Image, "aws-cli")

	initEnv := envToMap(podSpec.InitContainers[0].Env)
	assert.Equal(t, "us-east-1", initEnv["AWS_REGION"])
	assert.Equal(t, "arn:aws:iam::123456789012:role/ecr-reader", initEnv["ROLE_ARN"])
}

func TestCronJob_WIF_AKS(t *testing.T) {
	m := testAuditConfig()
	m.Spec.Containers.WorkloadIdentity = &v1alpha2.WorkloadIdentityConfig{
		Provider: v1alpha2.CloudProviderAKS,
		AKS: &v1alpha2.AKSWorkloadIdentity{
			ClientID:    "client-id-123",
			TenantID:    "tenant-id-456",
			LoginServer: "myregistry.azurecr.io",
		},
	}
	cfg := v1alpha2.MondooOperatorConfig{}

	cj := CronJob("test-image:latest", "", testClusterUID, "", m, cfg)
	podSpec := cj.Spec.JobTemplate.Spec.Template.Spec

	assert.Equal(t, WIFServiceAccountName(m.Name), podSpec.ServiceAccountName)
	require.Len(t, podSpec.InitContainers, 1)
	assert.Contains(t, podSpec.InitContainers[0].Image, "azure-cli")

	initEnv := envToMap(podSpec.InitContainers[0].Env)
	assert.Equal(t, "myregistry.azurecr.io", initEnv["ACR_LOGIN_SERVER"])

	// AKS should add WIF pod label only on pod template, not on CronJob metadata
	assert.Equal(t, "true", cj.Spec.JobTemplate.Spec.Template.Labels["azure.workload.identity/use"])
	assert.Empty(t, cj.Labels["azure.workload.identity/use"])
	assert.Empty(t, cj.Spec.JobTemplate.Labels["azure.workload.identity/use"])
}

func TestCronJob_WIF_IgnoresPrivateRegistrySecret(t *testing.T) {
	m := testAuditConfig()
	m.Spec.Containers.WorkloadIdentity = &v1alpha2.WorkloadIdentityConfig{
		Provider: v1alpha2.CloudProviderGKE,
		GKE: &v1alpha2.GKEWorkloadIdentity{
			ProjectID:            "my-project",
			GoogleServiceAccount: "scanner@my-project.iam.gserviceaccount.com",
		},
	}
	cfg := v1alpha2.MondooOperatorConfig{}

	// Pass a private registry secret name — it should be ignored when WIF is active
	cj := CronJob("test-image:latest", "", testClusterUID, "my-private-secret", m, cfg)
	podSpec := cj.Spec.JobTemplate.Spec.Template.Spec

	// Should NOT mount the static pull-secrets volume
	for _, v := range podSpec.Volumes {
		assert.NotEqual(t, "pull-secrets", v.Name)
	}

	// Should have WIF init container instead
	require.Len(t, podSpec.InitContainers, 1)
	assert.Equal(t, "generate-registry-creds", podSpec.InitContainers[0].Name)
}

func TestWIFServiceAccount_GKE(t *testing.T) {
	m := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Containers: v1alpha2.Containers{
				WorkloadIdentity: &v1alpha2.WorkloadIdentityConfig{
					Provider: v1alpha2.CloudProviderGKE,
					GKE: &v1alpha2.GKEWorkloadIdentity{
						ProjectID:            "my-project",
						GoogleServiceAccount: "scanner@my-project.iam.gserviceaccount.com",
					},
				},
			},
		},
	}

	sa := WIFServiceAccount(m)
	assert.Equal(t, "mondoo-client-cr-wif", sa.Name)
	assert.Equal(t, "scanner@my-project.iam.gserviceaccount.com", sa.Annotations["iam.gke.io/gcp-service-account"])
}

func TestWIFServiceAccount_EKS(t *testing.T) {
	m := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Containers: v1alpha2.Containers{
				WorkloadIdentity: &v1alpha2.WorkloadIdentityConfig{
					Provider: v1alpha2.CloudProviderEKS,
					EKS: &v1alpha2.EKSWorkloadIdentity{
						Region:  "us-east-1",
						RoleARN: "arn:aws:iam::123456789012:role/ecr-reader",
					},
				},
			},
		},
	}

	sa := WIFServiceAccount(m)
	assert.Equal(t, "arn:aws:iam::123456789012:role/ecr-reader", sa.Annotations["eks.amazonaws.com/role-arn"])
}

func TestWIFServiceAccount_AKS(t *testing.T) {
	m := &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Containers: v1alpha2.Containers{
				WorkloadIdentity: &v1alpha2.WorkloadIdentityConfig{
					Provider: v1alpha2.CloudProviderAKS,
					AKS: &v1alpha2.AKSWorkloadIdentity{
						ClientID:    "client-id-123",
						TenantID:    "tenant-id-456",
						LoginServer: "myregistry.azurecr.io",
					},
				},
			},
		},
	}

	sa := WIFServiceAccount(m)
	assert.Equal(t, "client-id-123", sa.Annotations["azure.workload.identity/client-id"])
	assert.Equal(t, "true", sa.Labels["azure.workload.identity/use"])
}

func TestValidateContainerRegistryWIF(t *testing.T) {
	tests := []struct {
		name    string
		wif     *v1alpha2.WorkloadIdentityConfig
		wantErr bool
	}{
		{name: "nil is valid", wif: nil, wantErr: false},
		{
			name:    "gke without gke config",
			wif:     &v1alpha2.WorkloadIdentityConfig{Provider: v1alpha2.CloudProviderGKE},
			wantErr: true,
		},
		{
			name:    "eks without eks config",
			wif:     &v1alpha2.WorkloadIdentityConfig{Provider: v1alpha2.CloudProviderEKS},
			wantErr: true,
		},
		{
			name:    "aks without aks config",
			wif:     &v1alpha2.WorkloadIdentityConfig{Provider: v1alpha2.CloudProviderAKS},
			wantErr: true,
		},
		{
			name: "valid gke",
			wif: &v1alpha2.WorkloadIdentityConfig{
				Provider: v1alpha2.CloudProviderGKE,
				GKE:      &v1alpha2.GKEWorkloadIdentity{ProjectID: "p", GoogleServiceAccount: "sa@p.iam.gserviceaccount.com"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContainerRegistryWIF(tt.wif)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInventory_WithRepositoryFilters(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Containers: v1alpha2.Containers{
				Repositories: v1alpha2.FilteringSpec{
					Exclude: []string{"ghcr.io/third-party/*", "docker.io/library/*"},
				},
			},
		},
	}

	invStr, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets)

	opts := inv.Spec.Assets[0].Connections[0].Options
	assert.Equal(t, "ghcr.io/third-party/*,docker.io/library/*", opts["images-exclude"])
	_, hasInclude := opts["images"]
	assert.False(t, hasInclude, "images key should not be present when only exclude is set")
}

func TestInventory_WithRepositoryInclude(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
		Spec: v1alpha2.MondooAuditConfigSpec{
			Containers: v1alpha2.Containers{
				Repositories: v1alpha2.FilteringSpec{
					Include: []string{"gcr.io/my-project/*"},
				},
			},
		},
	}

	invStr, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets)

	opts := inv.Spec.Assets[0].Connections[0].Options
	assert.Equal(t, "gcr.io/my-project/*", opts["images"])
	_, hasExclude := opts["images-exclude"]
	assert.False(t, hasExclude, "images-exclude key should not be present when only include is set")
}

func TestInventory_WithoutRepositoryFilters(t *testing.T) {
	auditConfig := v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mondoo-client"},
	}

	invStr, err := Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.NotEmpty(t, inv.Spec.Assets)

	opts := inv.Spec.Assets[0].Connections[0].Options
	_, hasInclude := opts["images"]
	_, hasExclude := opts["images-exclude"]
	assert.False(t, hasInclude, "images key should not be present when no filters configured")
	assert.False(t, hasExclude, "images-exclude key should not be present when no filters configured")
}

// envToMap converts a slice of EnvVar to a map for easy lookup.
func envToMap(envVars []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envVars))
	for _, e := range envVars {
		m[e.Name] = e.Value
	}
	return m
}
