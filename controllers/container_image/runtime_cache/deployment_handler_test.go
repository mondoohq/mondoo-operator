// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package runtime_cache

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	fakeMondoo "go.mondoo.com/mondoo-operator/pkg/utils/mondoo/fake"
	"go.mondoo.com/mondoo-operator/pkg/utils/test"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDeploymentHandlerReconcileCreate(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	m := runtimeCacheAuditConfig()
	node := &corev1.Node{}
	node.Name = "node-a"

	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(test.TestKubeSystemNamespace(), m, node).
		Build()
	handler := deploymentHandler(kubeClient, m)

	result, err := handler.Reconcile(ctx)
	require.NoError(t, err)
	require.True(t, result.IsZero())

	ds := &appsv1.DaemonSet{}
	require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: DaemonSetName(m.Name)}, ds))
	require.Equal(t, DefaultServiceAccount, ds.Spec.Template.Spec.ServiceAccountName)

	cm := &corev1.ConfigMap{}
	require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: ConfigMapName(m.Name)}, cm))
	require.Contains(t, cm.Data["delegates"], "containerd-cri")
	require.Contains(t, cm.Data["inventory"], "runtime-cache-images")
}

func TestDeploymentHandlerReconcileConfiguredLatestOmitsTimerForResolvedDigest(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	m := runtimeCacheAuditConfig()
	m.Spec.Scanner.Image.Name = "ghcr.io/mondoohq/mondoo-operator/cnspec"
	m.Spec.Scanner.Image.Tag = "latest"

	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(test.TestKubeSystemNamespace(), m).
		Build()
	handler := deploymentHandler(kubeClient, m)
	handler.ContainerImageResolver = &fakeMondoo.ContainerImageResolverMock{
		CnspecImageFunc: func(userImage, userTag, userDigest string, skipResolveImage bool) (string, error) {
			assert.Equal(t, "ghcr.io/mondoohq/mondoo-operator/cnspec", userImage)
			assert.Equal(t, "latest", userTag)
			assert.Empty(t, userDigest)
			assert.False(t, skipResolveImage)
			return "ghcr.io/mondoohq/mondoo-operator/cnspec@sha256:abc123", nil
		},
	}

	result, err := handler.Reconcile(ctx)
	require.NoError(t, err)
	require.True(t, result.IsZero())

	ds := &appsv1.DaemonSet{}
	require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: DaemonSetName(m.Name)}, ds))
	command := strings.Join(ds.Spec.Template.Spec.Containers[0].Command, " ")
	assert.Contains(t, command, "--inventory-file "+runtimeCacheInventoryRenderedPath)
	assert.NotContains(t, command, "--timer")
}

func TestDeploymentHandlerReconcileUsesResolvedRenderImage(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	m := runtimeCacheAuditConfig()
	m.Spec.Containers.RuntimeCache.ScannerSets = []v1alpha2.NodeScannerSet{{Name: "workers"}}

	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(test.TestKubeSystemNamespace(), m).
		Build()
	handler := deploymentHandler(kubeClient, m)
	handler.MondooOperatorConfig = &v1alpha2.MondooOperatorConfig{Spec: v1alpha2.MondooOperatorConfigSpec{
		SkipContainerResolution: true,
	}}
	handler.ContainerImageResolver = &fakeMondoo.ContainerImageResolverMock{
		CnspecImageFunc: func(userImage, userTag, userDigest string, skipResolveImage bool) (string, error) {
			return "registry.example.com/cnspec:13-rootless", nil
		},
		ContainerImageFunc: func(ctx context.Context, image string, skipResolveImage bool) (string, error) {
			assert.Equal(t, constants.BusyBoxImage, image)
			assert.True(t, skipResolveImage)
			return "registry.example.com/dockerhub/library/busybox:1.36", nil
		},
	}

	result, err := handler.Reconcile(ctx)
	require.NoError(t, err)
	require.True(t, result.IsZero())

	ds := &appsv1.DaemonSet{}
	require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: DaemonSetNameForScannerSet(m.Name, "workers")}, ds))
	require.Len(t, ds.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "registry.example.com/dockerhub/library/busybox:1.36", ds.Spec.Template.Spec.InitContainers[0].Image)
}

func TestDeploymentHandlerReconcileDoesNotMirrorNodeTaints(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	m := runtimeCacheAuditConfig()
	m.Spec.Containers.RuntimeCache.Tolerations = []corev1.Toleration{
		{Key: "runtime-cache", Operator: corev1.TolerationOpEqual, Value: "enabled", Effect: corev1.TaintEffectNoSchedule},
	}
	node := &corev1.Node{}
	node.Name = "tainted-node"
	node.Spec.Taints = []corev1.Taint{
		{Key: "node-role.kubernetes.io/control-plane", Effect: corev1.TaintEffectNoSchedule},
	}

	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(test.TestKubeSystemNamespace(), m, node).
		Build()
	handler := deploymentHandler(kubeClient, m)

	result, err := handler.Reconcile(ctx)
	require.NoError(t, err)
	require.True(t, result.IsZero())

	ds := &appsv1.DaemonSet{}
	require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: DaemonSetName(m.Name)}, ds))
	assert.Equal(t, []corev1.Toleration{
		{Key: "runtime-cache", Operator: corev1.TolerationOpEqual, Value: "enabled", Effect: corev1.TaintEffectNoSchedule},
	}, ds.Spec.Template.Spec.Tolerations)
}

func TestDeploymentHandlerReconcileScannerSets(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	m := runtimeCacheAuditConfig()
	m.Spec.Containers.RuntimeCache.ScannerSets = []v1alpha2.NodeScannerSet{
		{
			Name:         "control-plane",
			NodeSelector: map[string]string{"node-role.kubernetes.io/control-plane": ""},
			Tolerations: []corev1.Toleration{
				{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
			},
		},
		{
			Name:         "workers",
			NodeSelector: map[string]string{"node-role.kubernetes.io/worker": "true"},
		},
	}

	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(test.TestKubeSystemNamespace(), m).
		Build()
	handler := deploymentHandler(kubeClient, m)

	result, err := handler.Reconcile(ctx)
	require.NoError(t, err)
	require.True(t, result.IsZero())

	controlPlane := &appsv1.DaemonSet{}
	require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: DaemonSetNameForScannerSet(m.Name, "control-plane")}, controlPlane))
	assert.Equal(t, "control-plane", controlPlane.Labels["scanner_set"])
	assert.Equal(t, map[string]string{"node-role.kubernetes.io/control-plane": ""}, controlPlane.Spec.Template.Spec.NodeSelector)

	workers := &appsv1.DaemonSet{}
	require.NoError(t, kubeClient.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: DaemonSetNameForScannerSet(m.Name, "workers")}, workers))
	assert.Equal(t, "workers", workers.Labels["scanner_set"])
	assert.Equal(t, map[string]string{"node-role.kubernetes.io/worker": "true"}, workers.Spec.Template.Spec.NodeSelector)
}

func TestDeploymentHandlerReconcileDisabledCleansUp(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	m := runtimeCacheAuditConfig()
	m.Spec.Containers.RuntimeCache.Enable = false
	existingDS := &appsv1.DaemonSet{}
	existingDS.Name = DaemonSetName(m.Name)
	existingDS.Namespace = m.Namespace
	existingCM := &corev1.ConfigMap{}
	existingCM.Name = ConfigMapName(m.Name)
	existingCM.Namespace = m.Namespace

	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(test.TestKubeSystemNamespace(), m, existingDS, existingCM).
		Build()
	handler := deploymentHandler(kubeClient, m)

	result, err := handler.Reconcile(ctx)
	require.NoError(t, err)
	require.True(t, result.IsZero())

	require.True(t, apierrors.IsNotFound(kubeClient.Get(ctx, client.ObjectKeyFromObject(existingDS), &appsv1.DaemonSet{})))
	require.True(t, apierrors.IsNotFound(kubeClient.Get(ctx, client.ObjectKeyFromObject(existingCM), &corev1.ConfigMap{})))
	require.Empty(t, m.Status.Conditions)
}

func TestDeploymentHandlerReconcileInvalidConfigSetsCondition(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	m := runtimeCacheAuditConfig()
	m.Spec.Containers.RuntimeCache.Delegates = nil

	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(test.TestKubeSystemNamespace(), m).
		Build()
	handler := deploymentHandler(kubeClient, m)

	result, err := handler.Reconcile(ctx)
	require.NoError(t, err)
	require.True(t, result.IsZero())

	require.Len(t, m.Status.Conditions, 1)
	require.Equal(t, v1alpha2.RuntimeCacheScanningDegraded, m.Status.Conditions[0].Type)
	require.Equal(t, corev1.ConditionTrue, m.Status.Conditions[0].Status)
	require.Equal(t, "InvalidDelegateConfig", m.Status.Conditions[0].Reason)
}

func TestDeploymentHandlerReconcileErrorSetsConditionAndEvent(t *testing.T) {
	ctx := context.Background()
	scheme := testScheme(t)
	m := runtimeCacheAuditConfig()
	recorder := record.NewFakeRecorder(1)
	expectedErr := fmt.Errorf("image resolver failed")

	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(test.TestKubeSystemNamespace(), m).
		Build()
	handler := deploymentHandler(kubeClient, m)
	handler.ContainerImageResolver = &fakeMondoo.ContainerImageResolverMock{
		CnspecImageFunc: func(userImage, userTag, userDigest string, skipResolveImage bool) (string, error) {
			return "", expectedErr
		},
	}
	handler.EventRecorder = recorder

	result, err := handler.Reconcile(ctx)
	require.ErrorIs(t, err, expectedErr)
	require.True(t, result.IsZero())

	require.Len(t, m.Status.Conditions, 1)
	require.Equal(t, v1alpha2.RuntimeCacheScanningDegraded, m.Status.Conditions[0].Type)
	require.Equal(t, corev1.ConditionTrue, m.Status.Conditions[0].Status)
	require.Equal(t, "RuntimeCacheReconcileFailed", m.Status.Conditions[0].Reason)
	require.Contains(t, m.Status.Conditions[0].Message, expectedErr.Error())
	assert.Contains(t, <-recorder.Events, "Warning RuntimeCacheReconcileFailed")
}

func TestDaemonSetDegraded(t *testing.T) {
	tests := []struct {
		name string
		ds   appsv1.DaemonSet
		want bool
	}{
		{
			name: "new daemonset not observed yet",
			ds: appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status:     appsv1.DaemonSetStatus{ObservedGeneration: 0},
			},
			want: false,
		},
		{
			name: "observed with no desired pods",
			ds: appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status: appsv1.DaemonSetStatus{
					ObservedGeneration:     1,
					DesiredNumberScheduled: 0,
				},
			},
			want: true,
		},
		{
			name: "scheduled but not ready",
			ds: appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status: appsv1.DaemonSetStatus{
					ObservedGeneration:     1,
					DesiredNumberScheduled: 2,
					CurrentNumberScheduled: 2,
					NumberReady:            0,
				},
			},
			want: true,
		},
		{
			name: "unavailable pod",
			ds: appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status: appsv1.DaemonSetStatus{
					ObservedGeneration:     1,
					DesiredNumberScheduled: 2,
					CurrentNumberScheduled: 2,
					NumberReady:            2,
					NumberUnavailable:      1,
				},
			},
			want: true,
		},
		{
			name: "ready",
			ds: appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status: appsv1.DaemonSetStatus{
					ObservedGeneration:     1,
					DesiredNumberScheduled: 2,
					CurrentNumberScheduled: 2,
					NumberReady:            2,
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, daemonSetDegraded(&tt.ds))
		})
	}
}

func deploymentHandler(kubeClient client.Client, m *v1alpha2.MondooAuditConfig) *DeploymentHandler {
	return &DeploymentHandler{
		Mondoo:                 m,
		KubeClient:             kubeClient,
		MondooOperatorConfig:   &v1alpha2.MondooOperatorConfig{},
		ContainerImageResolver: fakeMondoo.NewNoOpContainerImageResolver(),
	}
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := clientgoscheme.Scheme
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	return scheme
}

var _ mondoo.ContainerImageResolver = fakeMondoo.NewNoOpContainerImageResolver()
