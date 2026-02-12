// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"context"
	"maps"
	"slices"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var logger = ctrl.Log.WithName("node-scanning")

type DeploymentHandler struct {
	KubeClient             client.Client
	Mondoo                 *v1alpha2.MondooAuditConfig
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
	IsOpenshift            bool
}

func (n *DeploymentHandler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	if !n.Mondoo.Spec.Nodes.Enable {
		return ctrl.Result{}, n.down(ctx)
	}

	switch n.Mondoo.Spec.Nodes.Style {
	case v1alpha2.NodeScanStyle_CronJob:
		if err := n.syncCronJob(ctx); err != nil {
			return ctrl.Result{}, err
		}
	case v1alpha2.NodeScanStyle_Deployment, v1alpha2.NodeScanStyle_DaemonSet:
		if err := n.syncDaemonSet(ctx); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (n *DeploymentHandler) syncCronJob(ctx context.Context) error {
	mondooClientImage, err := n.ContainerImageResolver.CnspecImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.Mondoo.Spec.Scanner.Image.Digest, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "Failed to resolve mondoo-client container image")
		return err
	}

	clusterUid, err := k8s.GetClusterUID(ctx, n.KubeClient, logger)
	if err != nil {
		logger.Error(err, "Failed to get cluster's UID")
		return err
	}

	nodes := &corev1.NodeList{}
	if err := n.KubeClient.List(ctx, nodes); err != nil {
		logger.Error(err, "Failed to list cluster nodes")
		return err
	}

	// Delete DaemonSet if it exists
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: DaemonSetName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, ds); err != nil {
		logger.Error(err, "Failed to clean up node scanning DaemonSet", "namespace", ds.Namespace, "name", ds.Name)
		return err
	}

	// Create/update CronJobs for nodes
	for _, node := range nodes.Items {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: ConfigMapNameWithNode(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace}}
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, cm); err != nil {
			logger.Error(err, "Failed to clean up old ConfigMap for node scanning", "namespace", cm.Namespace, "name", cm.Name)
			return err
		}

		if err := n.syncConfigMap(ctx, clusterUid); err != nil {
			return err
		}

		cronJob := CronJob(mondooClientImage, node, n.Mondoo, n.IsOpenshift, *n.MondooOperatorConfig)
		op, err := k8s.Apply(ctx, n.KubeClient, cronJob, n.Mondoo, logger, k8s.DefaultApplyOptions())
		if err != nil {
			logger.Error(err, "Failed to apply CronJob", "namespace", cronJob.Namespace, "name", cronJob.Name)
			return err
		}

		switch op {
		case k8s.ApplyCreated:
			if err := mondoo.UpdateMondooAuditConfig(ctx, n.KubeClient, n.Mondoo, logger); err != nil {
				logger.Error(err, "Failed to update MondooAuditConfig", "namespace", n.Mondoo.Namespace, "name", n.Mondoo.Name)
				return err
			}
		case k8s.ApplyUpdated:
			// Remove old Jobs so they don't continue running with stale config
			if err := n.KubeClient.DeleteAllOf(ctx, &batchv1.Job{},
				client.InNamespace(n.Mondoo.Namespace),
				client.MatchingLabels(NodeScanningLabels(*n.Mondoo)),
				client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
				logger.Error(err, "Failed to clean up old Jobs after CronJob update")
				return err
			}
		}
	}

	// Delete dangling CronJobs for nodes that have been deleted from the cluster.
	if err := n.cleanupCronJobsForDeletedNodes(ctx, *nodes); err != nil {
		return err
	}

	// List the CronJobs again after they have been synced.
	cronJobs, err := n.getCronJobsForAuditConfig(ctx)
	if err != nil {
		return err
	}

	// Get Pods for this CronJob
	pods := &corev1.PodList{}
	if len(cronJobs) > 0 {
		opts := &client.ListOptions{
			Namespace:     n.Mondoo.Namespace,
			LabelSelector: labels.SelectorFromSet(NodeScanningLabels(*n.Mondoo)),
		}
		err = n.KubeClient.List(ctx, pods, opts)
		if err != nil {
			logger.Error(err, "Failed to list Pods for Node Scanning")
			return err
		}
	}

	updateNodeConditions(n.Mondoo, !k8s.AreCronJobsSuccessful(cronJobs), pods)

	// Clean up any leftover GC CronJobs from previous versions
	gcCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: GarbageCollectCronJobName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, gcCronJob); err != nil {
		logger.Error(err, "Failed to clean up node garbage collect CronJob", "namespace", gcCronJob.Namespace, "name", gcCronJob.Name)
		return err
	}

	return nil
}

func (n *DeploymentHandler) syncDaemonSet(ctx context.Context) error {
	mondooClientImage, err := n.ContainerImageResolver.CnspecImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.Mondoo.Spec.Scanner.Image.Digest, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "Failed to resolve mondoo-client container image")
		return err
	}

	clusterUid, err := k8s.GetClusterUID(ctx, n.KubeClient, logger)
	if err != nil {
		logger.Error(err, "Failed to get cluster's UID")
		return err
	}

	nodes := &corev1.NodeList{}
	if err := n.KubeClient.List(ctx, nodes); err != nil {
		logger.Error(err, "Failed to list cluster nodes")
		return err
	}

	// Create/update Deployments for nodes
	for _, node := range nodes.Items {
		// Delete CronJob if it exists
		cronJob := &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: CronJobName(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace},
		}
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, cronJob); err != nil {
			logger.Error(err, "Failed to clean up node scanning CronJob", "namespace", cronJob.Namespace, "name", cronJob.Name)
			return err
		}

		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: ConfigMapNameWithNode(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace}}
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, cm); err != nil {
			logger.Error(err, "Failed to clean up old ConfigMap for node scanning", "namespace", cm.Namespace, "name", cm.Name)
			return err
		}

		if err := n.syncConfigMap(ctx, clusterUid); err != nil {
			return err
		}

		if n.Mondoo.Spec.Nodes.Style == v1alpha2.NodeScanStyle_Deployment {
			dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: DeploymentName(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace}}
			if err := k8s.DeleteIfExists(ctx, n.KubeClient, dep); err != nil {
				logger.Error(err, "Failed to clean up node scanning Deployment", "namespace", dep.Namespace, "name", dep.Name)
			}
		}
	}

	tolerations := make(map[corev1.Toleration]struct{})
	for _, node := range nodes.Items {
		for _, toleration := range k8s.TaintsToTolerations(node.Spec.Taints) {
			tolerations[toleration] = struct{}{}
		}
	}

	ds := DaemonSet(*n.Mondoo, n.IsOpenshift, mondooClientImage, *n.MondooOperatorConfig, slices.Collect(maps.Keys(tolerations)))
	op, err := k8s.Apply(ctx, n.KubeClient, ds, n.Mondoo, logger, k8s.DefaultApplyOptions())
	if err != nil {
		logger.Error(err, "Failed to apply DaemonSet", "namespace", ds.Namespace, "name", ds.Name)
		return err
	}

	if op == k8s.ApplyCreated {
		if err := mondoo.UpdateMondooAuditConfig(ctx, n.KubeClient, n.Mondoo, logger); err != nil {
			logger.Error(err, "Failed to update MondooAuditConfig", "namespace", n.Mondoo.Namespace, "name", n.Mondoo.Name)
			return err
		}
	}

	if err := n.KubeClient.Get(ctx, client.ObjectKeyFromObject(ds), ds); err != nil {
		logger.Error(err, "Failed to get DaemonSet", "namespace", ds.Namespace, "name", ds.Name)
	}

	// Get Pods for these Deployments
	pods := &corev1.PodList{}
	opts := &client.ListOptions{
		Namespace:     n.Mondoo.Namespace,
		LabelSelector: labels.SelectorFromSet(NodeScanningLabels(*n.Mondoo)),
	}
	err = n.KubeClient.List(ctx, pods, opts)
	if err != nil {
		logger.Error(err, "Failed to list Pods for Node Scanning")
		return err
	}

	updateNodeConditions(n.Mondoo, ds.Status.CurrentNumberScheduled < ds.Status.DesiredNumberScheduled, pods)

	// Clean up any leftover GC CronJobs from previous versions
	gcCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: GarbageCollectCronJobName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, gcCronJob); err != nil {
		logger.Error(err, "Failed to clean up node garbage collect CronJob", "namespace", gcCronJob.Namespace, "name", gcCronJob.Name)
		return err
	}

	return nil
}

// syncConfigMap syncs the inventory ConfigMap using Server-Side Apply.
func (n *DeploymentHandler) syncConfigMap(ctx context.Context, clusterUid string) error {
	integrationMrn, err := k8s.TryGetIntegrationMrnForAuditConfig(ctx, n.KubeClient, *n.Mondoo)
	if err != nil {
		logger.Error(err, "failed to retrieve IntegrationMRN")
		return err
	}

	cm, err := ConfigMap(integrationMrn, clusterUid, *n.Mondoo)
	if err != nil {
		logger.Error(err, "failed to generate ConfigMap")
		return err
	}

	if _, err := k8s.Apply(ctx, n.KubeClient, cm, n.Mondoo, logger, k8s.DefaultApplyOptions()); err != nil {
		logger.Error(err, "Failed to apply ConfigMap", "namespace", cm.Namespace, "name", cm.Name)
		return err
	}

	return nil
}

// cleanupCronJobsForDeletedNodes deletes dangling CronJobs for nodes that have been deleted from the cluster.
func (n *DeploymentHandler) cleanupCronJobsForDeletedNodes(ctx context.Context, currentNodes corev1.NodeList) error {
	cronJobs, err := n.getCronJobsForAuditConfig(ctx)
	if err != nil {
		return err
	}

	for _, c := range cronJobs {
		// Check if the node for that CronJob is still present in the cluster.
		found := false
		for _, node := range currentNodes.Items {
			if CronJobName(n.Mondoo.Name, node.Name) == c.Name {
				found = true
				break
			}
		}

		// If the node is still there, there is nothing to update.
		if found {
			continue
		}

		// If the node for the CronJob has been deleted from the cluster, the CronJob needs to be deleted.
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, &c); err != nil {
			logger.Error(err, "Failed to deleted CronJob", "namespace", c.Namespace, "name", c.Name)
			return err
		}
		logger.Info("Deleted CronJob", "namespace", c.Namespace, "name", c.Name)
	}
	return nil
}

func (n *DeploymentHandler) getCronJobsForAuditConfig(ctx context.Context) ([]batchv1.CronJob, error) {
	cronJobs := &batchv1.CronJobList{}
	cronJobLabels := NodeScanningLabels(*n.Mondoo)

	// Lists only the CronJobs in the namespace of the MondooAuditConfig and only the ones that exactly match our labels.
	listOpts := &client.ListOptions{Namespace: n.Mondoo.Namespace, LabelSelector: labels.SelectorFromSet(cronJobLabels)}
	if err := n.KubeClient.List(ctx, cronJobs, listOpts); err != nil {
		logger.Error(err, "Failed to list CronJobs in namespace", "namespace", n.Mondoo.Namespace)
		return nil, err
	}
	return cronJobs.Items, nil
}

func (n *DeploymentHandler) down(ctx context.Context) error {
	nodes := &corev1.NodeList{}
	if err := n.KubeClient.List(ctx, nodes); err != nil {
		logger.Error(err, "Failed to list cluster nodes")
		return err
	}

	for _, node := range nodes.Items {
		cronJob := &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: CronJobName(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace},
		}
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, cronJob); err != nil {
			logger.Error(err, "Failed to clean up node scanning CronJob", "namespace", cronJob.Namespace, "name", cronJob.Name)
			return err
		}
	}

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: DaemonSetName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, ds); err != nil {
		logger.Error(err, "Failed to clean up node scanning DaemonSet", "namespace", ds.Namespace, "name", ds.Name)
		return err
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, configMap); err != nil {
		logger.Error(err, "Failed to clean up inventory ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
		return err
	}

	gcCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: GarbageCollectCronJobName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, gcCronJob); err != nil {
		logger.Error(err, "Failed to clean up node garbage collect CronJob", "namespace", gcCronJob.Namespace, "name", gcCronJob.Name)
		return err
	}

	// Update any remnant conditions
	updateNodeConditions(n.Mondoo, false, &corev1.PodList{})

	return nil
}
