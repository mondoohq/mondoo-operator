// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package runtime_cache

import (
	"context"
	"fmt"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var logger = ctrl.Log.WithName("runtime-cache-scanning")

type DeploymentHandler struct {
	KubeClient             client.Client
	Mondoo                 *v1alpha2.MondooAuditConfig
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
	EventRecorder          record.EventRecorder
}

func (n *DeploymentHandler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	if !n.Mondoo.Spec.Containers.RuntimeCache.Enable {
		return ctrl.Result{}, n.down(ctx)
	}
	if err := Validate(n.Mondoo.Spec.Containers.RuntimeCache); err != nil {
		logger.Error(err, "invalid runtime cache scanner configuration")
		updateRuntimeCacheConditions(n.Mondoo, true, &corev1.PodList{}, "InvalidDelegateConfig", err.Error())
		n.recordWarningEvent("InvalidDelegateConfig", err)
		if cleanupErr := n.cleanupResources(ctx); cleanupErr != nil {
			n.markReconcileError(cleanupErr)
			return ctrl.Result{}, cleanupErr
		}
		return ctrl.Result{}, nil
	}

	if err := n.syncDaemonSet(ctx); err != nil {
		n.markReconcileError(err)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (n *DeploymentHandler) syncDaemonSet(ctx context.Context) error {
	mondooClientImage, err := n.ContainerImageResolver.CnspecImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.Mondoo.Spec.Scanner.Image.Digest, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "failed to resolve cnspec container image")
		return err
	}
	renderImage, err := n.ContainerImageResolver.ContainerImage(ctx, constants.BusyBoxImage, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "failed to resolve runtime cache config render container image")
		return err
	}

	clusterUid, err := k8s.GetClusterUID(ctx, n.KubeClient, logger)
	if err != nil {
		logger.Error(err, "failed to get cluster UID")
		return err
	}

	integrationMrn, err := k8s.TryGetIntegrationMrnForAuditConfig(ctx, n.KubeClient, *n.Mondoo)
	if err != nil {
		logger.Error(err, "failed to retrieve integration MRN")
		return err
	}

	if err := n.syncConfigMap(ctx, integrationMrn, clusterUid); err != nil {
		return err
	}

	desiredSets, err := n.desiredDaemonSets(mondooClientImage, renderImage, integrationMrn, clusterUid)
	if err != nil {
		return err
	}

	expectedNames := map[string]struct{}{}
	degraded := false
	for _, desired := range desiredSets {
		expectedNames[desired.Name] = struct{}{}
		ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
		op, err := k8s.CreateOrUpdate(ctx, n.KubeClient, ds, n.Mondoo, logger, func() error {
			k8s.UpdateDaemonSetFields(ds, desired)
			return nil
		})
		if err != nil {
			return err
		}

		if op == controllerutil.OperationResultUpdated {
			logger.Info("updated runtime cache scanner DaemonSet", "namespace", ds.Namespace, "name", ds.Name)
		}

		if err := n.KubeClient.Get(ctx, client.ObjectKeyFromObject(ds), ds); err != nil {
			logger.Error(err, "failed to get runtime cache scanner DaemonSet", "namespace", ds.Namespace, "name", ds.Name)
			return err
		}
		degraded = degraded || daemonSetDegraded(ds)
	}
	if err := n.cleanupOrphanedDaemonSets(ctx, expectedNames); err != nil {
		return err
	}

	pods, err := n.getPods(ctx)
	if err != nil {
		return err
	}
	updateRuntimeCacheConditions(n.Mondoo, degraded, pods, "", "")
	return nil
}

func (n *DeploymentHandler) desiredDaemonSets(image, renderImage, integrationMRN, clusterUID string) ([]*appsv1.DaemonSet, error) {
	cache := n.Mondoo.Spec.Containers.RuntimeCache
	if len(cache.ScannerSets) == 0 {
		ds, err := DaemonSet(image, renderImage, integrationMRN, clusterUID, n.Mondoo, *n.MondooOperatorConfig)
		if err != nil {
			return nil, err
		}
		return []*appsv1.DaemonSet{ds}, nil
	}

	out := make([]*appsv1.DaemonSet, 0, len(cache.ScannerSets))
	for _, set := range cache.ScannerSets {
		ds, err := DaemonSetForScannerSet(image, renderImage, integrationMRN, clusterUID, n.Mondoo, *n.MondooOperatorConfig, set)
		if err != nil {
			return nil, err
		}
		out = append(out, ds)
	}
	return out, nil
}

func daemonSetDegraded(ds *appsv1.DaemonSet) bool {
	if ds.Status.ObservedGeneration < ds.Generation {
		return false
	}
	if ds.Status.DesiredNumberScheduled == 0 {
		return true
	}
	return ds.Status.NumberUnavailable > 0 || ds.Status.NumberReady < ds.Status.DesiredNumberScheduled
}

func (n *DeploymentHandler) markReconcileError(err error) {
	updateRuntimeCacheConditions(n.Mondoo, true, &corev1.PodList{}, "RuntimeCacheReconcileFailed", err.Error())
	n.recordWarningEvent("RuntimeCacheReconcileFailed", err)
}

func (n *DeploymentHandler) recordWarningEvent(reason string, err error) {
	if n.EventRecorder == nil {
		return
	}
	n.EventRecorder.Event(n.Mondoo, corev1.EventTypeWarning, reason, fmt.Sprintf("Runtime cache scanner reconciliation failed: %v", err))
}

func (n *DeploymentHandler) syncConfigMap(ctx context.Context, integrationMrn, clusterUid string) error {
	desired, err := ConfigMap(integrationMrn, clusterUid, *n.Mondoo)
	if err != nil {
		logger.Error(err, "failed to generate runtime cache scanner ConfigMap")
		return err
	}

	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	if _, err := k8s.CreateOrUpdate(ctx, n.KubeClient, obj, n.Mondoo, logger, func() error {
		obj.Labels = desired.Labels
		obj.Data = desired.Data
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (n *DeploymentHandler) getPods(ctx context.Context) (*corev1.PodList, error) {
	pods := &corev1.PodList{}
	opts := &client.ListOptions{
		Namespace:     n.Mondoo.Namespace,
		LabelSelector: labels.SelectorFromSet(Labels(*n.Mondoo)),
	}
	if err := n.KubeClient.List(ctx, pods, opts); err != nil {
		logger.Error(err, "failed to list runtime cache scanner pods")
		return nil, err
	}
	return pods, nil
}

func (n *DeploymentHandler) down(ctx context.Context) error {
	if err := n.cleanupResources(ctx); err != nil {
		return err
	}
	if mondoo.FindMondooAuditConditions(n.Mondoo.Status.Conditions, v1alpha2.RuntimeCacheScanningDegraded) != nil {
		updateRuntimeCacheConditions(n.Mondoo, false, &corev1.PodList{}, "", "")
	}
	return nil
}

func (n *DeploymentHandler) cleanupResources(ctx context.Context) error {
	if err := n.cleanupOrphanedDaemonSets(ctx, map[string]struct{}{}); err != nil {
		return err
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace}}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, cm); err != nil {
		logger.Error(err, "failed to clean up runtime cache scanner ConfigMap", "namespace", cm.Namespace, "name", cm.Name)
		return err
	}
	return nil
}

func (n *DeploymentHandler) cleanupOrphanedDaemonSets(ctx context.Context, expectedNames map[string]struct{}) error {
	legacy := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: DaemonSetName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace}}
	if _, ok := expectedNames[legacy.Name]; !ok {
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, legacy); err != nil {
			logger.Error(err, "failed to clean up legacy runtime cache scanner DaemonSet", "namespace", legacy.Namespace, "name", legacy.Name)
			return err
		}
	}

	list := &appsv1.DaemonSetList{}
	if err := n.KubeClient.List(ctx, list, &client.ListOptions{
		Namespace:     n.Mondoo.Namespace,
		LabelSelector: labels.SelectorFromSet(Labels(*n.Mondoo)),
	}); err != nil {
		logger.Error(err, "failed to list runtime cache scanner DaemonSets")
		return err
	}
	for i := range list.Items {
		ds := &list.Items[i]
		if _, ok := expectedNames[ds.Name]; ok {
			continue
		}
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, ds); err != nil {
			logger.Error(err, "failed to clean up runtime cache scanner DaemonSet", "namespace", ds.Namespace, "name", ds.Name)
			return err
		}
	}
	return nil
}
