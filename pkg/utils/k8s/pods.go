// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// GetNewestPodFromList returns the most recent pod from a pod list
// This is determined by the creation timestamp of the pod
func GetNewestPodFromList(pods []corev1.Pod) corev1.Pod {
	podCreationTime := time.Unix(0, 0)
	currentPod := corev1.Pod{}
	for _, pod := range pods {
		if pod.ObjectMeta.CreationTimestamp.Time.Before(podCreationTime) {
			continue
		}
		podCreationTime = pod.ObjectMeta.CreationTimestamp.Time
		currentPod = pod
	}
	return currentPod
}
