// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package operator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTransformPod_KeepsFieldsTheOperatorReads(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:          "mondoo-client-node-scan",
			Namespace:     "mondoo-operator",
			Labels:        map[string]string{"mondoo_cr": "mondoo-client"},
			Annotations:   map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubelet"}},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{
					Name:  "cnspec",
					Image: "docker.io/mondoo/cnspec:latest",
					Env:   []corev1.EnvVar{{Name: "DEBUG", Value: "true"}},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("300Mi")},
					},
				},
			},
			Volumes:     []corev1.Volume{{Name: "root"}},
			Tolerations: []corev1.Toleration{{Key: "node-role.kubernetes.io/control-plane"}},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodFailed,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:                 "cnspec",
					RestartCount:         3,
					State:                corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}},
					Image:                "docker.io/mondoo/cnspec:latest",
					ImageID:              "sha256:unused",
					ContainerID:          "containerd://unused",
					LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 137}},
				},
			},
			PodIP: "10.0.0.1",
		},
	}

	out, err := transformPod(pod)
	require.NoError(t, err)

	got, ok := out.(*corev1.Pod)
	require.True(t, ok)

	// Kept.
	assert.Equal(t, "mondoo-client-node-scan", got.Name)
	assert.Equal(t, "mondoo-operator", got.Namespace)
	assert.Equal(t, map[string]string{"mondoo_cr": "mondoo-client"}, got.Labels)
	assert.Equal(t, "node-1", got.Spec.NodeName)
	require.Len(t, got.Spec.Containers, 1)
	assert.Equal(t, "cnspec", got.Spec.Containers[0].Name)
	assert.Equal(t, "docker.io/mondoo/cnspec:latest", got.Spec.Containers[0].Image)
	assert.Equal(t, "300Mi", got.Spec.Containers[0].Resources.Limits.Memory().String())
	assert.Equal(t, corev1.PodFailed, got.Status.Phase)
	assert.Len(t, got.Status.Conditions, 1)
	require.Len(t, got.Status.ContainerStatuses, 1)
	assert.Equal(t, int32(137), got.Status.ContainerStatuses[0].LastTerminationState.Terminated.ExitCode)
	assert.Equal(t, int32(3), got.Status.ContainerStatuses[0].RestartCount)
	assert.Equal(t, "OOMKilled", got.Status.ContainerStatuses[0].State.Terminated.Reason)

	// Dropped.
	assert.Nil(t, got.ManagedFields)
	assert.Nil(t, got.Annotations)
	assert.Nil(t, got.Spec.Containers[0].Env)
	assert.Nil(t, got.Spec.Volumes)
	assert.Nil(t, got.Spec.Tolerations)
	assert.Empty(t, got.Status.PodIP)
	assert.Empty(t, got.Status.ContainerStatuses[0].Image)
	assert.Empty(t, got.Status.ContainerStatuses[0].ImageID)
	assert.Empty(t, got.Status.ContainerStatuses[0].ContainerID)
}

func TestTransformPod_KeepsContainerIndexes(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "sidecar"}, {Name: "cnspec"}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{Name: "sidecar"}, {Name: "cnspec"}},
		},
	}

	out, err := transformPod(pod)
	require.NoError(t, err)

	got := out.(*corev1.Pod)
	require.Len(t, got.Spec.Containers, 2)
	for i, status := range got.Status.ContainerStatuses {
		assert.Equal(t, status.Name, got.Spec.Containers[i].Name)
	}
}

func TestTransformPod_IgnoresOtherTypes(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}

	out, err := transformPod(node)
	require.NoError(t, err)
	assert.Same(t, node, out)
}

func TestTransformNode(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:          "node-1",
			Labels:        map[string]string{"kubernetes.io/hostname": "node-1"},
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubelet"}},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{{Key: "dedicated", Value: "mondoo", Effect: corev1.TaintEffectNoSchedule}},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
			Capacity:    corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("8Gi")},
			Images:      []corev1.ContainerImage{{Names: []string{"docker.io/library/nginx:latest"}, SizeBytes: 1000}},
		},
	}

	out, err := transformNode(node)
	require.NoError(t, err)

	got, ok := out.(*corev1.Node)
	require.True(t, ok)

	assert.Equal(t, "node-1", got.Name)
	assert.Equal(t, map[string]string{"kubernetes.io/hostname": "node-1"}, got.Labels)
	assert.Equal(t, []corev1.Taint{{Key: "dedicated", Value: "mondoo", Effect: corev1.TaintEffectNoSchedule}}, got.Spec.Taints)
	assert.Equal(t, "4Gi", got.Status.Allocatable.Memory().String())
	assert.Equal(t, "8Gi", got.Status.Capacity.Memory().String())

	assert.Nil(t, got.ManagedFields)
	assert.Nil(t, got.Status.Images)
}

func TestTransformNode_IgnoresOtherTypes(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}}

	out, err := transformNode(pod)
	require.NoError(t, err)
	assert.Same(t, pod, out)
}

func TestCacheOptions(t *testing.T) {
	opts := cacheOptions()
	require.Len(t, opts.ByObject, 2)

	for obj, byObject := range opts.ByObject {
		assert.NotNil(t, byObject.Transform, "missing transform for %T", obj)
	}
}
