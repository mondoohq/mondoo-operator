// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDebouncer_Add(t *testing.T) {
	var received []byte
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, manifests []byte) error {
		mu.Lock()
		received = manifests
		mu.Unlock()
		return nil
	}

	d := NewDebouncer(50*time.Millisecond, 0, scanFunc) // No rate limiting

	// Add a resource
	d.Add("default/pod/test1", []byte("apiVersion: v1\nkind: Pod\nname: test1"))

	// Wait for debounce to flush
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.NotNil(t, received)
	assert.Contains(t, string(received), "test1")
	mu.Unlock()
}

func TestDebouncer_Batching(t *testing.T) {
	var received []byte
	var callCount int
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, manifests []byte) error {
		mu.Lock()
		received = manifests
		callCount++
		mu.Unlock()
		return nil
	}

	d := NewDebouncer(100*time.Millisecond, 0, scanFunc) // No rate limiting

	// Add multiple resources quickly (should batch)
	d.Add("default/pod/test1", []byte("apiVersion: v1\nkind: Pod\nname: test1"))
	d.Add("default/pod/test2", []byte("apiVersion: v1\nkind: Pod\nname: test2"))
	d.Add("default/pod/test3", []byte("apiVersion: v1\nkind: Pod\nname: test3"))

	// Wait for debounce to flush
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount, "Should have batched into a single scan")
	assert.Contains(t, string(received), "test1")
	assert.Contains(t, string(received), "test2")
	assert.Contains(t, string(received), "test3")
	mu.Unlock()
}

func TestDebouncer_Replacement(t *testing.T) {
	var received []byte
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, manifests []byte) error {
		mu.Lock()
		received = manifests
		mu.Unlock()
		return nil
	}

	d := NewDebouncer(50*time.Millisecond, 0, scanFunc) // No rate limiting

	// Add a resource then update it
	d.Add("default/pod/test1", []byte("apiVersion: v1\nkind: Pod\nname: test1-old"))
	d.Add("default/pod/test1", []byte("apiVersion: v1\nkind: Pod\nname: test1-new"))

	// Wait for debounce to flush
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Contains(t, string(received), "test1-new")
	assert.NotContains(t, string(received), "test1-old")
	mu.Unlock()
}

func TestDebouncer_QueueSize(t *testing.T) {
	scanFunc := func(ctx context.Context, manifests []byte) error {
		return nil
	}

	d := NewDebouncer(1*time.Hour, 0, scanFunc) // Long debounce so it doesn't flush, no rate limiting

	assert.Equal(t, 0, d.QueueSize())

	d.Add("key1", []byte("manifest1"))
	assert.Equal(t, 1, d.QueueSize())

	d.Add("key2", []byte("manifest2"))
	assert.Equal(t, 2, d.QueueSize())

	// Same key should replace, not add
	d.Add("key1", []byte("manifest1-updated"))
	assert.Equal(t, 2, d.QueueSize())
}

func TestDebouncer_RateLimiting(t *testing.T) {
	var callCount int
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, manifests []byte) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	}

	// 50ms debounce, 200ms rate limit
	d := NewDebouncer(50*time.Millisecond, 200*time.Millisecond, scanFunc)

	// First resource - should trigger scan after debounce
	d.Add("key1", []byte("manifest1"))
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount, "First scan should have fired")
	mu.Unlock()

	// Second resource immediately - should be rate limited
	d.Add("key2", []byte("manifest2"))
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
	var lastManifests []byte
	var mu sync.Mutex

	scanFunc := func(ctx context.Context, manifests []byte) error {
		mu.Lock()
		callCount++
		lastManifests = manifests
		mu.Unlock()
		return nil
	}

	// 50ms debounce, 300ms rate limit
	d := NewDebouncer(50*time.Millisecond, 300*time.Millisecond, scanFunc)

	// First scan
	d.Add("key1", []byte("manifest1"))
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount)
	mu.Unlock()

	// Add multiple resources while rate limited - should batch
	d.Add("key2", []byte("manifest2"))
	time.Sleep(20 * time.Millisecond)
	d.Add("key3", []byte("manifest3"))
	time.Sleep(20 * time.Millisecond)
	d.Add("key4", []byte("manifest4"))

	// Still rate limited
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	assert.Equal(t, 1, callCount, "Should still be rate limited")
	mu.Unlock()

	// Wait for rate limit to expire and scan to fire
	time.Sleep(250 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 2, callCount, "Should have batched all resources into one scan")
	assert.Contains(t, string(lastManifests), "manifest2")
	assert.Contains(t, string(lastManifests), "manifest3")
	assert.Contains(t, string(lastManifests), "manifest4")
	mu.Unlock()
}

func TestCombineManifests(t *testing.T) {
	manifests := [][]byte{
		[]byte("manifest1"),
		[]byte("manifest2"),
		[]byte("manifest3"),
	}

	combined := combineManifests(manifests)
	result := string(combined)

	assert.Contains(t, result, "manifest1")
	assert.Contains(t, result, "manifest2")
	assert.Contains(t, result, "manifest3")
	assert.Contains(t, result, "---") // YAML document separator
}

func TestCombineManifests_Empty(t *testing.T) {
	combined := combineManifests(nil)
	assert.Nil(t, combined)

	combined = combineManifests([][]byte{})
	assert.Nil(t, combined)
}
