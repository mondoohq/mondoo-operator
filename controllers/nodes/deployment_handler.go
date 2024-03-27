// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"context"

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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

	if n.Mondoo.Spec.Nodes.Style == v1alpha2.NodeScanStyle_CronJob {
		if err := n.syncCronJob(ctx); err != nil {
			return ctrl.Result{}, err
		}
	} else if n.Mondoo.Spec.Nodes.Style == v1alpha2.NodeScanStyle_Deployment {
		if err := n.syncDeployment(ctx); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (n *DeploymentHandler) syncCronJob(ctx context.Context) error {
	mondooClientImage, err := n.ContainerImageResolver.CnspecImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "Failed to resolve mondoo-client container image")
		return err
	}

	mondooOperatorImage, err := n.ContainerImageResolver.MondooOperatorImage(ctx, "", "", n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "Failed to resolve mondoo-operator container image")
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

	// Create/update CronJobs for nodes
	for _, node := range nodes.Items {
		updated, err := n.syncConfigMap(ctx, node, clusterUid)
		if err != nil {
			return err
		}

		// TODO: for CronJob we might consider triggering the CronJob now after the ConfigMap has been changed. It will make sense from the
		// user perspective to want to run the jobs after you have updated the config.
		if updated {
			logger.Info(
				"Inventory ConfigMap was just updated. The job will use the new config during the next scheduled run.",
				"namespace", n.Mondoo.Namespace,
				"name", CronJobName(n.Mondoo.Name, node.Name))
		}

		cronJob := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: CronJobName(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace}}
		op, err := k8s.CreateOrUpdate(ctx, n.KubeClient, cronJob, n.Mondoo, logger, func() error {
			UpdateCronJob(cronJob, mondooClientImage, node, n.Mondoo, n.IsOpenshift, *n.MondooOperatorConfig)
			return nil
		})
		if err != nil {
			return err
		}

		if op == controllerutil.OperationResultCreated {
			err = mondoo.UpdateMondooAuditConfig(ctx, n.KubeClient, n.Mondoo, logger)
			if err != nil {
				logger.Error(err, "Failed to update MondooAuditConfig", "namespace", n.Mondoo.Namespace, "name", n.Mondoo.Name)
				return err
			}
			continue
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
			LabelSelector: labels.SelectorFromSet(CronJobLabels(*n.Mondoo)),
		}
		err = n.KubeClient.List(ctx, pods, opts)
		if err != nil {
			logger.Error(err, "Failed to list Pods for Node Scanning")
			return err
		}
	}

	updateNodeConditions(n.Mondoo, !k8s.AreCronJobsSuccessful(cronJobs), pods)

	if err := n.syncGCCronjob(ctx, mondooOperatorImage, clusterUid); err != nil {
		return err
	}
	return nil
}

func (n *DeploymentHandler) syncDeployment(ctx context.Context) error {
	mondooClientImage, err := n.ContainerImageResolver.CnspecImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "Failed to resolve mondoo-client container image")
		return err
	}

	mondooOperatorImage, err := n.ContainerImageResolver.MondooOperatorImage(ctx, "", "", n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "Failed to resolve mondoo-operator container image")
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
		updated, err := n.syncConfigMap(ctx, node, clusterUid)
		if err != nil {
			return err
		}

		if updated {
			logger.Info(
				"Inventory ConfigMap was just updated. The deployment will use the new config during the next scheduled run.",
				"namespace", n.Mondoo.Namespace,
				"name", DeploymentName(n.Mondoo.Name, node.Name))
		}

		dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: DeploymentName(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace}}
		op, err := k8s.CreateOrUpdate(ctx, n.KubeClient, dep, n.Mondoo, logger, func() error {
			UpdateDeployment(dep, node, *n.Mondoo, n.IsOpenshift, mondooClientImage, *n.MondooOperatorConfig)
			return nil
		})
		if err != nil {
			return err
		}

		if op == controllerutil.OperationResultCreated {
			err = mondoo.UpdateMondooAuditConfig(ctx, n.KubeClient, n.Mondoo, logger)
			if err != nil {
				logger.Error(err, "Failed to update MondooAuditConfig", "namespace", n.Mondoo.Namespace, "name", n.Mondoo.Name)
				return err
			}
			continue
		}
	}

	// Delete dangling Deployments for nodes that have been deleted from the cluster.
	if err := n.cleanupDeploymentsForDeletedNodes(ctx, *nodes); err != nil {
		return err
	}

	// List the Deployments again after they have been synced.
	deployments, err := n.getDeploymentsForAuditConfig(ctx)
	if err != nil {
		return err
	}

	// Get Pods for these Deployments
	pods := &corev1.PodList{}
	if len(deployments) > 0 {
		opts := &client.ListOptions{
			Namespace:     n.Mondoo.Namespace,
			LabelSelector: labels.SelectorFromSet(CronJobLabels(*n.Mondoo)),
		}
		err = n.KubeClient.List(ctx, pods, opts)
		if err != nil {
			logger.Error(err, "Failed to list Pods for Node Scanning")
			return err
		}
	}

	deploymentsDegraded := false
	for _, d := range deployments {
		if d.Status.ReadyReplicas < *d.Spec.Replicas {
			deploymentsDegraded = true
			break
		}
	}

	updateNodeConditions(n.Mondoo, deploymentsDegraded, pods)

	if err := n.syncGCCronjob(ctx, mondooOperatorImage, clusterUid); err != nil {
		return err
	}
	return nil
}

// syncConfigMap syncs the inventory ConfigMap. Returns a boolean indicating whether the ConfigMap has been updated. It
// can only be "true", if the ConfigMap existed before this reconcile cycle and the inventory was different from the
// desired state.
func (n *DeploymentHandler) syncConfigMap(ctx context.Context, node corev1.Node, clusterUid string) (bool, error) {
	integrationMrn, err := k8s.TryGetIntegrationMrnForAuditConfig(ctx, n.KubeClient, *n.Mondoo)
	if err != nil {
		logger.Error(err, "failed to retrieve IntegrationMRN")
		return false, err
	}

	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace}}
	op, err := k8s.CreateOrUpdate(ctx, n.KubeClient, cm, n.Mondoo, logger, func() error {
		return UpdateConfigMap(cm, node, integrationMrn, clusterUid, *n.Mondoo)
	})
	if err != nil {
		return false, err
	}

	return op == controllerutil.OperationResultUpdated, nil
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

		configMap := &corev1.ConfigMap{}
		configMap.Name = ConfigMapName(n.Mondoo.Name, c.Spec.JobTemplate.Spec.Template.Spec.NodeName)
		configMap.Namespace = n.Mondoo.Namespace
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, configMap); err != nil {
			logger.Error(err, "Failed to delete ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
			return err
		}
	}
	return nil
}

// cleanupDeploymentsForDeletedNodes deletes dangling Deployments for nodes that have been deleted from the cluster.
func (n *DeploymentHandler) cleanupDeploymentsForDeletedNodes(ctx context.Context, currentNodes corev1.NodeList) error {
	deployments, err := n.getDeploymentsForAuditConfig(ctx)
	if err != nil {
		return err
	}

	for _, d := range deployments {
		// Check if the node for that Deployment is still present in the cluster.
		found := false
		for _, node := range currentNodes.Items {
			if DeploymentName(n.Mondoo.Name, node.Name) == d.Name {
				found = true
				break
			}
		}

		// If the node is still there, there is nothing to update.
		if found {
			continue
		}

		// If the node for the Deployment has been deleted from the cluster, the Deployment needs to be deleted.
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, &d); err != nil {
			logger.Error(err, "Failed to deleted Deployment", "namespace", d.Namespace, "name", d.Name)
			return err
		}
		logger.Info("Deleted Deployment", "namespace", d.Namespace, "name", d.Name)

		configMap := &corev1.ConfigMap{}
		configMap.Name = ConfigMapName(n.Mondoo.Name, d.Spec.Template.Spec.NodeName)
		configMap.Namespace = n.Mondoo.Namespace
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, configMap); err != nil {
			logger.Error(err, "Failed to delete ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
			return err
		}
	}
	return nil
}

func (n *DeploymentHandler) syncGCCronjob(ctx context.Context, mondooOperatorImage, clusterUid string) error {
	existing := &batchv1.CronJob{}
	desired := GarbageCollectCronJob(mondooOperatorImage, clusterUid, *n.Mondoo)

	if err := ctrl.SetControllerReference(n.Mondoo, desired, n.KubeClient.Scheme()); err != nil {
		logger.Error(err, "Failed to set ControllerReference", "namespace", desired.Namespace, "name", desired.Name)
		return err
	}

	created, err := k8s.CreateIfNotExist(ctx, n.KubeClient, existing, desired)
	if err != nil {
		logger.Error(err, "Failed to create garbage collect CronJob", "namespace", desired.Namespace, "name", desired.Name)
		return err
	}

	if created {
		logger.Info("Created garbage collect CronJob", "namespace", desired.Namespace, "name", desired.Name)
		return nil
	}

	if !k8s.AreCronJobsEqual(*existing, *desired) {
		existing.Spec.JobTemplate = desired.Spec.JobTemplate
		existing.Spec.ConcurrencyPolicy = desired.Spec.ConcurrencyPolicy
		existing.SetOwnerReferences(desired.GetOwnerReferences())

		if err := n.KubeClient.Update(ctx, existing); err != nil {
			logger.Error(err, "Failed to update garbage collect CronJob", "namespace", existing.Namespace, "name", existing.Name)
			return err
		}
	}
	return nil
}

func (n *DeploymentHandler) getCronJobsForAuditConfig(ctx context.Context) ([]batchv1.CronJob, error) {
	cronJobs := &batchv1.CronJobList{}
	cronJobLabels := CronJobLabels(*n.Mondoo)

	// Lists only the CronJobs in the namespace of the MondooAuditConfig and only the ones that exactly match our labels.
	listOpts := &client.ListOptions{Namespace: n.Mondoo.Namespace, LabelSelector: labels.SelectorFromSet(cronJobLabels)}
	if err := n.KubeClient.List(ctx, cronJobs, listOpts); err != nil {
		logger.Error(err, "Failed to list CronJobs in namespace", "namespace", n.Mondoo.Namespace)
		return nil, err
	}
	return cronJobs.Items, nil
}

func (n *DeploymentHandler) getDeploymentsForAuditConfig(ctx context.Context) ([]appsv1.Deployment, error) {
	deps := &appsv1.DeploymentList{}
	depLabels := CronJobLabels(*n.Mondoo)

	// Lists only the Deployments in the namespace of the MondooAuditConfig and only the ones that exactly match our labels.
	listOpts := &client.ListOptions{Namespace: n.Mondoo.Namespace, LabelSelector: labels.SelectorFromSet(depLabels)}
	if err := n.KubeClient.List(ctx, deps, listOpts); err != nil {
		logger.Error(err, "Failed to list Deployments in namespace", "namespace", n.Mondoo.Namespace)
		return nil, err
	}
	return deps.Items, nil
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

		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace},
		}
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, configMap); err != nil {
			logger.Error(err, "Failed to clean up inventory ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
			return err
		}
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
