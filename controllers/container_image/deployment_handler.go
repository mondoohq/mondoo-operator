// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package container_image

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

var logger = ctrl.Log.WithName("k8s-images-scanning")

type DeploymentHandler struct {
	KubeClient             client.Client
	Mondoo                 *v1alpha2.MondooAuditConfig
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
}

func (n *DeploymentHandler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	// TODO: remove in next version
	// Delete the old container scanning cronjob if it exists
	if err := k8s.DeleteIfExists(ctx,
		n.KubeClient,
		&batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{
			Name:      OldCronJobName(n.Mondoo.Name),
			Namespace: n.Mondoo.Namespace,
		}}); err != nil {
		return ctrl.Result{}, err
	}

	// TODO: KubernetesResources.ContainerImageScanning is a deprecated setting
	if !n.Mondoo.Spec.KubernetesResources.ContainerImageScanning && !n.Mondoo.Spec.Containers.Enable {
		return ctrl.Result{}, n.down(ctx)
	}

	if err := n.syncCronJob(ctx); err != nil {
		return ctrl.Result{}, err
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

	updated, err := n.syncConfigMap(ctx, clusterUid)
	if err != nil {
		return err
	}

	// TODO: for CronJob we might consider triggering the CronJob now after the ConfigMap has been changed. It will make sense from the
	// user perspective to want to run the jobs after you have updated the config.
	if updated {
		logger.Info(
			"Inventory ConfigMap was just updated. The job will use the new config during the next scheduled run.",
			"namespace", n.Mondoo.Namespace,
			"name", CronJobName(n.Mondoo.Name))
	}

	// check whether we have private registry pull secrets
	privateRegistriesSecretName := "mondoo-private-registries-secrets"
	if n.Mondoo.Spec.Scanner.PrivateRegistriesPullSecretRef.Name != "" {
		privateRegistriesSecretName = n.Mondoo.Spec.Scanner.PrivateRegistriesPullSecretRef.Name
	}
	privateRegistriesSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      privateRegistriesSecretName,
			Namespace: n.Mondoo.Namespace,
		},
	}
	found, err := k8s.CheckIfExists(ctx, n.KubeClient, privateRegistriesSecret, privateRegistriesSecret)
	if err != nil {
		return err
	}
	if !found {
		logger.Info("private registries pull secret not found",
			" namespace=", n.Mondoo.Namespace,
			" secretname=", privateRegistriesSecretName)
		logger.Info("trying to fetch imagePullSecrets for each discovered image")
		privateRegistriesSecretName = ""
	}

	existing := &batchv1.CronJob{}
	desired := CronJob(mondooClientImage, integrationMrn, clusterUid, privateRegistriesSecretName, *n.Mondoo, *n.MondooOperatorConfig)
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

	updateImageScanningConditions(n.Mondoo, !k8s.AreCronJobsSuccessful(cronJobs))
	return nil
}

// syncConfigMap syncs the inventory ConfigMap. Returns a boolean indicating whether the ConfigMap has been updated. It
// can only be "true", if the ConfigMap existed before this reconcile cycle and the inventory was different from the
// desired state.
func (n *DeploymentHandler) syncConfigMap(ctx context.Context, clusterUid string) (bool, error) {
	existing := &corev1.ConfigMap{}

	integrationMrn, err := k8s.TryGetIntegrationMrnForAuditConfig(ctx, n.KubeClient, *n.Mondoo)
	if err != nil {
		logger.Error(err, "failed to retrieve IntegrationMRN")
		return false, err
	}

	desired, err := ConfigMap(integrationMrn, clusterUid, *n.Mondoo)
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

	// Clear any remnant status
	updateImageScanningConditions(n.Mondoo, false)

	return nil
}
