// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

// CacheTransform strips a watched object down to its metadata before the
// informer stores it.
//
// The resource watcher cache exists to deliver change events. The event
// handler reads the namespace, the name and the labels of an object, then
// hands a type, namespace and name triple to the scanner. cnspec reads the
// object itself from the API server during the scan. The specs and the
// statuses in the cache are therefore never read, but they dominate the
// retained heap, in particular for ReplicaSets and Deployments which carry a
// full pod template each.
//
// Labels stay. They are small next to a spec, and label based filtering of the
// watched resources is in review in #1518.
func CacheTransform(obj any) (any, error) {
	switch o := obj.(type) {
	case *appsv1.Deployment:
		o.ManagedFields, o.Annotations = nil, nil
		o.Spec, o.Status = appsv1.DeploymentSpec{}, appsv1.DeploymentStatus{}
	case *appsv1.DaemonSet:
		o.ManagedFields, o.Annotations = nil, nil
		o.Spec, o.Status = appsv1.DaemonSetSpec{}, appsv1.DaemonSetStatus{}
	case *appsv1.StatefulSet:
		o.ManagedFields, o.Annotations = nil, nil
		o.Spec, o.Status = appsv1.StatefulSetSpec{}, appsv1.StatefulSetStatus{}
	case *appsv1.ReplicaSet:
		o.ManagedFields, o.Annotations = nil, nil
		o.Spec, o.Status = appsv1.ReplicaSetSpec{}, appsv1.ReplicaSetStatus{}
	case *corev1.Pod:
		o.ManagedFields, o.Annotations = nil, nil
		o.Spec, o.Status = corev1.PodSpec{}, corev1.PodStatus{}
	case *batchv1.Job:
		o.ManagedFields, o.Annotations = nil, nil
		o.Spec, o.Status = batchv1.JobSpec{}, batchv1.JobStatus{}
	case *batchv1.CronJob:
		o.ManagedFields, o.Annotations = nil, nil
		o.Spec, o.Status = batchv1.CronJobSpec{}, batchv1.CronJobStatus{}
	case *corev1.Service:
		o.ManagedFields, o.Annotations = nil, nil
		o.Spec, o.Status = corev1.ServiceSpec{}, corev1.ServiceStatus{}
	case *networkingv1.Ingress:
		o.ManagedFields, o.Annotations = nil, nil
		o.Spec, o.Status = networkingv1.IngressSpec{}, networkingv1.IngressStatus{}
	case *corev1.Namespace:
		o.ManagedFields, o.Annotations = nil, nil
		o.Spec, o.Status = corev1.NamespaceSpec{}, corev1.NamespaceStatus{}
	case *corev1.ConfigMap:
		o.ManagedFields, o.Annotations = nil, nil
		o.Data, o.BinaryData = nil, nil
	case *corev1.Secret:
		o.ManagedFields, o.Annotations = nil, nil
		o.Data, o.StringData = nil, nil
	case *corev1.ServiceAccount:
		o.ManagedFields, o.Annotations = nil, nil
		o.Secrets, o.ImagePullSecrets = nil, nil
	}
	return obj, nil
}
