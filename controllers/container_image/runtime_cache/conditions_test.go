// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package runtime_cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateRuntimeCacheConditionsOOMUsesContainerName(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			Containers: v1alpha2.Containers{
				RuntimeCache: v1alpha2.RuntimeCacheScanner{Enable: true},
			},
		},
	}
	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "runtime-cache-pod",
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sidecar",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("32Mi"),
								},
							},
						},
						{
							Name: "mondoo-runtime-cache-scan",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "mondoo-runtime-cache-scan",
							LastTerminationState: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{ExitCode: 137},
							},
						},
					},
				},
			},
		},
	}

	updateRuntimeCacheConditions(config, true, pods, "", "")

	cond := config.Status.Conditions[0]
	assert.Equal(t, oomMessage, cond.Message)
	assert.Equal(t, "RuntimeCacheScanningUnavailable", cond.Reason)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, v1alpha2.RuntimeCacheScanningDegraded, cond.Type)
	assert.Equal(t, "256Mi", cond.MemoryLimit)
	assert.Equal(t, []string{"runtime-cache-pod"}, cond.AffectedPods)
}

func TestUpdateRuntimeCacheConditionsDetectsOOMAcrossDaemonSetPods(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			Containers: v1alpha2.Containers{
				RuntimeCache: v1alpha2.RuntimeCacheScanner{Enable: true},
			},
		},
	}
	now := time.Now()
	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "runtime-cache-worker-a",
					CreationTimestamp: metav1.Time{Time: now.Add(-5 * time.Minute)},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "mondoo-runtime-cache-scan",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "mondoo-runtime-cache-scan",
							LastTerminationState: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{ExitCode: 137},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "runtime-cache-worker-b",
					CreationTimestamp: metav1.Time{Time: now},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "mondoo-runtime-cache-scan",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:  "mondoo-runtime-cache-scan",
							Ready: true,
						},
					},
				},
			},
		},
	}

	updateRuntimeCacheConditions(config, true, pods, "", "")

	cond := config.Status.Conditions[0]
	assert.Equal(t, oomMessage, cond.Message)
	assert.Equal(t, "RuntimeCacheScanningUnavailable", cond.Reason)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, "128Mi", cond.MemoryLimit)
	assert.Equal(t, []string{"runtime-cache-worker-a"}, cond.AffectedPods)
}
