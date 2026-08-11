// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func TestTaintsToTolerations(t *testing.T) {
	taints := []corev1.Taint{
		{
			Key:    "key",
			Value:  "value",
			Effect: corev1.TaintEffectNoExecute,
		},
		{
			Key:    "key2",
			Value:  "value2",
			Effect: corev1.TaintEffectNoSchedule,
		},
	}

	tolerations := TaintsToTolerations(taints)

	for i, taint := range taints {
		assert.Equal(t, taint.Key, tolerations[i].Key)
		assert.Equal(t, taint.Value, tolerations[i].Value)
		assert.Equal(t, taint.Effect, tolerations[i].Effect)
	}
}

func TestTaintToToleration(t *testing.T) {
	taint := corev1.Taint{
		Key:    "key",
		Value:  "value",
		Effect: corev1.TaintEffectNoExecute,
	}

	toleration := TaintToToleration(taint)

	assert.Equal(t, taint.Key, toleration.Key)
	assert.Equal(t, taint.Value, toleration.Value)
	assert.Equal(t, taint.Effect, toleration.Effect)
}

func TestSortTolerations_Deterministic(t *testing.T) {
	a := corev1.Toleration{Key: "a", Operator: corev1.TolerationOpEqual, Value: "1", Effect: corev1.TaintEffectNoSchedule}
	b := corev1.Toleration{Key: "b", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
	c := corev1.Toleration{Key: "b", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute}
	d := corev1.Toleration{Key: "b", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute, TolerationSeconds: ptr.To(int64(30))}

	want := []corev1.Toleration{a, c, d, b}

	// Every permutation must converge on the same sorted result.
	for _, input := range [][]corev1.Toleration{
		{a, b, c, d},
		{d, c, b, a},
		{b, a, d, c},
		{c, d, a, b},
	} {
		got := append([]corev1.Toleration{}, input...)
		SortTolerations(got)
		assert.Equal(t, want, got)
	}
}
