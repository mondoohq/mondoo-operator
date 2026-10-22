// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// ContainerMemoryLimit returns the memory limit of the named container as a
// string. It returns an empty string if the Pod has no such container.
//
// The kubelet sorts status.containerStatuses by name, while spec.containers
// keeps the order of the manifest. The two lists must therefore be matched by
// name, not by index.
func ContainerMemoryLimit(pod *corev1.Pod, containerName string) string {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == containerName {
			return pod.Spec.Containers[i].Resources.Limits.Memory().String()
		}
	}
	return ""
}

// GetNewestPodFromList returns the most recent pod from a pod list
// This is determined by the creation timestamp of the pod
func GetNewestPodFromList(pods []corev1.Pod) corev1.Pod {
	podCreationTime := time.Unix(0, 0)
	currentPod := corev1.Pod{}
	for _, pod := range pods {
		if pod.CreationTimestamp.Time.Before(podCreationTime) {
			continue
		}
		podCreationTime = pod.CreationTimestamp.Time
		currentPod = pod
	}
	return currentPod
}
