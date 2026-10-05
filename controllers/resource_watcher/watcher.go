// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var watcherLogger = ctrl.Log.WithName("resource-watcher")

const (
	defaultNamespaceLabelCacheTTL        = 30 * time.Second
	defaultNamespaceLabelCacheMaxEntries = 1024
)

// HighPriorityResourceTypes are stable resources that represent actual workloads.
// These are preferred over ephemeral resources like Pods and Jobs which change frequently
// but are covered by their parent resources.
var HighPriorityResourceTypes = []string{
	"deployments",
	"daemonsets",
	"statefulsets",
	"replicasets",
}

// DefaultResourceTypes is the full list of resource types to watch when WatchAllResources is true.
var DefaultResourceTypes = []string{
	"pods",
	"deployments",
	"daemonsets",
	"statefulsets",
	"replicasets",
	"jobs",
	"cronjobs",
	"services",
	"ingresses",
	"namespaces",
}

// WatcherConfig holds configuration for the ResourceWatcher.
type WatcherConfig struct {
	// Namespaces is the list of namespaces to watch. Empty means all namespaces.
	Namespaces []string
	// NamespacesExclude is the list of namespaces to exclude from watching.
	NamespacesExclude []string
	// ResourceTypes is the list of resource types to watch (e.g., "pods", "deployments").
	// If empty, defaults are used based on WatchAllResources.
	ResourceTypes []string
	// WatchAllResources determines which default resource types to watch.
	// When false (default), only HighPriorityResourceTypes are watched.
	// When true, all DefaultResourceTypes are watched (including ephemeral resources like Pods).
	WatchAllResources bool
	// NamespaceSelector selects namespaces whose resources should be watched.
	NamespaceSelector labels.Selector
	// ObjectSelector selects watched objects by their own labels.
	ObjectSelector labels.Selector
}

// ResourceWatcher watches Kubernetes resources and triggers scans when they change.
type ResourceWatcher struct {
	cache                         ctrlcache.Cache
	namespaceReader               client.Reader
	namespaceLabelCache           map[string]namespaceLabelCacheEntry
	namespaceLabelCacheMu         sync.RWMutex
	namespaceLabelCacheTTL        time.Duration
	namespaceLabelCacheMaxEntries int
	debouncer                     *Debouncer
	config                        WatcherConfig
}

// NewResourceWatcher creates a new ResourceWatcher.
func NewResourceWatcher(c ctrlcache.Cache, debouncer *Debouncer, config WatcherConfig) *ResourceWatcher {
	if len(config.ResourceTypes) == 0 {
		// Use high-priority resources by default (stable workload resources).
		// Only use all resources if explicitly requested via WatchAllResources.
		if config.WatchAllResources {
			config.ResourceTypes = DefaultResourceTypes
		} else {
			config.ResourceTypes = HighPriorityResourceTypes
		}
	}
	return &ResourceWatcher{
		cache:                         c,
		namespaceReader:               c,
		namespaceLabelCache:           map[string]namespaceLabelCacheEntry{},
		namespaceLabelCacheTTL:        defaultNamespaceLabelCacheTTL,
		namespaceLabelCacheMaxEntries: defaultNamespaceLabelCacheMaxEntries,
		debouncer:                     debouncer,
		config:                        config,
	}
}

// Start begins watching resources and processing events.
func (w *ResourceWatcher) Start(ctx context.Context) error {
	watcherLogger.Info("Starting resource watcher",
		"namespaces", w.config.Namespaces,
		"namespacesExclude", w.config.NamespacesExclude,
		"namespaceSelector", selectorString(w.config.NamespaceSelector),
		"objectSelector", selectorString(w.config.ObjectSelector),
		"resourceTypes", w.config.ResourceTypes)

	if selectorConfigured(w.config.NamespaceSelector) {
		if err := w.registerNamespaceLabelInformer(ctx); err != nil {
			watcherLogger.Error(err, "Failed to add namespace label informer")
		}
	}

	// Set up informers for each resource type
	for _, resourceType := range w.config.ResourceTypes {
		obj, err := w.getObjectForResourceType(resourceType)
		if err != nil {
			watcherLogger.Error(err, "Failed to get object for resource type", "resourceType", resourceType)
			continue
		}

		informer, err := w.cache.GetInformer(ctx, obj)
		if err != nil {
			watcherLogger.Error(err, "Failed to get informer for resource type", "resourceType", resourceType)
			continue
		}

		handler := &resourceEventHandler{
			watcher:      w,
			resourceType: resourceType,
			ctx:          ctx,
		}

		_, err = informer.AddEventHandler(handler)
		if err != nil {
			watcherLogger.Error(err, "Failed to add event handler", "resourceType", resourceType)
			continue
		}

		watcherLogger.Info("Started watching resource type", "resourceType", resourceType)
	}

	// Wait for context cancellation
	<-ctx.Done()
	watcherLogger.Info("Resource watcher stopped")
	return nil
}

func (w *ResourceWatcher) registerNamespaceLabelInformer(ctx context.Context) error {
	informer, err := w.cache.GetInformer(ctx, &corev1.Namespace{})
	if err != nil {
		return err
	}

	_, err = informer.AddEventHandler(&namespaceLabelEventHandler{watcher: w})
	return err
}

// getObjectForResourceType returns the client.Object for a resource type string.
func (w *ResourceWatcher) getObjectForResourceType(resourceType string) (client.Object, error) {
	switch strings.ToLower(resourceType) {
	case "pods", "pod":
		return &corev1.Pod{}, nil
	case "deployments", "deployment":
		return &appsv1.Deployment{}, nil
	case "daemonsets", "daemonset":
		return &appsv1.DaemonSet{}, nil
	case "statefulsets", "statefulset":
		return &appsv1.StatefulSet{}, nil
	case "replicasets", "replicaset":
		return &appsv1.ReplicaSet{}, nil
	case "jobs", "job":
		return &batchv1.Job{}, nil
	case "cronjobs", "cronjob":
		return &batchv1.CronJob{}, nil
	case "services", "service":
		return &corev1.Service{}, nil
	case "ingresses", "ingress":
		return &networkingv1.Ingress{}, nil
	case "namespaces", "namespace":
		return &corev1.Namespace{}, nil
	case "configmaps", "configmap":
		return &corev1.ConfigMap{}, nil
	case "secrets", "secret":
		return &corev1.Secret{}, nil
	case "serviceaccounts", "serviceaccount":
		return &corev1.ServiceAccount{}, nil
	default:
		return nil, fmt.Errorf("unknown resource type: %s", resourceType)
	}
}

// shouldWatchNamespace returns true if the namespace should be watched.
func (w *ResourceWatcher) shouldWatchNamespace(namespace string) bool {
	// If include list is specified, only watch those namespaces
	if len(w.config.Namespaces) > 0 {
		return slices.Contains(w.config.Namespaces, namespace)
	}

	// Check exclude list
	return !slices.Contains(w.config.NamespacesExclude, namespace)
}

func (w *ResourceWatcher) shouldWatchNamespaceLabels(ctx context.Context, namespace string) bool {
	if !selectorConfigured(w.config.NamespaceSelector) {
		return true
	}
	if labelSet, found, ok := w.cachedNamespaceLabels(namespace); ok {
		return found && selectorMatches(w.config.NamespaceSelector, labelSet)
	}
	if w.namespaceReader == nil {
		watcherLogger.Error(nil, "Cannot evaluate namespace selector without a namespace reader", "namespace", namespace)
		return false
	}

	ns := &corev1.Namespace{}
	if err := w.namespaceReader.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		if !apierrors.IsNotFound(err) {
			watcherLogger.Error(err, "Failed to read namespace labels", "namespace", namespace)
		}
		w.setCachedNamespaceLabels(namespace, nil, false)
		return false
	}

	labelSet := labels.Set(ns.GetLabels())
	w.setCachedNamespaceLabels(namespace, labelSet, true)
	return selectorMatches(w.config.NamespaceSelector, labelSet)
}

func (w *ResourceWatcher) shouldWatchObjectLabels(obj client.Object) bool {
	if _, ok := obj.(*corev1.Namespace); ok {
		return true
	}
	return selectorMatches(w.config.ObjectSelector, labels.Set(obj.GetLabels()))
}

func (w *ResourceWatcher) shouldWatchNamespaceResource(obj client.Object) bool {
	if _, ok := obj.(*corev1.Namespace); !ok {
		return true
	}
	return selectorMatches(w.config.NamespaceSelector, labels.Set(obj.GetLabels()))
}

func selectorMatches(selector labels.Selector, labelSet labels.Set) bool {
	return selector == nil || selector.Empty() || selector.Matches(labelSet)
}

func selectorConfigured(selector labels.Selector) bool {
	return selector != nil && !selector.Empty()
}

func selectorString(selector labels.Selector) string {
	if !selectorConfigured(selector) {
		return ""
	}
	return selector.String()
}

// resourceEventHandler handles resource events from informers.
type resourceEventHandler struct {
	watcher      *ResourceWatcher
	resourceType string
	// client-go ResourceEventHandler callbacks don't receive a context, but
	// namespace selector cache misses need one for namespace reads.
	ctx context.Context
}

func (h *resourceEventHandler) OnAdd(obj any, isInInitialList bool) {
	// Skip initial list sync - we only want to scan changes
	if isInInitialList {
		return
	}
	h.handleEvent(obj, "add")
}

func (h *resourceEventHandler) OnUpdate(oldObj, newObj any) {
	h.handleEvent(newObj, "update")
}

func (h *resourceEventHandler) OnDelete(obj any) {
	// We don't scan on delete - the resource is gone
	watcherLogger.V(1).Info("Resource deleted", "resourceType", h.resourceType)
}

func (h *resourceEventHandler) handleEvent(obj any, eventType string) {
	clientObj, ok := obj.(client.Object)
	if !ok {
		watcherLogger.Error(nil, "Failed to convert object to client.Object")
		return
	}

	if !h.watcher.shouldWatchObjectLabels(clientObj) {
		watcherLogger.V(2).Info("Skipping resource because object labels do not match selector",
			"resourceType", h.resourceType,
			"namespace", clientObj.GetNamespace(),
			"name", clientObj.GetName())
		return
	}

	// Namespace resources are cluster-scoped, so filter them by their own labels.
	if !h.watcher.shouldWatchNamespaceResource(clientObj) {
		watcherLogger.V(2).Info("Skipping namespace because labels do not match selector",
			"resourceType", h.resourceType,
			"name", clientObj.GetName())
		return
	}

	namespace := clientObj.GetNamespace()

	// Check namespace filtering (skip for cluster-scoped resources)
	if namespace != "" && !h.watcher.shouldWatchNamespace(namespace) {
		watcherLogger.V(2).Info("Skipping resource in excluded namespace",
			"resourceType", h.resourceType,
			"namespace", namespace,
			"name", clientObj.GetName())
		return
	}
	ctx := h.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if namespace != "" && !h.watcher.shouldWatchNamespaceLabels(ctx, namespace) {
		watcherLogger.V(2).Info("Skipping resource because namespace labels do not match selector",
			"resourceType", h.resourceType,
			"namespace", namespace,
			"name", clientObj.GetName())
		return
	}

	// Create unique key for the resource
	key := fmt.Sprintf("%s/%s/%s", namespace, h.resourceType, clientObj.GetName())
	if namespace == "" {
		key = fmt.Sprintf("%s/%s", h.resourceType, clientObj.GetName())
	}

	watcherLogger.V(1).Info("Resource changed",
		"event", eventType,
		"resourceType", h.resourceType,
		"namespace", namespace,
		"name", clientObj.GetName())

	// Create resource identifier for scanning
	resource := K8sResourceIdentifier{
		Type:      h.resourceType, // plural form (e.g., "deployments")
		Namespace: namespace,
		Name:      clientObj.GetName(),
	}

	// Add to debouncer
	h.watcher.debouncer.Add(key, resource)
}

type namespaceLabelCacheEntry struct {
	labels    labels.Set
	found     bool
	expiresAt time.Time
}

func (w *ResourceWatcher) cachedNamespaceLabels(namespace string) (labels.Set, bool, bool) {
	w.namespaceLabelCacheMu.RLock()
	entry, ok := w.namespaceLabelCache[namespace]
	w.namespaceLabelCacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false, false
	}
	return entry.labels, entry.found, true
}

func (w *ResourceWatcher) setCachedNamespaceLabels(namespace string, labelSet labels.Set, found bool) {
	ttl := w.namespaceLabelCacheTTL
	if ttl <= 0 {
		ttl = defaultNamespaceLabelCacheTTL
	}
	maxEntries := w.namespaceLabelCacheMaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultNamespaceLabelCacheMaxEntries
	}
	now := time.Now()

	w.namespaceLabelCacheMu.Lock()
	defer w.namespaceLabelCacheMu.Unlock()
	if w.namespaceLabelCache == nil {
		w.namespaceLabelCache = map[string]namespaceLabelCacheEntry{}
	}
	w.pruneExpiredNamespaceLabelCacheLocked(now)
	if _, exists := w.namespaceLabelCache[namespace]; !exists {
		for len(w.namespaceLabelCache) >= maxEntries {
			w.evictOldestNamespaceLabelCacheEntryLocked()
		}
	}
	w.namespaceLabelCache[namespace] = namespaceLabelCacheEntry{
		labels:    labelSet,
		found:     found,
		expiresAt: now.Add(ttl),
	}
}

func (w *ResourceWatcher) pruneExpiredNamespaceLabelCacheLocked(now time.Time) {
	for namespace, entry := range w.namespaceLabelCache {
		if now.After(entry.expiresAt) {
			delete(w.namespaceLabelCache, namespace)
		}
	}
}

func (w *ResourceWatcher) evictOldestNamespaceLabelCacheEntryLocked() {
	var oldestNamespace string
	var oldestExpiry time.Time
	for namespace, entry := range w.namespaceLabelCache {
		if oldestNamespace == "" || entry.expiresAt.Before(oldestExpiry) {
			oldestNamespace = namespace
			oldestExpiry = entry.expiresAt
		}
	}
	if oldestNamespace != "" {
		delete(w.namespaceLabelCache, oldestNamespace)
	}
}

func (w *ResourceWatcher) deleteCachedNamespaceLabels(namespace string) {
	w.namespaceLabelCacheMu.Lock()
	defer w.namespaceLabelCacheMu.Unlock()
	delete(w.namespaceLabelCache, namespace)
}

type namespaceLabelEventHandler struct {
	watcher *ResourceWatcher
}

func (h *namespaceLabelEventHandler) OnAdd(obj any, isInInitialList bool) {
	h.update(obj)
}

func (h *namespaceLabelEventHandler) OnUpdate(oldObj, newObj any) {
	h.update(newObj)
}

func (h *namespaceLabelEventHandler) OnDelete(obj any) {
	ns, ok := namespaceFromEvent(obj)
	if !ok {
		return
	}
	h.watcher.deleteCachedNamespaceLabels(ns.Name)
}

func (h *namespaceLabelEventHandler) update(obj any) {
	ns, ok := namespaceFromEvent(obj)
	if !ok {
		return
	}
	h.watcher.setCachedNamespaceLabels(ns.Name, labels.Set(ns.GetLabels()), true)
}

func namespaceFromEvent(obj any) (*corev1.Namespace, bool) {
	if ns, ok := obj.(*corev1.Namespace); ok {
		return ns, true
	}
	tombstone, ok := obj.(toolscache.DeletedFinalStateUnknown)
	if !ok {
		return nil, false
	}
	ns, ok := tombstone.Obj.(*corev1.Namespace)
	return ns, ok
}
