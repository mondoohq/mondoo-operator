// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

func TestEffectiveSpec_RemoteManagedFalse(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{
		KubernetesResources: v1alpha2.KubernetesResources{Enable: true},
	}
	result, _, err := EffectiveSpec(spec, "", types.UID("test-uid"))
	require.NoError(t, err)
	assert.True(t, result.KubernetesResources.Enable)
}

func TestEffectiveSpec_EmptyRemoteConfig(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{
		RemoteManaged:       true,
		KubernetesResources: v1alpha2.KubernetesResources{Enable: true},
	}
	result, _, err := EffectiveSpec(spec, "", types.UID("test-uid"))
	require.NoError(t, err)
	assert.True(t, result.KubernetesResources.Enable)
}

func TestEffectiveSpec_FullMapping(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{
		RemoteManaged:        true,
		MondooCredsSecretRef: corev1.LocalObjectReference{Name: "my-creds"},
		MondooTokenSecretRef: corev1.LocalObjectReference{Name: "my-token"},
		ConsoleIntegration:   v1alpha2.ConsoleIntegration{Enable: true},
		Scanner: v1alpha2.Scanner{
			ServiceAccountName: "custom-sa",
			Image:              v1alpha2.Image{Name: "custom-image", Tag: "v1"},
		},
	}
	remoteConfig := `{
		"scanWorkloads": true,
		"scanNodes": true,
		"scanNodesStyle": "DEPLOYMENT",
		"scanPublicImages": true,
		"scanLocalCluster": true,
		"namespaceDenyList": ["kube-system", "monitoring"],
		"schedule": "30 * * * *",
		"nodesSchedule": "15 * * * *",
		"containersSchedule": "45 * * * *",
		"scannerReplicas": 3,
		"nodesResources": {"memRequest": "256Mi", "memLimit": "512Mi", "cpuRequest": "100m"},
		"resourceWatcher": {"enable": true, "debounceInterval": "15s", "minimumScanInterval": "3m"},
		"containerRepositoriesAllowList": ["docker.io/mondoo/*"],
		"containerRepositoriesDenyList": ["docker.io/test/*"],
		"scanCacheEnabled": true,
		"scanCacheTtl": "24h",
		"k8sActiveDeadline": 3600,
		"containersActiveDeadline": 7200,
		"jobOverrides": {"ttlSecondsAfterFinished": 600, "annotations": {"app": "mondoo"}, "labels": {"team": "sec"}},
		"assetAnnotations": {"env": "prod"},
		"spaceId": "space-123",
		"nodesPriorityClassName": "high-priority",
		"nodesIntervalTimer": 900,
		"scannerEnv": [{"name": "HTTP_PROXY", "value": "http://proxy:8080"}],
		"nodesEnv": [{"name": "NO_PROXY", "value": "localhost"}]
	}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)

	assert.True(t, result.KubernetesResources.Enable)
	assert.True(t, result.Nodes.Enable)
	assert.Equal(t, v1alpha2.NodeScanStyle("deployment"), result.Nodes.Style)
	assert.True(t, result.Containers.Enable)
	assert.Equal(t, []string{"kube-system", "monitoring"}, result.Filtering.Namespaces.Exclude)
	assert.Equal(t, "30 * * * *", result.KubernetesResources.Schedule)
	assert.Equal(t, "15 * * * *", result.Nodes.Schedule)
	assert.Equal(t, "45 * * * *", result.Containers.Schedule)
	require.NotNil(t, result.Scanner.Replicas)
	assert.Equal(t, int32(3), *result.Scanner.Replicas)
	assert.Equal(t, resource.MustParse("256Mi"), result.Nodes.Resources.Requests[corev1.ResourceMemory])
	assert.Equal(t, resource.MustParse("512Mi"), result.Nodes.Resources.Limits[corev1.ResourceMemory])
	assert.Equal(t, resource.MustParse("100m"), result.Nodes.Resources.Requests[corev1.ResourceCPU])
	assert.True(t, result.KubernetesResources.ResourceWatcher.Enable)
	assert.Equal(t, metav1.Duration{Duration: 15 * time.Second}, result.KubernetesResources.ResourceWatcher.DebounceInterval)
	assert.Equal(t, metav1.Duration{Duration: 3 * time.Minute}, result.KubernetesResources.ResourceWatcher.MinimumScanInterval)
	assert.Equal(t, []string{"docker.io/mondoo/*"}, result.Containers.Repositories.Include)
	assert.Equal(t, []string{"docker.io/test/*"}, result.Containers.Repositories.Exclude)
	require.NotNil(t, result.Containers.ScanCache)
	assert.True(t, result.Containers.ScanCache.Enable)
	require.NotNil(t, result.Containers.ScanCache.CacheTTL)
	assert.Equal(t, metav1.Duration{Duration: 24 * time.Hour}, *result.Containers.ScanCache.CacheTTL)
	require.NotNil(t, result.KubernetesResources.ActiveDeadline)
	assert.Equal(t, metav1.Duration{Duration: 3600 * time.Second}, *result.KubernetesResources.ActiveDeadline)
	require.NotNil(t, result.Containers.ActiveDeadline)
	assert.Equal(t, metav1.Duration{Duration: 7200 * time.Second}, *result.Containers.ActiveDeadline)
	require.NotNil(t, result.JobOverrides.TTLSecondsAfterFinished)
	assert.Equal(t, int32(600), *result.JobOverrides.TTLSecondsAfterFinished)
	assert.Equal(t, map[string]string{"app": "mondoo"}, result.JobOverrides.Annotations)
	assert.Equal(t, map[string]string{"team": "sec"}, result.JobOverrides.Labels)
	assert.Equal(t, map[string]string{"env": "prod"}, result.Annotations)
	assert.Equal(t, "space-123", result.SpaceID)
	assert.Equal(t, "high-priority", result.Nodes.PriorityClassName)
	assert.Equal(t, 900, result.Nodes.IntervalTimer)
	require.Len(t, result.Scanner.Env, 1)
	assert.Equal(t, "HTTP_PROXY", result.Scanner.Env[0].Name)
	require.Len(t, result.Nodes.Env, 1)
	assert.Equal(t, "NO_PROXY", result.Nodes.Env[0].Name)

	// D4 denylist: local-only fields preserved
	assert.Equal(t, "my-creds", result.MondooCredsSecretRef.Name)
	assert.Equal(t, "my-token", result.MondooTokenSecretRef.Name)
	assert.True(t, result.ConsoleIntegration.Enable)
	assert.Equal(t, "custom-sa", result.Scanner.ServiceAccountName)
	assert.Equal(t, "custom-image", result.Scanner.Image.Name)
	assert.True(t, result.RemoteManaged)
}

func TestEffectiveSpec_DefaultReplicas(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{"scanWorkloads": true, "scanLocalCluster": true, "scannerReplicas": 0}`
	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)
	require.NotNil(t, result.Scanner.Replicas)
	assert.Equal(t, int32(1), *result.Scanner.Replicas)
}

func TestEffectiveSpec_DefaultNodesStyle(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{"scanNodes": true, "scanLocalCluster": true, "scanNodesStyle": ""}`
	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)
	assert.Equal(t, v1alpha2.NodeScanStyle_CronJob, result.Nodes.Style)
}

func TestEffectiveSpec_ScanLocalClusterFalse(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{"scanWorkloads": true, "scanNodes": true, "scanLocalCluster": false}`
	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)
	assert.False(t, result.KubernetesResources.Enable)
	assert.False(t, result.Nodes.Enable)
}

func TestEffectiveSpec_DeterministicScheduleFromUID(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{"scanWorkloads": true, "scanNodes": true, "scanPublicImages": true, "scanLocalCluster": true}`
	uid := types.UID("550e8400-e29b-41d4-a716-446655440000")

	result1, _, err := EffectiveSpec(spec, remoteConfig, uid)
	require.NoError(t, err)
	result2, _, err := EffectiveSpec(spec, remoteConfig, uid)
	require.NoError(t, err)

	assert.Equal(t, result1.KubernetesResources.Schedule, result2.KubernetesResources.Schedule)
	assert.Equal(t, result1.Nodes.Schedule, result2.Nodes.Schedule)
	assert.Equal(t, result1.Containers.Schedule, result2.Containers.Schedule)

	assert.Regexp(t, `^\d+ \* \* \* \*$`, result1.KubernetesResources.Schedule)

	assert.NotEqual(t, result1.KubernetesResources.Schedule, result1.Nodes.Schedule)
}

func TestEffectiveSpec_InvalidRemoteConfig(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	_, _, err := EffectiveSpec(spec, `{invalid`, types.UID("test-uid"))
	assert.Error(t, err)
}

func TestEffectiveSpec_ExternalClusters(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{
		"scanWorkloads": true,
		"scanLocalCluster": true,
		"externalClusters": [
			{
				"name": "prod-east",
				"kubeconfigSecret": "prod-east-kubeconfig",
				"containerImageScanning": true,
				"namespaceAllowList": ["default", "app"]
			}
		]
	}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)
	require.Len(t, result.KubernetesResources.ExternalClusters, 1)
	assert.Equal(t, "prod-east", result.KubernetesResources.ExternalClusters[0].Name)
	require.NotNil(t, result.KubernetesResources.ExternalClusters[0].KubeconfigSecretRef)
	assert.Equal(t, "prod-east-kubeconfig", result.KubernetesResources.ExternalClusters[0].KubeconfigSecretRef.Name)
	assert.True(t, result.KubernetesResources.ExternalClusters[0].ContainerImageScanning)
	require.NotNil(t, result.KubernetesResources.ExternalClusters[0].Filtering)
	assert.Equal(t, []string{"default", "app"}, result.KubernetesResources.ExternalClusters[0].Filtering.Namespaces.Include)
}

func TestEffectiveSpec_ContainersWif(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{
		"scanPublicImages": true,
		"scanLocalCluster": true,
		"containersWif": {
			"provider": "GKE",
			"gke": {
				"projectId": "my-project",
				"clusterName": "my-cluster",
				"clusterLocation": "us-central1",
				"googleServiceAccount": "sa@my-project.iam.gserviceaccount.com"
			}
		}
	}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)
	require.NotNil(t, result.Containers.WorkloadIdentity)
	assert.Equal(t, v1alpha2.CloudProviderGKE, result.Containers.WorkloadIdentity.Provider)
	require.NotNil(t, result.Containers.WorkloadIdentity.GKE)
	assert.Equal(t, "my-project", result.Containers.WorkloadIdentity.GKE.ProjectID)
	assert.Equal(t, "my-cluster", result.Containers.WorkloadIdentity.GKE.ClusterName)
}

// D3: Server is authoritative — remote config overrides ALL local spec values for mapped fields
func TestEffectiveSpec_D3_ServerOverridesLocalSpec(t *testing.T) {
	localReplicas := int32(5)
	spec := v1alpha2.MondooAuditConfigSpec{
		RemoteManaged: true,
		Scanner: v1alpha2.Scanner{
			Replicas: &localReplicas,
		},
		KubernetesResources: v1alpha2.KubernetesResources{
			Enable:   true,
			Schedule: "0 */2 * * *",
		},
		Nodes: v1alpha2.Nodes{
			Enable:   true,
			Style:    v1alpha2.NodeScanStyle_DaemonSet,
			Schedule: "10 * * * *",
		},
		Filtering: v1alpha2.Filtering{
			Namespaces: v1alpha2.FilteringSpec{
				Exclude: []string{"local-only-ns"},
			},
		},
	}
	remoteConfig := `{
		"scanWorkloads": true,
		"scanNodes": true,
		"scanLocalCluster": true,
		"scanNodesStyle": "CRONJOB",
		"scannerReplicas": 2,
		"schedule": "30 * * * *",
		"nodesSchedule": "45 * * * *",
		"namespaceDenyList": ["kube-system"]
	}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)

	// Server values win, not local
	require.NotNil(t, result.Scanner.Replicas)
	assert.Equal(t, int32(2), *result.Scanner.Replicas)
	assert.Equal(t, "30 * * * *", result.KubernetesResources.Schedule)
	assert.Equal(t, "45 * * * *", result.Nodes.Schedule)
	assert.Equal(t, v1alpha2.NodeScanStyle_CronJob, result.Nodes.Style)
	assert.Equal(t, []string{"kube-system"}, result.Filtering.Namespaces.Exclude)
}

// D4: Remote config can never override local-only fields — even if proto had them
func TestEffectiveSpec_D4_DenylistFieldsAlwaysLocal(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{
		RemoteManaged:        true,
		MondooCredsSecretRef: corev1.LocalObjectReference{Name: "prod-creds"},
		MondooTokenSecretRef: corev1.LocalObjectReference{Name: "prod-token"},
		ConsoleIntegration:   v1alpha2.ConsoleIntegration{Enable: true},
		Scanner: v1alpha2.Scanner{
			ServiceAccountName: "prod-sa",
			Image:              v1alpha2.Image{Name: "ghcr.io/mondoo/cnspec", Tag: "v11.0.0"},
		},
	}
	// Even a full config cannot override D4 fields
	remoteConfig := `{"scanWorkloads": true, "scanLocalCluster": true, "scannerReplicas": 1}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)

	assert.Equal(t, "prod-creds", result.MondooCredsSecretRef.Name)
	assert.Equal(t, "prod-token", result.MondooTokenSecretRef.Name)
	assert.True(t, result.ConsoleIntegration.Enable)
	assert.Equal(t, "prod-sa", result.Scanner.ServiceAccountName)
	assert.Equal(t, "ghcr.io/mondoo/cnspec", result.Scanner.Image.Name)
	assert.Equal(t, "v11.0.0", result.Scanner.Image.Tag)
	assert.True(t, result.RemoteManaged)
}

// D3: remoteManaged flipped from true → false → local spec used, remoteConfig ignored
func TestEffectiveSpec_RemoteManagedFlippedOff(t *testing.T) {
	localReplicas := int32(5)
	spec := v1alpha2.MondooAuditConfigSpec{
		RemoteManaged: false,
		Scanner:       v1alpha2.Scanner{Replicas: &localReplicas},
		KubernetesResources: v1alpha2.KubernetesResources{
			Enable:   true,
			Schedule: "15 * * * *",
		},
	}
	// Remote config exists in status but should be ignored
	remoteConfig := `{"scanWorkloads": true, "scannerReplicas": 1, "schedule": "30 * * * *"}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)

	// Local spec used entirely
	require.NotNil(t, result.Scanner.Replicas)
	assert.Equal(t, int32(5), *result.Scanner.Replicas)
	assert.Equal(t, "15 * * * *", result.KubernetesResources.Schedule)
	assert.True(t, result.KubernetesResources.Enable)
}

// D8: Explicit server schedule takes precedence over jitter generation
func TestEffectiveSpec_D8_ExplicitScheduleWinsOverJitter(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{
		"scanWorkloads": true,
		"scanNodes": true,
		"scanPublicImages": true,
		"scanLocalCluster": true,
		"schedule": "0 2 * * *",
		"nodesSchedule": "30 3 * * *"
	}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)

	assert.Equal(t, "0 2 * * *", result.KubernetesResources.Schedule)
	assert.Equal(t, "30 3 * * *", result.Nodes.Schedule)
	// Containers schedule empty from server → deterministic jitter
	assert.Regexp(t, `^\d+ \* \* \* \*$`, result.Containers.Schedule)
}

// D8: Different UIDs produce different schedules
func TestEffectiveSpec_D8_DifferentUIDsDifferentSchedules(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{"scanWorkloads": true, "scanLocalCluster": true}`

	result1, _, err := EffectiveSpec(spec, remoteConfig, types.UID("uid-aaa"))
	require.NoError(t, err)
	result2, _, err := EffectiveSpec(spec, remoteConfig, types.UID("uid-zzz"))
	require.NoError(t, err)

	assert.NotEqual(t, result1.KubernetesResources.Schedule, result2.KubernetesResources.Schedule)
}

// scanLocalCluster gates both k8s resources and nodes, but NOT containers
func TestEffectiveSpec_ScanLocalClusterGatingBehavior(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{
		"scanWorkloads": true,
		"scanNodes": true,
		"scanPublicImages": true,
		"scanLocalCluster": false
	}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)

	assert.False(t, result.KubernetesResources.Enable, "scanLocalCluster=false disables k8s resources")
	assert.False(t, result.Nodes.Enable, "scanLocalCluster=false disables nodes")
	assert.True(t, result.Containers.Enable, "scanLocalCluster does not affect containers")
}

// Partial config — only some fields set, rest should be zero-values
func TestEffectiveSpec_PartialConfig(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{"scanNodes": true, "scanLocalCluster": true}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)

	assert.True(t, result.Nodes.Enable)
	assert.False(t, result.KubernetesResources.Enable)
	assert.False(t, result.Containers.Enable)
	assert.Nil(t, result.Containers.ScanCache)
	assert.Nil(t, result.KubernetesResources.ActiveDeadline)
	assert.Nil(t, result.Containers.ActiveDeadline)
	assert.Empty(t, result.Annotations)
	assert.Empty(t, result.SpaceID)
	assert.Empty(t, result.Scanner.Env)
}

// Per-scan-type job overrides mapped independently
func TestEffectiveSpec_PerScanTypeJobOverrides(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{
		"scanWorkloads": true,
		"scanNodes": true,
		"scanPublicImages": true,
		"scanLocalCluster": true,
		"jobOverrides": {"annotations": {"global": "true"}},
		"scannerJobOverrides": {"annotations": {"scanner": "true"}},
		"nodesJobOverrides": {"labels": {"node-scan": "true"}},
		"containersJobOverrides": {"nodeSelector": {"pool": "scan"}}
	}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"global": "true"}, result.JobOverrides.Annotations)
	assert.Equal(t, map[string]string{"scanner": "true"}, result.KubernetesResources.JobOverrides.Annotations)
	assert.Equal(t, map[string]string{"node-scan": "true"}, result.Nodes.JobOverrides.Labels)
	assert.Equal(t, map[string]string{"pool": "scan"}, result.Containers.JobOverrides.NodeSelector)
}

// Unrecognized scanNodesStyle defaults to cronjob
func TestEffectiveSpec_UnknownNodesScanStyleDefaultsToCronJob(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{"scanNodes": true, "scanLocalCluster": true, "scanNodesStyle": "UNKNOWN_FUTURE_STYLE"}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)
	assert.Equal(t, v1alpha2.NodeScanStyle_CronJob, result.Nodes.Style)
}

// Private registries pull secret refs mapped correctly
func TestEffectiveSpec_PrivateRegistries(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{
		"scanPublicImages": true,
		"scanLocalCluster": true,
		"privateRegistriesPullSecretRefs": ["registry-creds-1", "registry-creds-2"]
	}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)
	require.Len(t, result.Scanner.PrivateRegistriesPullSecretRefs, 2)
	assert.Equal(t, "registry-creds-1", result.Scanner.PrivateRegistriesPullSecretRefs[0].Name)
	assert.Equal(t, "registry-creds-2", result.Scanner.PrivateRegistriesPullSecretRefs[1].Name)
}

// All env vars mapped to all scan types
func TestEffectiveSpec_AllEnvVars(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{
		"scanWorkloads": true,
		"scanNodes": true,
		"scanPublicImages": true,
		"scanLocalCluster": true,
		"scannerEnv": [{"name": "SCANNER_VAR", "value": "s"}],
		"nodesEnv": [{"name": "NODES_VAR", "value": "n"}, {"name": "EXTRA", "value": "e"}],
		"containersEnv": [{"name": "CONTAINERS_VAR", "value": "c"}]
	}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)

	require.Len(t, result.Scanner.Env, 1)
	assert.Equal(t, "SCANNER_VAR", result.Scanner.Env[0].Name)
	require.Len(t, result.Nodes.Env, 2)
	assert.Equal(t, "NODES_VAR", result.Nodes.Env[0].Name)
	assert.Equal(t, "EXTRA", result.Nodes.Env[1].Name)
	require.Len(t, result.Containers.Env, 1)
	assert.Equal(t, "CONTAINERS_VAR", result.Containers.Env[0].Name)
}

// ScanCache disabled means no ScanCache struct in result
func TestEffectiveSpec_ScanCacheDisabled(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{"scanPublicImages": true, "scanLocalCluster": true, "scanCacheEnabled": false}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)
	assert.Nil(t, result.Containers.ScanCache)
}

// ScanCache enabled without TTL — TTL pointer should be nil
func TestEffectiveSpec_ScanCacheEnabledNoTTL(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{"scanPublicImages": true, "scanLocalCluster": true, "scanCacheEnabled": true}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)
	require.NotNil(t, result.Containers.ScanCache)
	assert.True(t, result.Containers.ScanCache.Enable)
	assert.Nil(t, result.Containers.ScanCache.CacheTTL)
}

// All resource types can have resources specified
func TestEffectiveSpec_AllResourceTypes(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{
		"scanWorkloads": true,
		"scanNodes": true,
		"scanPublicImages": true,
		"scanLocalCluster": true,
		"scannerResources": {"cpuRequest": "100m", "cpuLimit": "500m", "memRequest": "128Mi", "memLimit": "256Mi"},
		"nodesResources": {"cpuRequest": "200m", "memRequest": "256Mi"},
		"containersResources": {"memLimit": "1Gi"}
	}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)

	assert.Equal(t, resource.MustParse("100m"), result.Scanner.Resources.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("500m"), result.Scanner.Resources.Limits[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("128Mi"), result.Scanner.Resources.Requests[corev1.ResourceMemory])
	assert.Equal(t, resource.MustParse("256Mi"), result.Scanner.Resources.Limits[corev1.ResourceMemory])
	assert.Equal(t, resource.MustParse("200m"), result.Nodes.Resources.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("256Mi"), result.Nodes.Resources.Requests[corev1.ResourceMemory])
	assert.Equal(t, resource.MustParse("1Gi"), result.Containers.Resources.Limits[corev1.ResourceMemory])
}

// EKS WIF for containers
func TestEffectiveSpec_ContainersWifEKS(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{
		"scanPublicImages": true,
		"scanLocalCluster": true,
		"containersWif": {
			"provider": "EKS",
			"eks": {
				"region": "us-west-2",
				"clusterName": "prod",
				"roleArn": "arn:aws:iam::123456:role/scan"
			}
		}
	}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)
	require.NotNil(t, result.Containers.WorkloadIdentity)
	assert.Equal(t, v1alpha2.CloudProviderEKS, result.Containers.WorkloadIdentity.Provider)
	require.NotNil(t, result.Containers.WorkloadIdentity.EKS)
	assert.Equal(t, "us-west-2", result.Containers.WorkloadIdentity.EKS.Region)
	assert.Equal(t, "arn:aws:iam::123456:role/scan", result.Containers.WorkloadIdentity.EKS.RoleARN)
}

func TestEffectiveSpec_JobOverridesTolerations(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{
		"scanWorkloads": true,
		"scanLocalCluster": true,
		"jobOverrides": {
			"tolerations": [
				{"key": "node.kubernetes.io/not-ready", "operator": "Exists", "effect": "NoSchedule"}
			]
		}
	}`

	result, _, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)
	require.Len(t, result.JobOverrides.Tolerations, 1)
	assert.Equal(t, "node.kubernetes.io/not-ready", result.JobOverrides.Tolerations[0].Key)
	assert.Equal(t, corev1.TolerationOpExists, result.JobOverrides.Tolerations[0].Operator)
	assert.Equal(t, corev1.TaintEffectNoSchedule, result.JobOverrides.Tolerations[0].Effect)
}

func TestEffectiveSpec_InvalidResourceQuantityWarning(t *testing.T) {
	spec := v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}
	remoteConfig := `{
		"scanWorkloads": true,
		"scanLocalCluster": true,
		"scannerResources": {"cpuRequest": "500m", "cpuLimit": "1", "memLimit": "1Bs"}
	}`

	result, warnings, err := EffectiveSpec(spec, remoteConfig, types.UID("test-uid"))
	require.NoError(t, err)

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "scannerResources.memLimit")
	assert.Contains(t, warnings[0], "1Bs")

	assert.Equal(t, resource.MustParse("500m"), result.Scanner.Resources.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("1"), result.Scanner.Resources.Limits[corev1.ResourceCPU])
	_, hasMemLimit := result.Scanner.Resources.Limits[corev1.ResourceMemory]
	assert.False(t, hasMemLimit, "invalid memLimit should be skipped")
}
