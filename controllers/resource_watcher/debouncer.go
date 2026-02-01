// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"context"
	"sync"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

var debouncerLogger = ctrl.Log.WithName("resource-watcher-debouncer")

// Debouncer batches resource changes over a configurable interval before triggering scans.
// This prevents excessive scanning when multiple resources change in quick succession.
// It also enforces a minimum interval between scans to provide rate limiting.
type Debouncer struct {
	interval     time.Duration
	minInterval  time.Duration // minimum time between scans (rate limit)
	mu           sync.Mutex
	pending      map[string][]byte // resource key -> YAML manifest
	scanFunc     func(ctx context.Context, manifests []byte) error
	timer        *time.Timer
	ctx          context.Context
	cancel       context.CancelFunc
	lastScanTime time.Time // time of the last completed scan
}

// NewDebouncer creates a new Debouncer with the given intervals and scan function.
// interval is the debounce interval (time to wait after last change before scanning).
// minInterval is the minimum time between scans (rate limit). Set to 0 to disable rate limiting.
func NewDebouncer(interval, minInterval time.Duration, scanFunc func(ctx context.Context, manifests []byte) error) *Debouncer {
	return &Debouncer{
		interval:    interval,
		minInterval: minInterval,
		pending:     make(map[string][]byte),
		scanFunc:    scanFunc,
	}
}

// Add adds a resource to the pending queue. The key should be unique per resource
// (e.g., "namespace/kind/name"). If a resource with the same key is already pending,
// it will be replaced with the new manifest.
func (d *Debouncer) Add(key string, manifest []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.pending[key] = manifest
	debouncerLogger.V(1).Info("Added resource to debounce queue", "key", key, "queueSize", len(d.pending))

	// Reset the timer if it exists, or start a new one
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.interval, d.flush)
}

// Start begins the debouncer's background processing. It should be called once
// and will run until the context is cancelled.
func (d *Debouncer) Start(ctx context.Context) error {
	d.ctx, d.cancel = context.WithCancel(ctx)
	debouncerLogger.Info("Debouncer started", "interval", d.interval, "minInterval", d.minInterval)
	<-d.ctx.Done()
	d.stop()
	return nil
}

// Stop stops the debouncer and flushes any pending resources.
func (d *Debouncer) stop() {
	d.mu.Lock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	d.mu.Unlock()

	// Final flush on shutdown
	d.flush()
	debouncerLogger.Info("Debouncer stopped")
}

// flush processes all pending resources by combining them into a single manifest
// and calling the scan function. It enforces the minimum interval between scans.
func (d *Debouncer) flush() {
	d.mu.Lock()
	if len(d.pending) == 0 {
		d.mu.Unlock()
		return
	}

	// Check rate limiting - if minInterval is set and not enough time has passed, reschedule
	if d.minInterval > 0 && !d.lastScanTime.IsZero() {
		elapsed := time.Since(d.lastScanTime)
		if elapsed < d.minInterval {
			waitTime := d.minInterval - elapsed
			debouncerLogger.Info("Rate limiting: rescheduling flush", "waitTime", waitTime, "pendingCount", len(d.pending))
			d.timer = time.AfterFunc(waitTime, d.flush)
			d.mu.Unlock()
			return
		}
	}

	// Collect all pending manifests
	manifests := make([][]byte, 0, len(d.pending))
	keys := make([]string, 0, len(d.pending))
	for key, manifest := range d.pending {
		manifests = append(manifests, manifest)
		keys = append(keys, key)
	}

	// Clear pending and update last scan time
	d.pending = make(map[string][]byte)
	d.lastScanTime = time.Now()
	d.mu.Unlock()

	debouncerLogger.Info("Flushing debounce queue", "resourceCount", len(manifests))

	// Combine all manifests with YAML document separators
	combined := combineManifests(manifests)

	// Execute scan
	ctx := d.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	if err := d.scanFunc(ctx, combined); err != nil {
		debouncerLogger.Error(err, "Failed to scan resources", "keys", keys)
	} else {
		debouncerLogger.Info("Successfully scanned resources", "keys", keys)
	}
}

// QueueSize returns the current number of pending resources.
func (d *Debouncer) QueueSize() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pending)
}

// combineManifests combines multiple YAML manifests into a single multi-document YAML.
func combineManifests(manifests [][]byte) []byte {
	if len(manifests) == 0 {
		return nil
	}

	// Calculate total size
	totalSize := 0
	separator := []byte("---\n")
	for _, m := range manifests {
		totalSize += len(m) + len(separator)
	}

	// Combine
	result := make([]byte, 0, totalSize)
	for i, m := range manifests {
		if i > 0 {
			result = append(result, separator...)
		}
		result = append(result, m...)
		// Ensure each manifest ends with a newline
		if len(m) > 0 && m[len(m)-1] != '\n' {
			result = append(result, '\n')
		}
	}

	return result
}
