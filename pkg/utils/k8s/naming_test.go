// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCronJobName_Default(t *testing.T) {
	assert.Equal(t, "mondoo-container-scan", CronJobName("container-scan", "mondoo-client"))
	assert.Equal(t, "mondoo-k8s-scan", CronJobName("k8s-scan", "mondoo-client"))
}

func TestCronJobName_WithShortIntegrationID(t *testing.T) {
	name := CronJobName("container-scan", "mondoo-client-abc123")
	assert.Equal(t, "mondoo-container-scan-abc123", name)
	assert.LessOrEqual(t, len(name), ResourceNameMaxLength)
}

func TestCronJobName_WithLongIntegrationID(t *testing.T) {
	name := CronJobName("container-scan", "mondoo-client-3drsen921s8x4dksnxthd5sedc4")
	assert.LessOrEqual(t, len(name), ResourceNameMaxLength)
	assert.Contains(t, name, "mondoo-container-scan-")
	assert.Contains(t, name, "3drsen921s8x4dk")
}

func TestCronJobName_SameHashAcrossTypes(t *testing.T) {
	containerName := CronJobName("container-scan", "mondoo-client-3drsen921s8x4dksnxthd5sedc4")
	k8sName := CronJobName("k8s-scan", "mondoo-client-3drsen921s8x4dksnxthd5sedc4")

	// Both should contain the same hash of the integration ID
	// Extract last 8 chars (the hash) before any cluster name
	containerParts := containerName[len("mondoo-container-scan-"):]
	k8sParts := k8sName[len("mondoo-k8s-scan-"):]

	containerHash := containerParts[len(containerParts)-8:]
	k8sHash := k8sParts[len(k8sParts)-8:]
	assert.Equal(t, containerHash, k8sHash)
}

func TestCronJobName_Deterministic(t *testing.T) {
	name1 := CronJobName("container-scan", "mondoo-client-3drsen921s8x4dksnxthd5sedc4")
	name2 := CronJobName("container-scan", "mondoo-client-3drsen921s8x4dksnxthd5sedc4")
	assert.Equal(t, name1, name2)
}

func TestCronJobNameWithCluster_Default(t *testing.T) {
	name := CronJobNameWithCluster("k8s-scan", "mondoo-client", "target-cluster")
	assert.Equal(t, "mondoo-k8s-scan-target-cluster", name)
}

func TestCronJobNameWithCluster_WithIntegrationID(t *testing.T) {
	name := CronJobNameWithCluster("k8s-scan", "mondoo-client-abc123", "target")
	assert.Equal(t, "mondoo-k8s-scan-abc123-target", name)
	assert.LessOrEqual(t, len(name), ResourceNameMaxLength)
}

func TestCronJobNameWithCluster_Long(t *testing.T) {
	name := CronJobNameWithCluster("k8s-scan", "mondoo-client-3drsen921s8x4dksnxthd5sedc4", "target-cluster")
	assert.LessOrEqual(t, len(name), ResourceNameMaxLength)
	assert.Contains(t, name, "mondoo-k8s-scan-")
	assert.Contains(t, name, "target-cluster")
}

func TestCronJobNameWithCluster_LongClusterName(t *testing.T) {
	name := CronJobNameWithCluster("k8s-scan", "mondoo-client", "my-extremely-long-gke-cluster-name-that-exceeds-the-limit")
	assert.LessOrEqual(t, len(name), ResourceNameMaxLength)
	assert.Contains(t, name, "mondoo-k8s-scan-")
}

func TestCronJobNameWithCluster_LongBoth(t *testing.T) {
	name := CronJobNameWithCluster("k8s-scan", "mondoo-client-3drsen921s8x4dksnxthd5sedc4", "my-very-long-gke-cluster-name-in-production")
	assert.LessOrEqual(t, len(name), ResourceNameMaxLength)
	assert.Contains(t, name, "mondoo-k8s-scan-")
}

func TestCronJobName_NonStandardName(t *testing.T) {
	name := CronJobName("k8s-scan", "my-custom-config")
	assert.Equal(t, "mondoo-k8s-scan-my-custom-config", name)
}

func TestIntegrationID(t *testing.T) {
	assert.Equal(t, "", integrationID("mondoo-client"))
	assert.Equal(t, "abc123", integrationID("mondoo-client-abc123"))
	assert.Equal(t, "my-config", integrationID("my-config"))
}
