// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

func TestConditions_Disabled(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{}
	config.Spec.KubernetesResources.Enable = false

	updateResourceWatcherConditions(config, false, &corev1.PodList{})

	cond := mondoo.FindMondooAuditConditions(config.Status.Conditions, v1alpha2.ResourceWatcherDegraded)
	assert.NotNil(t, cond)
	assert.Equal(t, v1alpha2.ResourceWatcherDegraded, cond.Type)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, "ResourceWatcherDisabled", cond.Reason)
}

func TestConditions_K8sEnabledButWatcherDisabled(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{}
	config.Spec.KubernetesResources.Enable = true
	config.Spec.KubernetesResources.ResourceWatcher.Enable = false

	updateResourceWatcherConditions(config, false, &corev1.PodList{})

	cond := mondoo.FindMondooAuditConditions(config.Status.Conditions, v1alpha2.ResourceWatcherDegraded)
	assert.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, "ResourceWatcherDisabled", cond.Reason)
}

func TestConditions_Available(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{}
	config.Spec.KubernetesResources.Enable = true
	config.Spec.KubernetesResources.ResourceWatcher.Enable = true

	updateResourceWatcherConditions(config, false, &corev1.PodList{})

	cond := mondoo.FindMondooAuditConditions(config.Status.Conditions, v1alpha2.ResourceWatcherDegraded)
	assert.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, "ResourceWatcherAvailable", cond.Reason)
	assert.Equal(t, "Resource Watcher is available", cond.Message)
}

func TestConditions_Degraded(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{}
	config.Spec.KubernetesResources.Enable = true
	config.Spec.KubernetesResources.ResourceWatcher.Enable = true

	updateResourceWatcherConditions(config, true, &corev1.PodList{})

	cond := mondoo.FindMondooAuditConditions(config.Status.Conditions, v1alpha2.ResourceWatcherDegraded)
	assert.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, "ResourceWatcherUnavailable", cond.Reason)
	assert.Equal(t, "Resource Watcher is unavailable", cond.Message)
}

func TestConditions_OOM(t *testing.T) {
	config := &v1alpha2.MondooAuditConfig{}
	config.Spec.KubernetesResources.Enable = true
	config.Spec.KubernetesResources.ResourceWatcher.Enable = true

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pod",
					CreationTimestamp: metav1.Now(),
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "mondoo-resource-watcher",
							LastTerminationState: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 137, // OOM kill
								},
							},
						},
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "mondoo-resource-watcher",
						},
					},
				},
			},
		},
	}

	updateResourceWatcherConditions(config, true, pods)

	cond := mondoo.FindMondooAuditConditions(config.Status.Conditions, v1alpha2.ResourceWatcherDegraded)
	assert.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, oomMessage, cond.Message)
	assert.Contains(t, cond.AffectedPods, "test-pod")
}
