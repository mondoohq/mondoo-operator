// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
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
