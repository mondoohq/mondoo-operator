// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package operator

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// cacheOptions returns the manager cache configuration. The operator watches
// Pods and Nodes cluster wide, so the informer cache retains one copy of every
// Pod and every Node in the cluster. The transforms below drop the fields that
// no controller reads, which cuts the retained heap of both caches.
func cacheOptions() cache.Options {
	return cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&corev1.Pod{}:  {Transform: transformPod},
			&corev1.Node{}: {Transform: transformNode},
		},
	}
}

// transformPod strips a Pod down to the fields the operator reads.
//
// The operator reads:
//   - object metadata, for names, labels, namespaces and creation timestamps,
//   - spec.nodeName, to group node scan Pods per node,
//   - spec.containers name, image and resources, to read the operator image and
//     to report the memory limit of an OOM killed scan container,
//   - status phase, conditions and container statuses, to detect OOM kills.
//
// Everything else (managed fields, annotations, env vars, volumes, tolerations,
// affinity, security context) is dropped. Container order and count stay intact
// because the condition code indexes spec.containers by container status index.
func transformPod(obj any) (any, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return obj, nil
	}

	pod.ManagedFields = nil
	pod.Annotations = nil

	containers := make([]corev1.Container, len(pod.Spec.Containers))
	for i, c := range pod.Spec.Containers {
		containers[i] = corev1.Container{Name: c.Name, Image: c.Image, Resources: c.Resources}
	}
	pod.Spec = corev1.PodSpec{NodeName: pod.Spec.NodeName, Containers: containers}

	statuses := make([]corev1.ContainerStatus, len(pod.Status.ContainerStatuses))
	for i, status := range pod.Status.ContainerStatuses {
		statuses[i] = corev1.ContainerStatus{
			Name:                 status.Name,
			State:                status.State,
			LastTerminationState: status.LastTerminationState,
			RestartCount:         status.RestartCount,
		}
	}

	pod.Status = corev1.PodStatus{
		Phase:             pod.Status.Phase,
		Conditions:        pod.Status.Conditions,
		ContainerStatuses: statuses,
	}

	return pod, nil
}

// transformNode drops the Node fields that no controller reads. The operator
// reads node names, labels, taints and the allocatable resources. The image
// list in the status is the largest part of a Node object on a busy node and
// nothing reads it.
func transformNode(obj any) (any, error) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return obj, nil
	}

	node.ManagedFields = nil
	node.Status.Images = nil

	return node, nil
}
