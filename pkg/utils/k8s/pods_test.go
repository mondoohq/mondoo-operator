// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestContainerMemoryLimit(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "sidecar",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("64Mi")},
					},
				},
				{
					Name: "cnspec",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("300Mi")},
					},
				},
				{
					Name: "no-limits",
				},
			},
		},
	}

	assert.Equal(t, "300Mi", ContainerMemoryLimit(pod, "cnspec"))
	assert.Equal(t, "64Mi", ContainerMemoryLimit(pod, "sidecar"))
	assert.Equal(t, "0", ContainerMemoryLimit(pod, "no-limits"))
	assert.Equal(t, "", ContainerMemoryLimit(pod, "missing"))
	assert.Equal(t, "", ContainerMemoryLimit(&corev1.Pod{}, "cnspec"))
}
