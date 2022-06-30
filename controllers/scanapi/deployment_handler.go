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

package scanapi

import (
	"context"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var logger = ctrl.Log.WithName("scan-api")

type DeploymentHandler struct {
	KubeClient             client.Client
	Mondoo                 *v1alpha2.MondooAuditConfig
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
}

func (n *DeploymentHandler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	// If neither KubernetesResources, nor Admission is enabled, the scan API is not needed.
	if (!n.Mondoo.Spec.KubernetesResources.Enable && !n.Mondoo.Spec.Admission.Enable) ||
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
	scanApiDeployment := ScanApiDeployment(n.Mondoo.Namespace, "", *n.Mondoo) // Image is not relevant when deleting.
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
	updateScanAPIConditions(n.Mondoo, false, []appsv1.DeploymentCondition{})

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
		logger.Info("Token Secret for scan API already exists")
		return nil
	} else {
		logger.Error(err, "Faled to create/check for existence of token Secret for scan API")
		return err
	}
}

func (n *DeploymentHandler) syncDeployment(ctx context.Context) error {
	mondooClientImage, err := n.ContainerImageResolver.MondooClientImage("", "", n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		return err
	}

	deployment := ScanApiDeployment(n.Mondoo.Namespace, mondooClientImage, *n.Mondoo)
	if err := ctrl.SetControllerReference(n.Mondoo, deployment, n.KubeClient.Scheme()); err != nil {
		return err
	}

	if *deployment.Spec.Replicas < 2 && n.Mondoo.Spec.Admission.Mode == v1alpha2.Enforcing {
		logger.Info("WARNING: Scan API deployment is only scaled to 1 replica, but the admission mode is set to 'enforcing'. This might be problematic if the API server is not able to connect to the admission webhook. Please consider increasing the replicas.")
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
	} else if !k8s.AreDeploymentsEqual(*deployment, existingDeployment) {
		logger.Info("Update needed for scan API Deployment")
		// If the deployment exists but it is different from what we actually want it to be, then update.
		k8s.UpdateDeployment(&existingDeployment, *deployment)
		if err := n.KubeClient.Update(ctx, &existingDeployment); err != nil {
			return err
		}
	}

	updateScanAPIConditions(n.Mondoo, existingDeployment.Status.UnavailableReplicas != 0, existingDeployment.Status.Conditions)

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
