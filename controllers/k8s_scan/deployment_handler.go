/*
Copyright 2022 Mondoo, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
}

func (n *DeploymentHandler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	if !n.Mondoo.Spec.KubernetesResources.Enable {
		return ctrl.Result{}, n.down(ctx)
	}

	if err := n.syncCronJob(ctx); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (n *DeploymentHandler) syncCronJob(ctx context.Context) error {
	// TODO: think about overriding these images
	mondooClientImage, err := n.ContainerImageResolver.MondooOperatorImage(
		"", "", n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "Failed to resolve mondoo-client container image")
		return err
	}

	existing := &batchv1.CronJob{}
	desired := CronJob(mondooClientImage, *n.Mondoo)
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
		existing.SetOwnerReferences(desired.GetOwnerReferences())

		if err := n.KubeClient.Update(ctx, existing); err != nil {
			logger.Error(err, "Failed to update CronJob", "namespace", existing.Namespace, "name", existing.Name)
			return err
		}
	}

	cronJobs, err := n.getCronJobsForAuditConfig(ctx)
	if err != nil {
		return err
	}

	updateWorkloadsConditions(n.Mondoo, !k8s.AreCronJobsSuccessful(cronJobs))
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
	updateWorkloadsConditions(n.Mondoo, false)

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
