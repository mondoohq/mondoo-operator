// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers/resource_monitor/scan_api_store"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

var logger = ctrl.Log.WithName("k8s-resources-scanning")

const (
	workloadDeploymentConfigMapNameTemplate = `%s-deploy`
	WorkloadDeploymentNameTemplate          = `%s-workload`
)

type DeploymentHandler struct {
	KubeClient             client.Client
	Mondoo                 *v1alpha2.MondooAuditConfig
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
	ScanApiStore           scan_api_store.ScanApiStore
}

func (n *DeploymentHandler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	if !n.Mondoo.Spec.KubernetesResources.Enable {
		n.ScanApiStore.Delete(scanapi.ScanApiServiceUrl(*n.Mondoo))
		return ctrl.Result{}, n.down(ctx)
	}

	if err := scan_api_store.HandleAuditConfig(ctx, n.KubeClient, n.ScanApiStore, *n.Mondoo); err != nil {
		logger.Error(
			err, "failed to add scan API URL to the store for audit config",
			"namespace", n.Mondoo.Namespace,
			"name", n.Mondoo.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, n.syncCronJob(ctx)
}

func (n *DeploymentHandler) syncCronJob(ctx context.Context) error {
	mondooOperatorImage, err := n.ContainerImageResolver.MondooOperatorImage(ctx, "", "", false)
	if err != nil {
		logger.Error(err, "Failed to resolve mondoo-operator container image")
		return err
	}

	integrationMrn, err := k8s.TryGetIntegrationMrnForAuditConfig(ctx, n.KubeClient, *n.Mondoo)
	if err != nil {
		logger.Error(err,
			"failed to retrieve integration-mrn for MondooAuditConfig", "namespace", n.Mondoo.Namespace, "name", n.Mondoo.Name)
		return err
	}

	clusterUid, err := k8s.GetClusterUID(ctx, n.KubeClient, logger)
	if err != nil {
		logger.Error(err, "Failed to get cluster's UID")
		return err
	}

	existing := &batchv1.CronJob{}
	desired := CronJob(mondooOperatorImage, integrationMrn, clusterUid, n.Mondoo)
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
	} else if !k8s.AreCronJobsEqual(*existing, *desired) {
		existing.Spec.JobTemplate = desired.Spec.JobTemplate
		existing.Spec.Schedule = desired.Spec.Schedule
		existing.Spec.ConcurrencyPolicy = desired.Spec.ConcurrencyPolicy
		existing.SetOwnerReferences(desired.GetOwnerReferences())

		// Remove any old jobs because they won't be updated when the cronjob changes
		if err := n.KubeClient.DeleteAllOf(ctx, &batchv1.Job{},
			client.InNamespace(n.Mondoo.Namespace),
			client.MatchingLabels(CronJobLabels(*n.Mondoo)),
			client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
			return err
		}

		if err := n.KubeClient.Update(ctx, existing); err != nil {
			logger.Error(err, "Failed to update CronJob", "namespace", existing.Namespace, "name", existing.Name)
			return err
		}
	}

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
			logger.Error(err, "Failed to list Pods for scan Kubernetes Reosurce Scanning")
			return err
		}
	}

	updateWorkloadsConditions(n.Mondoo, !k8s.AreCronJobsSuccessful(cronJobs), pods)
	return n.cleanupWorkloadDeployment(ctx)
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
	cronJob := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: CronJobName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace}}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, cronJob); err != nil {
		logger.Error(
			err, "failed to clean up Kubernetes resource scanning CronJob", "namespace", cronJob.Namespace, "name", cronJob.Name)
		return err
	}

	if err := n.cleanupWorkloadDeployment(ctx); err != nil {
		return err
	}

	// Clear any remnant status
	updateWorkloadsConditions(n.Mondoo, false, &corev1.PodList{})

	return nil
}

// TODO: remove with 0.5.0 release
// This can be removed once we believe enough time has passed where the old-style named
// Deployment for workloads has been replaced and removed to keep us from orphaning the old-style Deployment.
func (n *DeploymentHandler) cleanupWorkloadDeployment(ctx context.Context) error {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: n.Mondoo.Namespace,
			Name:      fmt.Sprintf(WorkloadDeploymentNameTemplate, n.Mondoo.Name),
		},
	}

	if err := k8s.DeleteIfExists(ctx, n.KubeClient, dep); err != nil {
		logger.Error(err, "failed to clean up old Deployment for workloads", "namespace", dep.Namespace, "name", dep.Name)
		return err
	}

	cfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(workloadDeploymentConfigMapNameTemplate, n.Mondoo.Name),
			Namespace: n.Mondoo.Namespace,
		},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, cfgMap); err != nil {
		logger.Error(err, "failed to cleanup configmap", "namespace", cfgMap.Namespace, "name", cfgMap.Name)
		return err
	}
	return nil
}
