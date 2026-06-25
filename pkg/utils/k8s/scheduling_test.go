// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestAddPodSchedulingToSpec(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Tolerations: []corev1.Toleration{
			{
				Key:      "existing",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
		},
	}

	nodeSelector := map[string]string{"nodepool": "scanners"}
	tolerations := []corev1.Toleration{
		{
			Key:      "sriov",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}

	AddPodSchedulingToSpec(podSpec, nodeSelector, tolerations)

	assert.Equal(t, nodeSelector, podSpec.NodeSelector)
	assert.ElementsMatch(t, []corev1.Toleration{
		{
			Key:      "existing",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "sriov",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}, podSpec.Tolerations)

	nodeSelector["nodepool"] = "mutated"
	assert.Equal(t, "scanners", podSpec.NodeSelector["nodepool"])
}

func TestMergeTolerations_DeduplicatesByValue(t *testing.T) {
	seconds := int64(60)
	existing := corev1.Toleration{
		Key:               "sriov",
		Operator:          corev1.TolerationOpEqual,
		Value:             "true",
		Effect:            corev1.TaintEffectNoExecute,
		TolerationSeconds: &seconds,
	}

	merged := MergeTolerations(
		[]corev1.Toleration{existing},
		[]corev1.Toleration{existing},
	)

	assert.Equal(t, []corev1.Toleration{existing}, merged)
}
