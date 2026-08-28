// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func testMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:          "test-resource",
		Namespace:     "test-namespace",
		Labels:        map[string]string{"app": "test"},
		Annotations:   map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
		ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kube-controller-manager"}},
	}
}

func TestCacheTransform_KeepsIdentity(t *testing.T) {
	podTemplate := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "nginx:latest"}}},
	}

	objects := []client.Object{
		&appsv1.Deployment{ObjectMeta: testMeta(), Spec: appsv1.DeploymentSpec{Replicas: ptr.To(int32(3)), Template: podTemplate}},
		&appsv1.DaemonSet{ObjectMeta: testMeta(), Spec: appsv1.DaemonSetSpec{Template: podTemplate}},
		&appsv1.StatefulSet{ObjectMeta: testMeta(), Spec: appsv1.StatefulSetSpec{Template: podTemplate}},
		&appsv1.ReplicaSet{ObjectMeta: testMeta(), Spec: appsv1.ReplicaSetSpec{Template: podTemplate}},
		&corev1.Pod{ObjectMeta: testMeta(), Spec: podTemplate.Spec},
		&batchv1.Job{ObjectMeta: testMeta(), Spec: batchv1.JobSpec{Template: podTemplate}},
		&batchv1.CronJob{ObjectMeta: testMeta(), Spec: batchv1.CronJobSpec{Schedule: "* * * * *"}},
		&corev1.Service{ObjectMeta: testMeta(), Spec: corev1.ServiceSpec{ClusterIP: "10.0.0.1"}},
		&networkingv1.Ingress{ObjectMeta: testMeta(), Spec: networkingv1.IngressSpec{DefaultBackend: &networkingv1.IngressBackend{
			Service: &networkingv1.IngressServiceBackend{Name: "test", Port: networkingv1.ServiceBackendPort{Number: 80}},
		}}},
		&corev1.Namespace{ObjectMeta: testMeta(), Spec: corev1.NamespaceSpec{Finalizers: []corev1.FinalizerName{"kubernetes"}}},
		&corev1.ConfigMap{ObjectMeta: testMeta(), Data: map[string]string{"key": "value"}},
		&corev1.Secret{ObjectMeta: testMeta(), Data: map[string][]byte{"key": []byte("value")}},
		&corev1.ServiceAccount{ObjectMeta: testMeta(), Secrets: []corev1.ObjectReference{{Name: "token"}}},
	}

	for _, obj := range objects {
		out, err := CacheTransform(obj)
		require.NoError(t, err)

		got, ok := out.(client.Object)
		require.True(t, ok)

		assert.Equal(t, "test-resource", got.GetName())
		assert.Equal(t, "test-namespace", got.GetNamespace())
		assert.Equal(t, map[string]string{"app": "test"}, got.GetLabels())
		assert.Nil(t, got.GetAnnotations())
		assert.Nil(t, got.GetManagedFields())
	}
}

func TestCacheTransform_DropsPayload(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: testMeta(),
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(3))},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 3},
	}
	out, err := CacheTransform(deployment)
	require.NoError(t, err)
	assert.Equal(t, appsv1.DeploymentSpec{}, out.(*appsv1.Deployment).Spec)
	assert.Equal(t, appsv1.DeploymentStatus{}, out.(*appsv1.Deployment).Status)

	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: testMeta(),
		Spec: appsv1.ReplicaSetSpec{Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "nginx:latest"}}},
		}},
	}
	out, err = CacheTransform(replicaSet)
	require.NoError(t, err)
	assert.Empty(t, out.(*appsv1.ReplicaSet).Spec.Template.Spec.Containers)

	configMap := &corev1.ConfigMap{ObjectMeta: testMeta(), Data: map[string]string{"key": "value"}, BinaryData: map[string][]byte{"bin": {1}}}
	out, err = CacheTransform(configMap)
	require.NoError(t, err)
	assert.Nil(t, out.(*corev1.ConfigMap).Data)
	assert.Nil(t, out.(*corev1.ConfigMap).BinaryData)

	secret := &corev1.Secret{ObjectMeta: testMeta(), Data: map[string][]byte{"key": []byte("value")}, StringData: map[string]string{"key": "value"}}
	out, err = CacheTransform(secret)
	require.NoError(t, err)
	assert.Nil(t, out.(*corev1.Secret).Data)
	assert.Nil(t, out.(*corev1.Secret).StringData)

	ingress := &networkingv1.Ingress{ObjectMeta: testMeta(), Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "example.com"}}}}
	out, err = CacheTransform(ingress)
	require.NoError(t, err)
	assert.Empty(t, out.(*networkingv1.Ingress).Spec.Rules)
}

func TestCacheTransform_PassesUnknownTypesThrough(t *testing.T) {
	obj := &corev1.Endpoints{
		ObjectMeta: testMeta(),
		Subsets:    []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}}},
	}
	want := obj.DeepCopy()

	out, err := CacheTransform(obj)
	require.NoError(t, err)
	assert.Same(t, obj, out)
	assert.Equal(t, want, out, "unknown types must pass through unmodified")
}

func TestCacheTransform_HandlesEveryWatchedResourceType(t *testing.T) {
	watcher := NewResourceWatcher(nil, nil, WatcherConfig{})
	types := append([]string{}, DefaultResourceTypes...)
	types = append(types, "configmaps", "secrets", "serviceaccounts")

	for _, resourceType := range types {
		obj, err := watcher.getObjectForResourceType(resourceType)
		require.NoError(t, err)

		obj.SetAnnotations(map[string]string{"test": "value"})
		obj.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "test"}})

		out, err := CacheTransform(obj)
		require.NoError(t, err)

		got := out.(client.Object)
		assert.Nil(t, got.GetAnnotations(), "annotations kept for %s", resourceType)
		assert.Nil(t, got.GetManagedFields(), "managed fields kept for %s", resourceType)
	}
}
