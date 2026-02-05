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
	var received []string
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, resourceTypes []string) error {
		mu.Lock()
		received = resourceTypes
		mu.Unlock()
		return nil
	}

	d := NewDebouncer(50*time.Millisecond, 0, scanFunc) // No rate limiting

	// Add a resource
	d.Add("default/pods/test1", "pods")

	// Wait for debounce to flush
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.NotNil(t, received)
	assert.Contains(t, received, "pods")
	mu.Unlock()
}

func TestDebouncer_Batching(t *testing.T) {
	var received []string
	var callCount int
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, resourceTypes []string) error {
		mu.Lock()
		received = resourceTypes
		callCount++
		mu.Unlock()
		return nil
	}

	d := NewDebouncer(100*time.Millisecond, 0, scanFunc) // No rate limiting

	// Add multiple resources of different types quickly (should batch)
	d.Add("default/pods/test1", "pods")
	d.Add("default/deployments/test2", "deployments")
	d.Add("default/pods/test3", "pods") // Same type as first

	// Wait for debounce to flush
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount, "Should have batched into a single scan")
	// Should have unique resource types
	sort.Strings(received)
	assert.Equal(t, []string{"deployments", "pods"}, received)
	mu.Unlock()
}

func TestDebouncer_Replacement(t *testing.T) {
	var received []string
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, resourceTypes []string) error {
		mu.Lock()
		received = resourceTypes
		mu.Unlock()
		return nil
	}

	d := NewDebouncer(50*time.Millisecond, 0, scanFunc) // No rate limiting

	// Add a resource then update it (same key, should replace)
	d.Add("default/pods/test1", "pods")
	d.Add("default/pods/test1", "pods") // Same key

	// Wait for debounce to flush
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	// Should only have one resource type since they're the same
	assert.Equal(t, []string{"pods"}, received)
	mu.Unlock()
}

func TestDebouncer_QueueSize(t *testing.T) {
	scanFunc := func(ctx context.Context, resourceTypes []string) error {
		return nil
	}

	d := NewDebouncer(1*time.Hour, 0, scanFunc) // Long debounce so it doesn't flush, no rate limiting

	assert.Equal(t, 0, d.QueueSize())

	d.Add("key1", "pods")
	assert.Equal(t, 1, d.QueueSize())

	d.Add("key2", "deployments")
	assert.Equal(t, 2, d.QueueSize())

	// Same key should replace, not add
	d.Add("key1", "pods")
	assert.Equal(t, 2, d.QueueSize())
}

func TestDebouncer_RateLimiting(t *testing.T) {
	var callCount int
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, resourceTypes []string) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	}

	// 50ms debounce, 200ms rate limit
	d := NewDebouncer(50*time.Millisecond, 200*time.Millisecond, scanFunc)

	// First resource - should trigger scan after debounce
	d.Add("key1", "pods")
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount, "First scan should have fired")
	mu.Unlock()

	// Second resource immediately - should be rate limited
	d.Add("key2", "deployments")
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
	var lastResourceTypes []string
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, resourceTypes []string) error {
		mu.Lock()
		callCount++
		lastResourceTypes = resourceTypes
		mu.Unlock()
		return nil
	}

	// 50ms debounce, 300ms rate limit
	d := NewDebouncer(50*time.Millisecond, 300*time.Millisecond, scanFunc)

	// First scan
	d.Add("key1", "pods")
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount)
	mu.Unlock()

	// Add multiple resources while rate limited - should batch
	d.Add("key2", "deployments")
	time.Sleep(20 * time.Millisecond)
	d.Add("key3", "statefulsets")
	time.Sleep(20 * time.Millisecond)
	d.Add("key4", "daemonsets")

	// Still rate limited
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	assert.Equal(t, 1, callCount, "Should still be rate limited")
	mu.Unlock()

	// Wait for rate limit to expire and scan to fire
	time.Sleep(250 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 2, callCount, "Should have batched all resources into one scan")
	sort.Strings(lastResourceTypes)
	assert.Equal(t, []string{"daemonsets", "deployments", "statefulsets"}, lastResourceTypes)
	mu.Unlock()
}

func TestDebouncer_UniqueResourceTypes(t *testing.T) {
	var received []string
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, resourceTypes []string) error {
		mu.Lock()
		received = resourceTypes
		mu.Unlock()
		return nil
	}

	d := NewDebouncer(50*time.Millisecond, 0, scanFunc)

	// Add multiple resources of the same type
	d.Add("default/pods/test1", "pods")
	d.Add("default/pods/test2", "pods")
	d.Add("default/pods/test3", "pods")
	d.Add("kube-system/pods/test4", "pods")

	// Wait for debounce to flush
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	// Should only have one unique resource type
	assert.Equal(t, 1, len(received))
	assert.Contains(t, received, "pods")
	mu.Unlock()
}
