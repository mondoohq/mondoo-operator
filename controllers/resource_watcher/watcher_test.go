// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
)

func TestNewResourceWatcher_NormalizesAndDeduplicatesResourceTypes(t *testing.T) {
	w := NewResourceWatcher(nil, nil, WatcherConfig{
		ResourceTypes: []string{"Deployment", "deployments", " pod ", "pods", "CronJob", "cronjobs"},
	})

	assert.Equal(t, []string{"deployments", "pods", "cronjobs"}, w.config.ResourceTypes)
}

func TestNewResourceWatcher_DefaultHighPriorityResourceTypes(t *testing.T) {
	before := slices.Clone(HighPriorityResourceTypes)
	w := NewResourceWatcher(nil, nil, WatcherConfig{})

	assert.Equal(t, before, w.config.ResourceTypes)
	w.config.ResourceTypes[0] = "changed"
	assert.Equal(t, before, HighPriorityResourceTypes)
}

func TestGetObjectForResourceType_AcceptsSingularAndPlural(t *testing.T) {
	w := &ResourceWatcher{}

	deploymentObj, err := w.getObjectForResourceType("deployment")
	assert.NoError(t, err)
	assert.IsType(t, &appsv1.Deployment{}, deploymentObj)

	cronjobObj, err := w.getObjectForResourceType("cronjobs")
	assert.NoError(t, err)
	assert.IsType(t, &batchv1.CronJob{}, cronjobObj)
}

func BenchmarkNormalizeResourceTypes_WithDuplicates(b *testing.B) {
	resourceTypes := make([]string, 0, 12000)
	for i := 0; i < 2000; i++ {
		resourceTypes = append(resourceTypes, "deployment", "deployments", "pod", "pods", "CronJob", "cronjobs")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = normalizeResourceTypes(resourceTypes)
	}
}

func TestNewResourceWatcher_PreservesUnknownTypesForStartupError(t *testing.T) {
	w := NewResourceWatcher(nil, nil, WatcherConfig{ResourceTypes: []string{"deplyoments"}})
	assert.Equal(t, []string{"deplyoments"}, w.config.ResourceTypes)
	_, err := w.getObjectForResourceType(w.config.ResourceTypes[0])
	assert.EqualError(t, err, "unknown resource type: deplyoments")
}
