// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"maps"
	"slices"

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

// MergeTolerations returns base and extra tolerations without duplicating identical tolerations.
func MergeTolerations(base, extra []corev1.Toleration) []corev1.Toleration {
	if len(base) == 0 {
		return slices.Clone(extra)
	}
	if len(extra) == 0 {
		return slices.Clone(base)
	}

	tolerations := slices.Clone(base)
	seen := make(map[tolerationKey]struct{}, len(base)+len(extra))
	for _, toleration := range base {
		seen[newTolerationKey(toleration)] = struct{}{}
	}

	for _, toleration := range extra {
		key := newTolerationKey(toleration)
		if _, ok := seen[key]; ok {
			continue
		}
		tolerations = append(tolerations, toleration)
		seen[key] = struct{}{}
	}

	return tolerations
}

type tolerationKey struct {
	Key                  string
	Operator             corev1.TolerationOperator
	Value                string
	Effect               corev1.TaintEffect
	HasTolerationSeconds bool
	TolerationSeconds    int64
}

func newTolerationKey(toleration corev1.Toleration) tolerationKey {
	key := tolerationKey{
		Key:      toleration.Key,
		Operator: toleration.Operator,
		Value:    toleration.Value,
		Effect:   toleration.Effect,
	}
	if toleration.TolerationSeconds != nil {
		key.HasTolerationSeconds = true
		key.TolerationSeconds = *toleration.TolerationSeconds
	}
	return key
}
