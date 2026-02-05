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
	pending      map[string]string // resource key -> resource type
	scanFunc     func(ctx context.Context, resourceTypes []string) error
	timer        *time.Timer
	ctx          context.Context
	cancel       context.CancelFunc
	lastScanTime time.Time // time of the last completed scan
}

// NewDebouncer creates a new Debouncer with the given intervals and scan function.
// interval is the debounce interval (time to wait after last change before scanning).
// minInterval is the minimum time between scans (rate limit). Set to 0 to disable rate limiting.
func NewDebouncer(interval, minInterval time.Duration, scanFunc func(ctx context.Context, resourceTypes []string) error) *Debouncer {
	return &Debouncer{
		interval:    interval,
		minInterval: minInterval,
		pending:     make(map[string]string),
		scanFunc:    scanFunc,
	}
}

// Add adds a resource to the pending queue. The key should be unique per resource
// (e.g., "namespace/kind/name" or "kind/name" for cluster-scoped resources).
// The resourceType is extracted from the key for scanning.
func (d *Debouncer) Add(key string, resourceType string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.pending[key] = resourceType
	debouncerLogger.V(1).Info("Added resource to debounce queue", "key", key, "resourceType", resourceType, "queueSize", len(d.pending))

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

// flush processes all pending resources by collecting unique resource types
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

	// Collect unique resource types
	resourceTypeSet := make(map[string]struct{})
	keys := make([]string, 0, len(d.pending))
	for key, resourceType := range d.pending {
		resourceTypeSet[resourceType] = struct{}{}
		keys = append(keys, key)
	}

	// Convert set to slice
	resourceTypes := make([]string, 0, len(resourceTypeSet))
	for rt := range resourceTypeSet {
		resourceTypes = append(resourceTypes, rt)
	}

	// Clear pending and update last scan time
	d.pending = make(map[string]string)
	d.lastScanTime = time.Now()
	d.mu.Unlock()

	debouncerLogger.Info("Flushing debounce queue", "resourceCount", len(keys), "resourceTypes", resourceTypes)

	// Execute scan
	ctx := d.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	if err := d.scanFunc(ctx, resourceTypes); err != nil {
		debouncerLogger.Error(err, "Failed to scan resources", "keys", keys, "resourceTypes", resourceTypes)
	} else {
		debouncerLogger.Info("Successfully scanned resources", "keys", keys, "resourceTypes", resourceTypes)
	}
}

// QueueSize returns the current number of pending resources.
func (d *Debouncer) QueueSize() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pending)
}
