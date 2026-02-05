// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDebouncer_Add(t *testing.T) {
	var received []K8sResourceIdentifier
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, resources []K8sResourceIdentifier) error {
		mu.Lock()
		received = resources
		mu.Unlock()
		return nil
	}

	d := NewDebouncer(50*time.Millisecond, 0, scanFunc) // No rate limiting

	// Add a resource
	d.Add("default/pods/test1", K8sResourceIdentifier{Type: "pod", Namespace: "default", Name: "test1"})

	// Wait for debounce to flush
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.NotNil(t, received)
	assert.Len(t, received, 1)
	assert.Equal(t, "pod", received[0].Type)
	assert.Equal(t, "default", received[0].Namespace)
	assert.Equal(t, "test1", received[0].Name)
	mu.Unlock()
}

func TestDebouncer_Batching(t *testing.T) {
	var received []K8sResourceIdentifier
	var callCount int
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, resources []K8sResourceIdentifier) error {
		mu.Lock()
		received = resources
		callCount++
		mu.Unlock()
		return nil
	}

	d := NewDebouncer(100*time.Millisecond, 0, scanFunc) // No rate limiting

	// Add multiple resources of different types quickly (should batch)
	d.Add("default/pods/test1", K8sResourceIdentifier{Type: "pod", Namespace: "default", Name: "test1"})
	d.Add("default/deployments/test2", K8sResourceIdentifier{Type: "deployment", Namespace: "default", Name: "test2"})
	d.Add("default/pods/test3", K8sResourceIdentifier{Type: "pod", Namespace: "default", Name: "test3"})

	// Wait for debounce to flush
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount, "Should have batched into a single scan")
	assert.Len(t, received, 3)
	mu.Unlock()
}

func TestDebouncer_Replacement(t *testing.T) {
	var received []K8sResourceIdentifier
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, resources []K8sResourceIdentifier) error {
		mu.Lock()
		received = resources
		mu.Unlock()
		return nil
	}

	d := NewDebouncer(50*time.Millisecond, 0, scanFunc) // No rate limiting

	// Add a resource then update it (same key, should replace)
	d.Add("default/pods/test1", K8sResourceIdentifier{Type: "pod", Namespace: "default", Name: "test1"})
	d.Add("default/pods/test1", K8sResourceIdentifier{Type: "pod", Namespace: "default", Name: "test1"})

	// Wait for debounce to flush
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	// Should only have one resource since they have the same key
	assert.Len(t, received, 1)
	mu.Unlock()
}

func TestDebouncer_QueueSize(t *testing.T) {
	scanFunc := func(ctx context.Context, resources []K8sResourceIdentifier) error {
		return nil
	}

	d := NewDebouncer(1*time.Hour, 0, scanFunc) // Long debounce so it doesn't flush, no rate limiting

	assert.Equal(t, 0, d.QueueSize())

	d.Add("key1", K8sResourceIdentifier{Type: "pod", Namespace: "default", Name: "test1"})
	assert.Equal(t, 1, d.QueueSize())

	d.Add("key2", K8sResourceIdentifier{Type: "deployment", Namespace: "default", Name: "test2"})
	assert.Equal(t, 2, d.QueueSize())

	// Same key should replace, not add
	d.Add("key1", K8sResourceIdentifier{Type: "pod", Namespace: "default", Name: "test1"})
	assert.Equal(t, 2, d.QueueSize())
}

func TestDebouncer_RateLimiting(t *testing.T) {
	var callCount int
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, resources []K8sResourceIdentifier) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	}

	// 50ms debounce, 200ms rate limit
	d := NewDebouncer(50*time.Millisecond, 200*time.Millisecond, scanFunc)

	// First resource - should trigger scan after debounce
	d.Add("key1", K8sResourceIdentifier{Type: "pod", Namespace: "default", Name: "test1"})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount, "First scan should have fired")
	mu.Unlock()

	// Second resource immediately - should be rate limited
	d.Add("key2", K8sResourceIdentifier{Type: "deployment", Namespace: "default", Name: "test2"})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount, "Second scan should be rate limited")
	mu.Unlock()

	// Wait for rate limit to expire
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 2, callCount, "Second scan should have fired after rate limit")
	mu.Unlock()
}

func TestDebouncer_RateLimitingBatches(t *testing.T) {
	var callCount int
	var lastResources []K8sResourceIdentifier
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, resources []K8sResourceIdentifier) error {
		mu.Lock()
		callCount++
		lastResources = resources
		mu.Unlock()
		return nil
	}

	// 50ms debounce, 300ms rate limit
	d := NewDebouncer(50*time.Millisecond, 300*time.Millisecond, scanFunc)

	// First scan
	d.Add("key1", K8sResourceIdentifier{Type: "pod", Namespace: "default", Name: "test1"})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount)
	mu.Unlock()

	// Add multiple resources while rate limited - should batch
	d.Add("key2", K8sResourceIdentifier{Type: "deployment", Namespace: "default", Name: "test2"})
	time.Sleep(20 * time.Millisecond)
	d.Add("key3", K8sResourceIdentifier{Type: "statefulset", Namespace: "default", Name: "test3"})
	time.Sleep(20 * time.Millisecond)
	d.Add("key4", K8sResourceIdentifier{Type: "daemonset", Namespace: "default", Name: "test4"})

	// Still rate limited
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	assert.Equal(t, 1, callCount, "Should still be rate limited")
	mu.Unlock()

	// Wait for rate limit to expire and scan to fire
	time.Sleep(250 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 2, callCount, "Should have batched all resources into one scan")
	assert.Len(t, lastResources, 3)
	// Sort by name for consistent testing
	sort.Slice(lastResources, func(i, j int) bool {
		return lastResources[i].Name < lastResources[j].Name
	})
	assert.Equal(t, "test2", lastResources[0].Name)
	assert.Equal(t, "test3", lastResources[1].Name)
	assert.Equal(t, "test4", lastResources[2].Name)
	mu.Unlock()
}

func TestDebouncer_MultipleNamespaces(t *testing.T) {
	var received []K8sResourceIdentifier
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, resources []K8sResourceIdentifier) error {
		mu.Lock()
		received = resources
		mu.Unlock()
		return nil
	}

	d := NewDebouncer(50*time.Millisecond, 0, scanFunc)

	// Add resources from different namespaces
	d.Add("default/pods/test1", K8sResourceIdentifier{Type: "pod", Namespace: "default", Name: "test1"})
	d.Add("kube-system/pods/test2", K8sResourceIdentifier{Type: "pod", Namespace: "kube-system", Name: "test2"})
	d.Add("production/deployment/nginx", K8sResourceIdentifier{Type: "deployment", Namespace: "production", Name: "nginx"})

	// Wait for debounce to flush
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Len(t, received, 3)
	// Verify all resources are present with correct namespaces
	namespaces := make(map[string]bool)
	for _, r := range received {
		namespaces[r.Namespace] = true
	}
	assert.True(t, namespaces["default"])
	assert.True(t, namespaces["kube-system"])
	assert.True(t, namespaces["production"])
	mu.Unlock()
}

func TestK8sResourceIdentifier_String(t *testing.T) {
	// Namespaced resource
	r := K8sResourceIdentifier{Type: "deployment", Namespace: "default", Name: "nginx"}
	assert.Equal(t, "deployment:default:nginx", r.String())

	// Cluster-scoped resource
	r = K8sResourceIdentifier{Type: "namespace", Namespace: "", Name: "kube-system"}
	assert.Equal(t, "namespace:kube-system", r.String())
}
