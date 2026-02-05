// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var watcherLogger = ctrl.Log.WithName("resource-watcher")

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
}

// ResourceWatcher watches Kubernetes resources and triggers scans when they change.
type ResourceWatcher struct {
	cache     cache.Cache
	debouncer *Debouncer
	config    WatcherConfig
	scheme    *runtime.Scheme
}

// NewResourceWatcher creates a new ResourceWatcher.
func NewResourceWatcher(c cache.Cache, debouncer *Debouncer, config WatcherConfig, scheme *runtime.Scheme) *ResourceWatcher {
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
		cache:     c,
		debouncer: debouncer,
		config:    config,
		scheme:    scheme,
	}
}

// Start begins watching resources and processing events.
func (w *ResourceWatcher) Start(ctx context.Context) error {
	watcherLogger.Info("Starting resource watcher",
		"namespaces", w.config.Namespaces,
		"namespacesExclude", w.config.NamespacesExclude,
		"resourceTypes", w.config.ResourceTypes)

	// Set up informers for each resource type
	for _, resourceType := range w.config.ResourceTypes {
		obj, gvk, err := w.getObjectForResourceType(resourceType)
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
			gvk:          gvk,
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

// getObjectForResourceType returns the client.Object and GroupVersionKind for a resource type string.
func (w *ResourceWatcher) getObjectForResourceType(resourceType string) (client.Object, schema.GroupVersionKind, error) {
	switch strings.ToLower(resourceType) {
	case "pods", "pod":
		return &corev1.Pod{}, corev1.SchemeGroupVersion.WithKind("Pod"), nil
	case "deployments", "deployment":
		return &appsv1.Deployment{}, appsv1.SchemeGroupVersion.WithKind("Deployment"), nil
	case "daemonsets", "daemonset":
		return &appsv1.DaemonSet{}, appsv1.SchemeGroupVersion.WithKind("DaemonSet"), nil
	case "statefulsets", "statefulset":
		return &appsv1.StatefulSet{}, appsv1.SchemeGroupVersion.WithKind("StatefulSet"), nil
	case "replicasets", "replicaset":
		return &appsv1.ReplicaSet{}, appsv1.SchemeGroupVersion.WithKind("ReplicaSet"), nil
	case "jobs", "job":
		return &batchv1.Job{}, batchv1.SchemeGroupVersion.WithKind("Job"), nil
	case "cronjobs", "cronjob":
		return &batchv1.CronJob{}, batchv1.SchemeGroupVersion.WithKind("CronJob"), nil
	case "services", "service":
		return &corev1.Service{}, corev1.SchemeGroupVersion.WithKind("Service"), nil
	case "ingresses", "ingress":
		return &networkingv1.Ingress{}, networkingv1.SchemeGroupVersion.WithKind("Ingress"), nil
	case "namespaces", "namespace":
		return &corev1.Namespace{}, corev1.SchemeGroupVersion.WithKind("Namespace"), nil
	case "configmaps", "configmap":
		return &corev1.ConfigMap{}, corev1.SchemeGroupVersion.WithKind("ConfigMap"), nil
	case "secrets", "secret":
		return &corev1.Secret{}, corev1.SchemeGroupVersion.WithKind("Secret"), nil
	case "serviceaccounts", "serviceaccount":
		return &corev1.ServiceAccount{}, corev1.SchemeGroupVersion.WithKind("ServiceAccount"), nil
	default:
		return nil, schema.GroupVersionKind{}, fmt.Errorf("unknown resource type: %s", resourceType)
	}
}

// shouldWatchNamespace returns true if the namespace should be watched.
func (w *ResourceWatcher) shouldWatchNamespace(namespace string) bool {
	// If include list is specified, only watch those namespaces
	if len(w.config.Namespaces) > 0 {
		for _, ns := range w.config.Namespaces {
			if ns == namespace {
				return true
			}
		}
		return false
	}

	// Check exclude list
	for _, ns := range w.config.NamespacesExclude {
		if ns == namespace {
			return false
		}
	}

	return true
}

// resourceEventHandler handles resource events from informers.
type resourceEventHandler struct {
	watcher      *ResourceWatcher
	resourceType string
	gvk          schema.GroupVersionKind
}

func (h *resourceEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	// Skip initial list sync - we only want to scan changes
	if isInInitialList {
		return
	}
	h.handleEvent(obj, "add")
}

func (h *resourceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	h.handleEvent(newObj, "update")
}

func (h *resourceEventHandler) OnDelete(obj interface{}) {
	// We don't scan on delete - the resource is gone
	watcherLogger.V(1).Info("Resource deleted", "resourceType", h.resourceType)
}

func (h *resourceEventHandler) handleEvent(obj interface{}, eventType string) {
	clientObj, ok := obj.(client.Object)
	if !ok {
		watcherLogger.Error(nil, "Failed to convert object to client.Object")
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
	// Note: resourceType is plural (e.g., "deployments"), but cnspec's k8s-resources filter
	// expects singular form (e.g., "deployment"), so we trim the trailing 's'
	singularType := h.resourceType
	if len(singularType) > 0 && singularType[len(singularType)-1] == 's' {
		singularType = singularType[:len(singularType)-1]
	}

	resource := K8sResourceIdentifier{
		Type:      singularType,
		Namespace: namespace,
		Name:      clientObj.GetName(),
	}

	// Add to debouncer
	h.watcher.debouncer.Add(key, resource)
}
