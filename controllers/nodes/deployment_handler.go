/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package nodes

import (
	"context"
	"reflect"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
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

	if err := n.syncCronJob(ctx); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (n *DeploymentHandler) syncCronJob(ctx context.Context) error {
	mondooClientImage, err := n.ContainerImageResolver.MondooClientImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "Failed to resolve mondoo-client container image")
		return err
	}

	mondooOperatorImage, err := n.ContainerImageResolver.MondooOperatorImage(
		"", "", n.MondooOperatorConfig.Spec.SkipContainerResolution)
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

		existing := &batchv1.CronJob{}
		desired := CronJob(mondooClientImage, node, *n.Mondoo, n.IsOpenshift)

		if err := ctrl.SetControllerReference(n.Mondoo, desired, n.KubeClient.Scheme()); err != nil {
			logger.Error(err, "Failed to set ControllerReference", "namespace", desired.Namespace, "name", desired.Name)
			return err
		}

		created, err := k8s.CreateIfNotExist(ctx, n.KubeClient, existing, desired)
		if err != nil {
			logger.Error(err, "Failed to create CronJob", "namespace", desired.Namespace, "name", desired.Name)
			return err
		}

		if created {
			logger.Info("Created CronJob", "namespace", desired.Namespace, "name", desired.Name)
			continue
		}

		if !k8s.AreCronJobsEqual(*existing, *desired) {
			existing.Spec.JobTemplate = desired.Spec.JobTemplate
			existing.SetOwnerReferences(desired.GetOwnerReferences())

			if err := n.KubeClient.Update(ctx, existing); err != nil {
				logger.Error(err, "Failed to update CronJob", "namespace", existing.Namespace, "name", existing.Name)
				return err
			}
		}
	}

	if err := n.syncGCCronjob(ctx, mondooOperatorImage, clusterUid); err != nil {
		return err
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

	updateNodeConditions(n.Mondoo, !k8s.AreCronJobsSuccessful(cronJobs))
	return nil
}

// syncConfigMap syncs the inventory ConfigMap. Returns a boolean indicating whether the ConfigMap has been updated. It
// can only be "true", if the ConfigMap existed before this reconcile cycle and the inventory was different from the
// desired state.
func (n *DeploymentHandler) syncConfigMap(ctx context.Context, node corev1.Node, clusterUid string) (bool, error) {
	existing := &corev1.ConfigMap{}

	integrationMrn, err := k8s.TryGetIntegrationMrnForAuditConfig(ctx, n.KubeClient, *n.Mondoo)
	if err != nil {
		logger.Error(err, "failed to retrieve IntegrationMRN")
		return false, err
	}

	desired, err := ConfigMap(node, integrationMrn, clusterUid, *n.Mondoo)
	if err != nil {
		logger.Error(err, "failed to generate desired ConfigMap with inventory")
		return false, err
	}

	if err := ctrl.SetControllerReference(n.Mondoo, desired, n.KubeClient.Scheme()); err != nil {
		logger.Error(err, "Failed to set ControllerReference", "namespace", desired.Namespace, "name", desired.Name)
		return false, err
	}

	created, err := k8s.CreateIfNotExist(ctx, n.KubeClient, existing, desired)
	if err != nil {
		logger.Error(err, "Failed to create inventory ConfigMap", "namespace", desired.Namespace, "name", desired.Name)
		return false, err
	}

	if created {
		logger.Info("Created inventory ConfigMap", "namespace", desired.Namespace, "name", desired.Name)
		return false, nil
	}

	updated := false
	if existing.Data["inventory"] != desired.Data["inventory"] ||
		!reflect.DeepEqual(existing.GetOwnerReferences(), desired.GetOwnerReferences()) {
		existing.Data["inventory"] = desired.Data["inventory"]
		existing.SetOwnerReferences(desired.GetOwnerReferences())

		if err := n.KubeClient.Update(ctx, existing); err != nil {
			logger.Error(err, "Failed to update inventory ConfigMap", "namespace", existing.Namespace, "name", existing.Name)
			return false, err
		}
		updated = true
	}
	return updated, nil
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
	updateNodeConditions(n.Mondoo, false)

	return nil
}
