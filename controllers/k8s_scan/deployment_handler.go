// Copyright Mondoo, Inc. 2026
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
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

var logger = ctrl.Log.WithName("k8s-resources-scanning")

const (
	workloadDeploymentConfigMapNameTemplate = `%s-deploy`
	WorkloadDeploymentNameTemplate          = `%s-workload`
)

// validateExternalClusterAuth validates that exactly one authentication method is specified
func validateExternalClusterAuth(cluster v1alpha2.ExternalCluster) error {
	authMethods := 0
	if cluster.KubeconfigSecretRef != nil {
		authMethods++
	}
	if cluster.ServiceAccountAuth != nil {
		authMethods++
	}
	if cluster.WorkloadIdentity != nil {
		authMethods++
	}
	if cluster.SPIFFEAuth != nil {
		authMethods++
	}

	if authMethods == 0 {
		return fmt.Errorf("externalCluster %q: must specify one of kubeconfigSecretRef, serviceAccountAuth, workloadIdentity, or spiffeAuth", cluster.Name)
	}
	if authMethods > 1 {
		return fmt.Errorf("externalCluster %q: kubeconfigSecretRef, serviceAccountAuth, workloadIdentity, and spiffeAuth are mutually exclusive", cluster.Name)
	}

	// Provider-specific validation for WorkloadIdentity
	if cluster.WorkloadIdentity != nil {
		switch cluster.WorkloadIdentity.Provider {
		case v1alpha2.CloudProviderGKE:
			if cluster.WorkloadIdentity.GKE == nil {
				return fmt.Errorf("externalCluster %q: gke config required when provider is gke", cluster.Name)
			}
		case v1alpha2.CloudProviderEKS:
			if cluster.WorkloadIdentity.EKS == nil {
				return fmt.Errorf("externalCluster %q: eks config required when provider is eks", cluster.Name)
			}
		case v1alpha2.CloudProviderAKS:
			if cluster.WorkloadIdentity.AKS == nil {
				return fmt.Errorf("externalCluster %q: aks config required when provider is aks", cluster.Name)
			}
		}
	}

	// Validation for ServiceAccountAuth
	if cluster.ServiceAccountAuth != nil {
		if cluster.ServiceAccountAuth.Server == "" {
			return fmt.Errorf("externalCluster %q: server is required for serviceAccountAuth", cluster.Name)
		}
		if cluster.ServiceAccountAuth.CredentialsSecretRef.Name == "" {
			return fmt.Errorf("externalCluster %q: credentialsSecretRef.name is required for serviceAccountAuth", cluster.Name)
		}
	}

	// Validation for KubeconfigSecretRef
	if cluster.KubeconfigSecretRef != nil {
		if cluster.KubeconfigSecretRef.Name == "" {
			return fmt.Errorf("externalCluster %q: kubeconfigSecretRef.name is required", cluster.Name)
		}
	}

	// Validation for SPIFFEAuth
	if cluster.SPIFFEAuth != nil {
		if cluster.SPIFFEAuth.Server == "" {
			return fmt.Errorf("externalCluster %q: server is required for spiffeAuth", cluster.Name)
		}
		if cluster.SPIFFEAuth.TrustBundleSecretRef.Name == "" {
			return fmt.Errorf("externalCluster %q: trustBundleSecretRef.name is required for spiffeAuth", cluster.Name)
		}
	}
	return nil
}

type DeploymentHandler struct {
	KubeClient             client.Client
	Mondoo                 *v1alpha2.MondooAuditConfig
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
}

func (n *DeploymentHandler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	hasExternalClusters := len(n.Mondoo.Spec.KubernetesResources.ExternalClusters) > 0

	if !n.Mondoo.Spec.KubernetesResources.Enable {
		// Clean up local cluster resources only
		if err := n.downLocalCluster(ctx); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// Sync local cluster CronJob
		if err := n.syncCronJob(ctx); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile external cluster CronJobs if any are configured, otherwise clean up orphaned resources
	if hasExternalClusters {
		if err := n.reconcileExternalClusters(ctx); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// No external clusters configured - clean up any orphaned external resources
		if err := n.cleanupOrphanedExternalClusterResources(ctx, make(map[string]bool)); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (n *DeploymentHandler) syncCronJob(ctx context.Context) error {
	mondooOperatorImage, err := n.ContainerImageResolver.MondooOperatorImage(
		ctx, n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.Mondoo.Spec.Scanner.Image.Digest, n.MondooOperatorConfig.Spec.SkipContainerResolution)
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

	if err := n.syncConfigMap(ctx, integrationMrn, clusterUid); err != nil {
		return err
	}

	desired := CronJob(mondooOperatorImage, integrationMrn, clusterUid, n.Mondoo, *n.MondooOperatorConfig)
	op, err := k8s.Apply(ctx, n.KubeClient, desired, n.Mondoo, logger, k8s.DefaultApplyOptions())
	if err != nil {
		logger.Error(err, "Failed to apply CronJob", "namespace", desired.Namespace, "name", desired.Name)
		return err
	}

	// When a CronJob is updated, remove old Jobs so they don't continue running with stale config
	if op == k8s.ApplyUpdated {
		if err := n.KubeClient.DeleteAllOf(ctx, &batchv1.Job{},
			client.InNamespace(n.Mondoo.Namespace),
			client.MatchingLabels(CronJobLabels(*n.Mondoo)),
			client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
			logger.Error(err, "Failed to clean up old Jobs after CronJob update")
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
			logger.Error(err, "Failed to list Pods for Kubernetes Resource Scanning")
			return err
		}
	}

	updateWorkloadsConditions(n.Mondoo, !k8s.AreCronJobsSuccessful(cronJobs), pods)
	return n.cleanupWorkloadDeployment(ctx)
}

// syncConfigMap syncs the inventory ConfigMap using Server-Side Apply.
func (n *DeploymentHandler) syncConfigMap(ctx context.Context, integrationMrn, clusterUid string) error {
	desired, err := ConfigMap(integrationMrn, clusterUid, *n.Mondoo, *n.MondooOperatorConfig)
	if err != nil {
		logger.Error(err, "failed to generate desired ConfigMap with inventory")
		return err
	}

	if _, err := k8s.Apply(ctx, n.KubeClient, desired, n.Mondoo, logger, k8s.DefaultApplyOptions()); err != nil {
		logger.Error(err, "Failed to apply inventory ConfigMap", "namespace", desired.Namespace, "name", desired.Name)
		return err
	}

	return nil
}

// reconcileExternalClusters reconciles CronJobs for external clusters
func (n *DeploymentHandler) reconcileExternalClusters(ctx context.Context) error {
	mondooClientImage, err := n.ContainerImageResolver.CnspecImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.Mondoo.Spec.Scanner.Image.Digest, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "Failed to resolve cnspec container image")
		return err
	}

	integrationMrn, err := k8s.TryGetIntegrationMrnForAuditConfig(ctx, n.KubeClient, *n.Mondoo)
	if err != nil {
		logger.Error(err, "failed to retrieve integration-mrn for MondooAuditConfig")
		return err
	}

	clusterUid, err := k8s.GetClusterUID(ctx, n.KubeClient, logger)
	if err != nil {
		logger.Error(err, "Failed to get cluster's UID")
		return err
	}

	// Track which external cluster names are configured
	configuredClusters := make(map[string]bool)

	for _, cluster := range n.Mondoo.Spec.KubernetesResources.ExternalClusters {
		configuredClusters[cluster.Name] = true

		// Validate authentication configuration
		if err := validateExternalClusterAuth(cluster); err != nil {
			logger.Error(err, "invalid external cluster authentication configuration", "cluster", cluster.Name)
			return err
		}

		// Sync SA kubeconfig ConfigMap if using ServiceAccountAuth
		if cluster.ServiceAccountAuth != nil {
			if err := n.syncExternalClusterSAKubeconfigConfigMap(ctx, cluster); err != nil {
				return err
			}
		}

		// Sync WIF ServiceAccount if using WorkloadIdentity
		if cluster.WorkloadIdentity != nil {
			if err := n.syncWIFServiceAccount(ctx, cluster); err != nil {
				return err
			}
		}

		// Sync ConfigMap for this external cluster
		if err := n.syncExternalClusterConfigMap(ctx, integrationMrn, clusterUid, cluster); err != nil {
			return err
		}

		// Sync CronJob for this external cluster
		if err := n.syncExternalClusterCronJob(ctx, mondooClientImage, cluster); err != nil {
			return err
		}
	}

	// Clean up CronJobs and ConfigMaps for external clusters that are no longer configured
	if err := n.cleanupOrphanedExternalClusterResources(ctx, configuredClusters); err != nil {
		return err
	}

	return nil
}

func (n *DeploymentHandler) syncExternalClusterConfigMap(ctx context.Context, integrationMrn, clusterUid string, cluster v1alpha2.ExternalCluster) error {
	desired, err := ExternalClusterConfigMap(integrationMrn, clusterUid, cluster, *n.Mondoo, *n.MondooOperatorConfig)
	if err != nil {
		logger.Error(err, "failed to generate desired ConfigMap for external cluster", "cluster", cluster.Name)
		return err
	}

	if _, err := k8s.Apply(ctx, n.KubeClient, desired, n.Mondoo, logger, k8s.DefaultApplyOptions()); err != nil {
		logger.Error(err, "Failed to apply inventory ConfigMap for external cluster", "cluster", cluster.Name)
		return err
	}

	return nil
}

func (n *DeploymentHandler) syncExternalClusterCronJob(ctx context.Context, image string, cluster v1alpha2.ExternalCluster) error {
	desired := ExternalClusterCronJob(image, cluster, n.Mondoo, *n.MondooOperatorConfig)

	op, err := k8s.Apply(ctx, n.KubeClient, desired, n.Mondoo, logger, k8s.DefaultApplyOptions())
	if err != nil {
		logger.Error(err, "Failed to apply CronJob for external cluster", "cluster", cluster.Name)
		return err
	}

	// When a CronJob is updated, remove old Jobs so they don't continue running with stale config
	if op == k8s.ApplyUpdated {
		if err := n.KubeClient.DeleteAllOf(ctx, &batchv1.Job{},
			client.InNamespace(n.Mondoo.Namespace),
			client.MatchingLabels(ExternalClusterCronJobLabels(*n.Mondoo, cluster.Name)),
			client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
			logger.Error(err, "Failed to clean up old Jobs after CronJob update for external cluster", "cluster", cluster.Name)
			return err
		}
	}

	return nil
}

func (n *DeploymentHandler) cleanupOrphanedExternalClusterResources(ctx context.Context, configuredClusters map[string]bool) error {
	// List all CronJobs with our labels
	cronJobs := &batchv1.CronJobList{}
	listOpts := &client.ListOptions{
		Namespace: n.Mondoo.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app":       "mondoo-k8s-scan",
			"mondoo_cr": n.Mondoo.Name,
		}),
	}
	if err := n.KubeClient.List(ctx, cronJobs, listOpts); err != nil {
		return err
	}

	for _, cj := range cronJobs.Items {
		clusterName, hasClusterLabel := cj.Labels["cluster_name"]
		if !hasClusterLabel {
			// This is the main cluster CronJob, not an external cluster
			continue
		}

		if !configuredClusters[clusterName] {
			// This external cluster is no longer configured, delete its resources
			if err := k8s.DeleteIfExists(ctx, n.KubeClient, &cj); err != nil {
				logger.Error(err, "failed to delete orphaned CronJob for external cluster", "cluster", clusterName)
				return err
			}

			// Delete inventory ConfigMap
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ExternalClusterConfigMapName(n.Mondoo.Name, clusterName),
					Namespace: n.Mondoo.Namespace,
				},
			}
			if err := k8s.DeleteIfExists(ctx, n.KubeClient, configMap); err != nil {
				logger.Error(err, "failed to delete orphaned ConfigMap for external cluster", "cluster", clusterName)
				return err
			}

			// Delete SA kubeconfig ConfigMap (if exists)
			saKubeconfigCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ExternalClusterSAKubeconfigName(n.Mondoo.Name, clusterName),
					Namespace: n.Mondoo.Namespace,
				},
			}
			if err := k8s.DeleteIfExists(ctx, n.KubeClient, saKubeconfigCM); err != nil {
				logger.Error(err, "failed to delete orphaned SA kubeconfig ConfigMap for external cluster", "cluster", clusterName)
				return err
			}

			// Delete WIF ServiceAccount (if exists)
			wifSA := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      WIFServiceAccountName(n.Mondoo.Name, clusterName),
					Namespace: n.Mondoo.Namespace,
				},
			}
			if err := k8s.DeleteIfExists(ctx, n.KubeClient, wifSA); err != nil {
				logger.Error(err, "failed to delete orphaned WIF ServiceAccount for external cluster", "cluster", clusterName)
				return err
			}

			logger.Info("Cleaned up orphaned resources for external cluster", "cluster", clusterName)
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

// downLocalCluster cleans up only the local cluster scanning resources
func (n *DeploymentHandler) downLocalCluster(ctx context.Context) error {
	// Delete main cluster CronJob
	cronJob := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: CronJobName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace}}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, cronJob); err != nil {
		logger.Error(err, "failed to clean up Kubernetes resource scanning CronJob", "namespace", cronJob.Namespace, "name", cronJob.Name)
		return err
	}

	// Delete main cluster ConfigMap
	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace}}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, configMap); err != nil {
		logger.Error(err, "failed to clean up Kubernetes resource scanning ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
		return err
	}

	if err := n.cleanupWorkloadDeployment(ctx); err != nil {
		return err
	}

	// Clear local cluster status
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

// syncExternalClusterSAKubeconfigConfigMap syncs a ConfigMap containing the generated kubeconfig for ServiceAccountAuth
func (n *DeploymentHandler) syncExternalClusterSAKubeconfigConfigMap(ctx context.Context, cluster v1alpha2.ExternalCluster) error {
	desired := ExternalClusterSAKubeconfigConfigMap(cluster, n.Mondoo)

	if _, err := k8s.Apply(ctx, n.KubeClient, desired, n.Mondoo, logger, k8s.DefaultApplyOptions()); err != nil {
		logger.Error(err, "Failed to apply SA kubeconfig ConfigMap for external cluster", "cluster", cluster.Name)
		return err
	}

	return nil
}

// syncWIFServiceAccount syncs a ServiceAccount with cloud-specific annotations for Workload Identity Federation
func (n *DeploymentHandler) syncWIFServiceAccount(ctx context.Context, cluster v1alpha2.ExternalCluster) error {
	desired := WIFServiceAccount(cluster, n.Mondoo)

	if _, err := k8s.Apply(ctx, n.KubeClient, desired, n.Mondoo, logger, k8s.DefaultApplyOptions()); err != nil {
		logger.Error(err, "Failed to apply WIF ServiceAccount for external cluster", "cluster", cluster.Name)
		return err
	}

	return nil
}
