// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package admission

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConditions_Disabled(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{}
	updateAdmissionConditions(config, true, &corev1.PodList{})

	cond := config.Status.Conditions[0]
	assert.Equal(t, "Admission controller is disabled", cond.Message)
	assert.Equal(t, "AdmissionDisabled", cond.Reason)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, v1alpha2.AdmissionDegraded, cond.Type)
}

func TestConditions_Available(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			Admission: v1alpha2.Admission{Enable: true},
		},
	}
	updateAdmissionConditions(config, false, &corev1.PodList{})

	cond := config.Status.Conditions[0]
	assert.Equal(t, "Admission controller is available", cond.Message)
	assert.Equal(t, "AdmissionAvailable", cond.Reason)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, v1alpha2.AdmissionDegraded, cond.Type)
}

func TestConditions_Degraded(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			Admission: v1alpha2.Admission{Enable: true},
		},
	}
	updateAdmissionConditions(config, true, &corev1.PodList{})

	cond := config.Status.Conditions[0]
	assert.Equal(t, "Admission controller is unavailable", cond.Message)
	assert.Equal(t, "AdmissionUnvailable", cond.Reason)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, v1alpha2.AdmissionDegraded, cond.Type)
}

func TestConditions_OOM(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			Admission: v1alpha2.Admission{Enable: true},
		},
	}

	podList := oomPodList()
	pod := podList.Items[0]
	updateAdmissionConditions(config, true, podList)

	cond := config.Status.Conditions[0]
	assert.Equal(t, oomMessage, cond.Message)
	assert.Equal(t, "AdmissionUnvailable", cond.Reason)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, v1alpha2.AdmissionDegraded, cond.Type)
	assert.Equal(t, pod.Spec.Containers[0].Resources.Limits.Memory().String(), cond.MemoryLimit)
	assert.Equal(t, []string{pod.Name}, cond.AffectedPods)
}

func TestConditions_OOM_Unavailable(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			Admission: v1alpha2.Admission{Enable: true},
		},
	}

	podList := oomPodList()
	pod := podList.Items[0]
	updateAdmissionConditions(config, true, podList)

	cond := config.Status.Conditions[0]
	assert.Equal(t, oomMessage, cond.Message)
	assert.Equal(t, "AdmissionUnvailable", cond.Reason)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, v1alpha2.AdmissionDegraded, cond.Type)
	assert.Equal(t, pod.Spec.Containers[0].Resources.Limits.Memory().String(), cond.MemoryLimit)
	assert.Equal(t, []string{pod.Name}, cond.AffectedPods)

	updateAdmissionConditions(config, true, &corev1.PodList{})

	// Verify nothing changed
	cond = config.Status.Conditions[0]
	assert.Equal(t, oomMessage, cond.Message)
	assert.Equal(t, "AdmissionUnvailable", cond.Reason)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, v1alpha2.AdmissionDegraded, cond.Type)
	assert.Equal(t, pod.Spec.Containers[0].Resources.Limits.Memory().String(), cond.MemoryLimit)
	assert.Equal(t, []string{pod.Name}, cond.AffectedPods)
}

func TestConditions_OOM_Available(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{
		Spec: v1alpha2.MondooAuditConfigSpec{
			Admission: v1alpha2.Admission{Enable: true},
		},
	}
	updateAdmissionConditions(config, true, oomPodList())

	cond := config.Status.Conditions[0]
	assert.Equal(t, oomMessage, cond.Message)
	assert.Equal(t, "AdmissionUnvailable", cond.Reason)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, v1alpha2.AdmissionDegraded, cond.Type)

	updateAdmissionConditions(config, false, &corev1.PodList{})

	cond = config.Status.Conditions[0]
	assert.Equal(t, "Admission controller is available", cond.Message)
	assert.Equal(t, "AdmissionAvailable", cond.Reason)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, v1alpha2.AdmissionDegraded, cond.Type)
	assert.Empty(t, cond.AffectedPods)
}

func oomPodList() *corev1.PodList {
	return &corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.Time{Time: time.Now()}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "webhook",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "webhook",
							LastTerminationState: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 137,
								},
							},
						},
					},
				},
			},
		},
	}
}
