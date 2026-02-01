// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package scanapi

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

var logger = ctrl.Log.WithName("scan-api")

type DeploymentHandler struct {
	KubeClient             client.Client
	Mondoo                 *v1alpha2.MondooAuditConfig
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
	DeployOnOpenShift      bool
}

func (n *DeploymentHandler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	// If KubernetesResources is not enabled, the scan API is not needed.
	if (!n.Mondoo.Spec.KubernetesResources.Enable && !n.Mondoo.Spec.Nodes.Enable) ||
		!n.Mondoo.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, n.down(ctx)
	}

	if err := n.syncSecret(ctx); err != nil {
		return ctrl.Result{}, err
	}
	if err := n.syncDeployment(ctx); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, n.syncService(ctx)
}

// down cleans up the scan API for a given MondooAuditConfig. The function returns no errors if the scan API is already
// deleted.
func (n *DeploymentHandler) down(ctx context.Context) error {
	scanApiTokenSecret := ScanApiSecret(*n.Mondoo)
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, scanApiTokenSecret); err != nil {
		logger.Error(err, "failed to clean up scan API token Secret resource")
		return err
	}
	scanApiDeployment := ScanApiDeployment(n.Mondoo.Namespace, "", *n.Mondoo, *n.MondooOperatorConfig, "", n.DeployOnOpenShift) // Image and private image scanning secret are not relevant when deleting.
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, scanApiDeployment); err != nil {
		logger.Error(err, "failed to clean up scan API Deployment resource")
		return err
	}

	scanApiService := ScanApiService(n.Mondoo.Namespace, *n.Mondoo)
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, scanApiService); err != nil {
		logger.Error(err, "failed to clean up scan API Service resource")
		return err
	}

	// Make sure to clear any degraded status
	updateScanAPIConditions(n.Mondoo, false, []appsv1.DeploymentCondition{}, &corev1.PodList{})

	return nil
}

func (n *DeploymentHandler) syncSecret(ctx context.Context) error {
	scanApiTokenSecret := ScanApiSecret(*n.Mondoo)
	if err := ctrl.SetControllerReference(n.Mondoo, scanApiTokenSecret, n.KubeClient.Scheme()); err != nil {
		return err
	}

	// Doing a direct Create() so that we don't have to do the Get()->IfNotExists->Create() dance
	// which lets us avoid asking for Get/List on Secrets across all Namespaces.
	err := n.KubeClient.Create(ctx, scanApiTokenSecret)
	if err == nil {
		logger.Info("Created token Secret for scan API")
		return nil
	} else if errors.IsAlreadyExists(err) {
		return nil
	} else {
		logger.Error(err, "Faled to create/check for existence of token Secret for scan API")
		return err
	}
}

func (n *DeploymentHandler) syncDeployment(ctx context.Context) error {
	cnspecImage, err := n.ContainerImageResolver.CnspecImage(n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		return err
	}
	logger.V(7).Info("Cnspec client image: ", "image", cnspecImage)
	logger.V(7).Info("Cnspec skip resolve: ", "SkipContainerResolution", n.MondooOperatorConfig.Spec.SkipContainerResolution)

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

	deployment := ScanApiDeployment(n.Mondoo.Namespace, cnspecImage, *n.Mondoo, *n.MondooOperatorConfig, privateRegistriesSecretName, n.DeployOnOpenShift)
	if err := ctrl.SetControllerReference(n.Mondoo, deployment, n.KubeClient.Scheme()); err != nil {
		return err
	}

	existingDeployment := appsv1.Deployment{}
	created, err := k8s.CreateIfNotExist(ctx, n.KubeClient, &existingDeployment, deployment)
	if err != nil {
		logger.Error(err, "Failed to create Deployment for scan API")
		return err
	}

	if created {
		logger.Info("Created Deployment for scan API")
		// set conditions on next iteration to not set to unavailable during initialisation
		return nil
	}

	// Get Pods for this deployment
	selector, _ := metav1.LabelSelectorAsSelector(existingDeployment.Spec.Selector)
	opts := &client.ListOptions{
		Namespace:     existingDeployment.Namespace,
		LabelSelector: client.MatchingLabelsSelector{Selector: selector},
	}
	pods := &corev1.PodList{}
	err = n.KubeClient.List(ctx, pods, opts)
	if err != nil {
		logger.Error(err, "Failed to list Pods for scan API")
		return err
	}

	updateScanAPIConditions(n.Mondoo, existingDeployment.Status.ReadyReplicas < *existingDeployment.Spec.Replicas, existingDeployment.Status.Conditions, pods)

	if !k8s.AreDeploymentsEqual(*deployment, existingDeployment) {
		logger.Info("Update needed for scan API Deployment")
		// If the deployment exists but it is different from what we actually want it to be, then update.
		k8s.UpdateDeployment(&existingDeployment, *deployment)
		if err := n.KubeClient.Update(ctx, &existingDeployment); err != nil {
			return err
		}
	}

	return nil
}

func (n *DeploymentHandler) syncService(ctx context.Context) error {
	service := ScanApiService(n.Mondoo.Namespace, *n.Mondoo)
	if err := ctrl.SetControllerReference(n.Mondoo, service, n.KubeClient.Scheme()); err != nil {
		return err
	}
	existingService := corev1.Service{}
	created, err := k8s.CreateIfNotExist(ctx, n.KubeClient, &existingService, service)
	if err != nil {
		logger.Error(err, "Failed to create Service for scan API")
		return err
	}

	if created {
		logger.Info("Created Service for scan API")
	} else if !k8s.AreServicesEqual(*service, existingService) {
		k8s.UpdateService(&existingService, *service)
		// If the service exists but it is different from what we actually want it to be, then update.
		if err := n.KubeClient.Update(ctx, &existingService); err != nil {
			return err
		}
	}
	return nil
}
