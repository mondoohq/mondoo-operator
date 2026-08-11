// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"cmp"
	"slices"

	corev1 "k8s.io/api/core/v1"
)

func TaintsToTolerations(taints []corev1.Taint) []corev1.Toleration {
	var tolerations []corev1.Toleration
	for _, t := range taints {
		tolerations = append(tolerations, TaintToToleration(t))
	}
	return tolerations
}

// SortTolerations sorts tolerations in place into a deterministic order, since a slice built from a map has none.
func SortTolerations(tolerations []corev1.Toleration) {
	slices.SortFunc(tolerations, func(a, b corev1.Toleration) int {
		return cmp.Or(
			cmp.Compare(a.Key, b.Key),
			cmp.Compare(a.Operator, b.Operator),
			cmp.Compare(a.Value, b.Value),
			cmp.Compare(a.Effect, b.Effect),
			compareTolerationSeconds(a.TolerationSeconds, b.TolerationSeconds),
		)
	})
}

// compareTolerationSeconds treats unset (nil) as less than any set value.
func compareTolerationSeconds(a, b *int64) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	default:
		return cmp.Compare(*a, *b)
	}
}

func TaintToToleration(t corev1.Taint) corev1.Toleration {
	return corev1.Toleration{
		Key:    t.Key,
		Effect: t.Effect,
		Value:  t.Value,
	}
}
