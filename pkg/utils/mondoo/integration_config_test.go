// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseK8sIntegrationConfig_FullConfig(t *testing.T) {
	raw := `{
		"scanNodes": true,
		"scanNodesStyle": "CRONJOB",
		"scanWorkloads": true,
		"scanPublicImages": true,
		"namespaceDenyList": ["kube-system"],
		"schedule": "30 * * * *",
		"nodesSchedule": "15 * * * *",
		"containersSchedule": "45 * * * *",
		"pauseScanning": false,
		"scannerReplicas": 2,
		"nodesResources": {
			"memRequest": "200Mi",
			"memLimit": "500Mi"
		},
		"resourceWatcher": {
			"enable": true,
			"debounceInterval": "15s",
			"minimumScanInterval": "3m"
		},
		"jobOverrides": {
			"ttlSecondsAfterFinished": 600,
			"annotations": {"app": "mondoo"},
			"nodeSelector": {"disk": "ssd"},
			"labels": {"team": "security"}
		},
		"assetAnnotations": {"env": "prod"},
		"spaceId": "space-123",
		"nodesPriorityClassName": "high-priority",
		"nodesIntervalTimer": 900,
		"scannerEnv": [{"name": "HTTP_PROXY", "value": "http://proxy:8080"}],
		"containerRepositoriesAllowList": ["docker.io/mondoo/*"],
		"containerRepositoriesDenyList": ["docker.io/test/*"],
		"scanCacheEnabled": true,
		"scanCacheTtl": "24h",
		"k8sActiveDeadline": 3600,
		"containersActiveDeadline": 7200
	}`

	cfg, err := ParseK8sIntegrationConfig([]byte(raw))
	require.NoError(t, err)

	assert.True(t, cfg.ScanNodes)
	assert.Equal(t, "CRONJOB", cfg.ScanNodesStyle)
	assert.True(t, cfg.ScanWorkloads)
	assert.True(t, cfg.ScanPublicImages)
	assert.Equal(t, []string{"kube-system"}, cfg.NamespaceDenyList)
	assert.Equal(t, "30 * * * *", cfg.Schedule)
	assert.Equal(t, "15 * * * *", cfg.NodesSchedule)
	assert.Equal(t, "45 * * * *", cfg.ContainersSchedule)
	assert.False(t, cfg.PauseScanning)
	assert.Equal(t, int32(2), cfg.ScannerReplicas)
	require.NotNil(t, cfg.NodesResources)
	assert.Equal(t, "200Mi", cfg.NodesResources.MemRequest)
	assert.Equal(t, "500Mi", cfg.NodesResources.MemLimit)
	require.NotNil(t, cfg.ResourceWatcher)
	assert.True(t, cfg.ResourceWatcher.Enable)
	assert.Equal(t, "15s", cfg.ResourceWatcher.DebounceInterval)
	require.NotNil(t, cfg.JobOverrides)
	assert.Equal(t, int32(600), cfg.JobOverrides.TTLSecondsAfterFinished)
	assert.Equal(t, map[string]string{"app": "mondoo"}, cfg.JobOverrides.Annotations)
	assert.Equal(t, map[string]string{"disk": "ssd"}, cfg.JobOverrides.NodeSelector)
	assert.Equal(t, map[string]string{"team": "security"}, cfg.JobOverrides.Labels)
	assert.Equal(t, map[string]string{"env": "prod"}, cfg.AssetAnnotations)
	assert.Equal(t, "space-123", cfg.SpaceID)
	assert.Equal(t, "high-priority", cfg.NodesPriorityClassName)
	assert.Equal(t, int32(900), cfg.NodesIntervalTimer)
	require.Len(t, cfg.ScannerEnv, 1)
	assert.Equal(t, "HTTP_PROXY", cfg.ScannerEnv[0].Name)
	assert.Equal(t, []string{"docker.io/mondoo/*"}, cfg.ContainerRepositoriesAllowList)
	assert.Equal(t, []string{"docker.io/test/*"}, cfg.ContainerRepositoriesDenyList)
	assert.True(t, cfg.ScanCacheEnabled)
	assert.Equal(t, "24h", cfg.ScanCacheTTL)
	assert.Equal(t, int64(3600), cfg.K8sActiveDeadline)
	assert.Equal(t, int64(7200), cfg.ContainersActiveDeadline)
}

func TestParseK8sIntegrationConfig_MinimalConfig(t *testing.T) {
	raw := `{"pauseScanning": true}`
	cfg, err := ParseK8sIntegrationConfig([]byte(raw))
	require.NoError(t, err)
	assert.True(t, cfg.PauseScanning)
	assert.Equal(t, int32(0), cfg.ScannerReplicas)
}

func TestParseK8sIntegrationConfig_InvalidJSON(t *testing.T) {
	_, err := ParseK8sIntegrationConfig([]byte(`{invalid`))
	assert.Error(t, err)
}
