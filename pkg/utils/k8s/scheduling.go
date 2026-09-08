// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"maps"

	corev1 "k8s.io/api/core/v1"
)

// AddPodSchedulingToSpec applies node scheduling settings to a PodSpec.
func AddPodSchedulingToSpec(podSpec *corev1.PodSpec, nodeSelector map[string]string, tolerations []corev1.Toleration) {
	if len(nodeSelector) > 0 {
		podSpec.NodeSelector = maps.Clone(nodeSelector)
	}
	if len(tolerations) > 0 {
		podSpec.Tolerations = MergeTolerations(podSpec.Tolerations, tolerations)
	}
}
