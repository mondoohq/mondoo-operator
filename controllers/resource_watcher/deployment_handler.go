// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

var deploymentHandlerLogger = ctrl.Log.WithName("resource-watcher-handler")

// DeploymentHandler handles the reconciliation of the resource watcher deployment.
type DeploymentHandler struct {
	KubeClient             client.Client
	Mondoo                 *v1alpha2.MondooAuditConfig
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
}

// Reconcile ensures the resource watcher deployment matches the desired state.
func (h *DeploymentHandler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	// Resource watcher is only enabled if K8s resources scanning is enabled AND resource watcher is enabled
	if !h.Mondoo.Spec.KubernetesResources.Enable || !h.Mondoo.Spec.KubernetesResources.ResourceWatcher.Enable {
		return ctrl.Result{}, h.down(ctx)
	}

	if err := h.syncDeployment(ctx); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (h *DeploymentHandler) syncDeployment(ctx context.Context) error {
	mondooClientImage, err := h.ContainerImageResolver.MondooOperatorImage(
		ctx, h.Mondoo.Spec.Scanner.Image.Name, h.Mondoo.Spec.Scanner.Image.Tag, h.Mondoo.Spec.Scanner.Image.Digest, h.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		deploymentHandlerLogger.Error(err, "Failed to resolve mondoo-operator container image")
		return err
	}

	// Get cluster UID for asset labeling (best-effort)
	clusterUID, err := k8s.GetClusterUID(ctx, h.KubeClient, deploymentHandlerLogger)
	if err != nil {
		deploymentHandlerLogger.Info("Failed to get cluster UID, continuing without it", "error", err)
	}

	// Get integration MRN if available (best-effort)
	integrationMRN, err := k8s.TryGetIntegrationMrnForAuditConfig(ctx, h.KubeClient, *h.Mondoo)
	if err != nil {
		deploymentHandlerLogger.Info("Failed to get integration MRN, continuing without it", "error", err)
	}

	desired := Deployment(mondooClientImage, integrationMRN, clusterUID, h.Mondoo, *h.MondooOperatorConfig)
	if _, err := k8s.Apply(ctx, h.KubeClient, desired, h.Mondoo, deploymentHandlerLogger, k8s.DefaultApplyOptions()); err != nil {
		deploymentHandlerLogger.Error(err, "Failed to apply resource watcher Deployment", "namespace", desired.Namespace, "name", desired.Name)
		return err
	}

	// Get deployment status for condition updates
	deployments, err := h.getDeploymentsForAuditConfig(ctx)
	if err != nil {
		return err
	}

	// Get Pods for this Deployment
	pods := &corev1.PodList{}
	if len(deployments) > 0 {
		opts := &client.ListOptions{
			Namespace:     h.Mondoo.Namespace,
			LabelSelector: labels.SelectorFromSet(DeploymentLabels(*h.Mondoo)),
		}
		err = h.KubeClient.List(ctx, pods, opts)
		if err != nil {
			deploymentHandlerLogger.Error(err, "Failed to list Pods for Resource Watcher")
			return err
		}
	}

	updateResourceWatcherConditions(h.Mondoo, !areDeploymentsReady(deployments), pods)
	return nil
}

// areDeploymentsReady checks if all deployments have their desired replicas available.
func areDeploymentsReady(deployments []appsv1.Deployment) bool {
	for _, d := range deployments {
		if d.Status.AvailableReplicas < d.Status.Replicas {
			return false
		}
		// Also check for any unavailable replicas
		if d.Status.UnavailableReplicas > 0 {
			return false
		}
	}
	return true
}

func (h *DeploymentHandler) getDeploymentsForAuditConfig(ctx context.Context) ([]appsv1.Deployment, error) {
	deployments := &appsv1.DeploymentList{}
	deploymentLabels := DeploymentLabels(*h.Mondoo)

	listOpts := &client.ListOptions{Namespace: h.Mondoo.Namespace, LabelSelector: labels.SelectorFromSet(deploymentLabels)}
	if err := h.KubeClient.List(ctx, deployments, listOpts); err != nil {
		deploymentHandlerLogger.Error(err, "Failed to list Deployments in namespace", "namespace", h.Mondoo.Namespace)
		return nil, err
	}
	return deployments.Items, nil
}

func (h *DeploymentHandler) down(ctx context.Context) error {
	// Delete Deployment
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: DeploymentName(h.Mondoo.Name), Namespace: h.Mondoo.Namespace}}
	if err := k8s.DeleteIfExists(ctx, h.KubeClient, deployment); err != nil {
		deploymentHandlerLogger.Error(
			err, "failed to clean up resource watcher Deployment", "namespace", deployment.Namespace, "name", deployment.Name)
		return err
	}

	// Clear any remnant status
	updateResourceWatcherConditions(h.Mondoo, false, &corev1.PodList{})

	return nil
}
