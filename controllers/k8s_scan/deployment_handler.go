// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"context"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/constants"
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
	if cluster.VaultAuth != nil {
		authMethods++
	}

	if authMethods == 0 {
		return fmt.Errorf("externalCluster %q: must specify one of kubeconfigSecretRef, serviceAccountAuth, workloadIdentity, spiffeAuth, or vaultAuth", cluster.Name)
	}
	if authMethods > 1 {
		return fmt.Errorf("externalCluster %q: kubeconfigSecretRef, serviceAccountAuth, workloadIdentity, spiffeAuth, and vaultAuth are mutually exclusive", cluster.Name)
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

	// Validation for VaultAuth
	if cluster.VaultAuth != nil {
		if cluster.VaultAuth.Server == "" {
			return fmt.Errorf("externalCluster %q: server is required for vaultAuth", cluster.Name)
		}
		if cluster.VaultAuth.VaultAddr == "" {
			return fmt.Errorf("externalCluster %q: vaultAddr is required for vaultAuth", cluster.Name)
		}
		if cluster.VaultAuth.AuthRole == "" {
			return fmt.Errorf("externalCluster %q: authRole is required for vaultAuth", cluster.Name)
		}
		if cluster.VaultAuth.CredsRole == "" {
			return fmt.Errorf("externalCluster %q: credsRole is required for vaultAuth", cluster.Name)
		}
		if cluster.VaultAuth.CACertSecretRef != nil && cluster.VaultAuth.CACertSecretRef.Name == "" {
			return fmt.Errorf("externalCluster %q: caCertSecretRef.name is required when caCertSecretRef is specified for vaultAuth", cluster.Name)
		}
		if cluster.VaultAuth.TargetCACertSecretRef != nil && cluster.VaultAuth.TargetCACertSecretRef.Name == "" {
			return fmt.Errorf("externalCluster %q: targetCACertSecretRef.name is required when targetCACertSecretRef is specified for vaultAuth", cluster.Name)
		}
	}
	return nil
}

const defaultSATokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token" //nolint:gosec

type DeploymentHandler struct {
	KubeClient             client.Client
	Mondoo                 *v1alpha2.MondooAuditConfig
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
	MondooClientBuilder    func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error)
	VaultTokenFetcher      VaultTokenFetcher
	// SATokenPath is the path to the operator pod's service account token.
	// Defaults to /var/run/secrets/kubernetes.io/serviceaccount/token.
	SATokenPath string
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

	// Perform garbage collection of stale K8s resource scan assets if a new successful scan has completed
	if n.Mondoo.Spec.KubernetesResources.Enable || hasExternalClusters {
		clusterUid, err := k8s.GetClusterUID(ctx, n.KubeClient, logger)
		if err != nil {
			logger.Error(err, "Failed to get cluster's UID for garbage collection")
		} else {
			n.garbageCollectIfNeeded(ctx, clusterUid)
		}
	}

	return ctrl.Result{}, nil
}

func (n *DeploymentHandler) syncCronJob(ctx context.Context) error {
	cnspecImage, err := n.ContainerImageResolver.CnspecImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.Mondoo.Spec.Scanner.Image.Digest, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "Failed to resolve cnspec container image")
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

	desired := CronJob(cnspecImage, n.Mondoo, *n.MondooOperatorConfig)
	obj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	op, err := k8s.CreateOrUpdate(ctx, n.KubeClient, obj, n.Mondoo, logger, func() error {
		k8s.UpdateCronJobFields(obj, desired)
		return nil
	})
	if err != nil {
		return err
	}

	// When a CronJob is updated, remove completed Jobs so they don't linger with stale config
	if op == controllerutil.OperationResultUpdated {
		if err := k8s.DeleteCompletedJobs(ctx, n.KubeClient, n.Mondoo.Namespace, CronJobLabels(*n.Mondoo), logger); err != nil {
			logger.Error(err, "Failed to clean up completed Jobs after CronJob update")
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

func (n *DeploymentHandler) syncConfigMap(ctx context.Context, integrationMrn, clusterUid string) error {
	desired, err := ConfigMap(integrationMrn, clusterUid, *n.Mondoo, *n.MondooOperatorConfig)
	if err != nil {
		logger.Error(err, "failed to generate desired ConfigMap with inventory")
		return err
	}

	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	if _, err := k8s.CreateOrUpdate(ctx, n.KubeClient, obj, n.Mondoo, logger, func() error {
		obj.Labels = desired.Labels
		obj.Data = desired.Data
		return nil
	}); err != nil {
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

		// Sync Vault kubeconfig Secret if using VaultAuth
		if cluster.VaultAuth != nil {
			if err := n.syncVaultKubeconfigSecret(ctx, cluster); err != nil {
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

	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	if _, err := k8s.CreateOrUpdate(ctx, n.KubeClient, obj, n.Mondoo, logger, func() error {
		obj.Labels = desired.Labels
		obj.Data = desired.Data
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (n *DeploymentHandler) syncExternalClusterCronJob(ctx context.Context, image string, cluster v1alpha2.ExternalCluster) error {
	desired := ExternalClusterCronJob(image, cluster, n.Mondoo, *n.MondooOperatorConfig)

	obj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	op, err := k8s.CreateOrUpdate(ctx, n.KubeClient, obj, n.Mondoo, logger, func() error {
		k8s.UpdateCronJobFields(obj, desired)
		return nil
	})
	if err != nil {
		return err
	}

	// When a CronJob is updated, remove completed Jobs so they don't linger with stale config
	if op == controllerutil.OperationResultUpdated {
		if err := k8s.DeleteCompletedJobs(ctx, n.KubeClient, n.Mondoo.Namespace, ExternalClusterCronJobLabels(*n.Mondoo, cluster.Name), logger); err != nil {
			logger.Error(err, "Failed to clean up completed Jobs after CronJob update for external cluster", "cluster", cluster.Name)
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

			// Delete Vault kubeconfig Secret (if exists)
			vaultKubeconfigSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      VaultKubeconfigSecretName(n.Mondoo.Name, clusterName),
					Namespace: n.Mondoo.Namespace,
				},
			}
			if err := k8s.DeleteIfExists(ctx, n.KubeClient, vaultKubeconfigSecret); err != nil {
				logger.Error(err, "failed to delete orphaned Vault kubeconfig Secret for external cluster", "cluster", clusterName)
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

	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	if _, err := k8s.CreateOrUpdate(ctx, n.KubeClient, obj, n.Mondoo, logger, func() error {
		obj.Labels = desired.Labels
		obj.Data = desired.Data
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// garbageCollectIfNeeded checks whether a new successful K8s scan has completed since the last GC run,
// and if so, performs garbage collection of stale assets via the Mondoo API.
func (n *DeploymentHandler) garbageCollectIfNeeded(ctx context.Context, clusterUid string) {
	// List all k8s-scan CronJobs (local + external) for this audit config
	cronJobs := &batchv1.CronJobList{}
	listOpts := &client.ListOptions{
		Namespace: n.Mondoo.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app":       "mondoo-k8s-scan",
			"mondoo_cr": n.Mondoo.Name,
		}),
	}
	if err := n.KubeClient.List(ctx, cronJobs, listOpts); err != nil {
		logger.Error(err, "Failed to list CronJobs for garbage collection")
		return
	}

	// Find the latest lastSuccessfulTime across all CronJobs
	var latestSuccess *metav1.Time
	for i := range cronJobs.Items {
		t := cronJobs.Items[i].Status.LastSuccessfulTime
		if t != nil && (latestSuccess == nil || t.After(latestSuccess.Time)) {
			latestSuccess = t
		}
	}

	if latestSuccess == nil {
		// No successful scans yet
		return
	}

	// Skip if we already ran GC for this (or a newer) successful scan
	if n.Mondoo.Status.LastK8sResourceGarbageCollectionTime != nil &&
		!latestSuccess.After(n.Mondoo.Status.LastK8sResourceGarbageCollectionTime.Time) {
		return
	}

	managedBy := "mondoo-operator-" + clusterUid
	if err := n.performGarbageCollection(ctx, managedBy); err != nil {
		logger.Error(err, "Failed to perform garbage collection of K8s resource scan assets")
	}

	// Always update the timestamp so we don't retry until the next new successful scan.
	// GC failure is non-critical — stale assets will be cleaned up on the next attempt.
	now := metav1.Now()
	n.Mondoo.Status.LastK8sResourceGarbageCollectionTime = &now
}

// performGarbageCollection calls the Mondoo API to garbage collect stale K8s resource scan assets.
func (n *DeploymentHandler) performGarbageCollection(ctx context.Context, managedBy string) error {
	if n.MondooClientBuilder == nil {
		logger.Info("MondooClientBuilder not configured, skipping garbage collection")
		return nil
	}

	// Read service account credentials from the creds secret
	credsSecret := &corev1.Secret{}
	credsSecretKey := client.ObjectKey{
		Namespace: n.Mondoo.Namespace,
		Name:      n.Mondoo.Spec.MondooCredsSecretRef.Name,
	}
	if err := n.KubeClient.Get(ctx, credsSecretKey, credsSecret); err != nil {
		return fmt.Errorf("failed to get credentials secret: %w", err)
	}

	saData, ok := credsSecret.Data[constants.MondooCredsSecretServiceAccountKey]
	if !ok {
		return fmt.Errorf("credentials secret missing key %q", constants.MondooCredsSecretServiceAccountKey)
	}

	sa, err := mondoo.LoadServiceAccountFromFile(saData)
	if err != nil {
		return fmt.Errorf("failed to load service account: %w", err)
	}

	token, err := mondoo.GenerateTokenFromServiceAccount(*sa, logger)
	if err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}

	opts := mondooclient.MondooClientOptions{
		ApiEndpoint: sa.ApiEndpoint,
		Token:       token,
	}
	if n.MondooOperatorConfig != nil {
		opts.HttpProxy = n.MondooOperatorConfig.Spec.HttpProxy
		opts.HttpsProxy = n.MondooOperatorConfig.Spec.HttpsProxy
		opts.NoProxy = n.MondooOperatorConfig.Spec.NoProxy
	}

	mondooClient, err := n.MondooClientBuilder(opts)
	if err != nil {
		return fmt.Errorf("failed to create mondoo client: %w", err)
	}

	gcOpts := &mondooclient.GarbageCollectOptions{
		ManagedBy:       managedBy,
		PlatformRuntime: "k8s-cluster",
		OlderThan:       time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
	}

	if err := mondooClient.GarbageCollectAssets(ctx, gcOpts); err != nil {
		return fmt.Errorf("garbage collection API call failed: %w", err)
	}

	logger.Info("Successfully performed garbage collection of K8s resource scan assets")
	return nil
}

// syncVaultKubeconfigSecret fetches credentials from Vault and writes a kubeconfig Secret.
func (n *DeploymentHandler) syncVaultKubeconfigSecret(ctx context.Context, cluster v1alpha2.ExternalCluster) error {
	if n.VaultTokenFetcher == nil {
		return fmt.Errorf("VaultTokenFetcher not configured")
	}

	// Read the operator pod's service account token
	tokenPath := n.SATokenPath
	if tokenPath == "" {
		tokenPath = defaultSATokenPath
	}
	saToken, err := os.ReadFile(tokenPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to read service account token: %w", err)
	}

	// Read Vault CA cert if configured
	var vaultCACert []byte
	if cluster.VaultAuth.CACertSecretRef != nil {
		caSecret := &corev1.Secret{}
		caKey := client.ObjectKey{
			Namespace: n.Mondoo.Namespace,
			Name:      cluster.VaultAuth.CACertSecretRef.Name,
		}
		if err := n.KubeClient.Get(ctx, caKey, caSecret); err != nil {
			return fmt.Errorf("failed to get Vault CA cert secret: %w", err)
		}
		var ok bool
		vaultCACert, ok = caSecret.Data["ca.crt"]
		if !ok || len(vaultCACert) == 0 {
			return fmt.Errorf("vault CA cert secret %q is missing the \"ca.crt\" key", cluster.VaultAuth.CACertSecretRef.Name)
		}
	}

	// Read target cluster CA cert if configured
	var targetCACert []byte
	if cluster.VaultAuth.TargetCACertSecretRef != nil {
		targetCASecret := &corev1.Secret{}
		targetCAKey := client.ObjectKey{
			Namespace: n.Mondoo.Namespace,
			Name:      cluster.VaultAuth.TargetCACertSecretRef.Name,
		}
		if err := n.KubeClient.Get(ctx, targetCAKey, targetCASecret); err != nil {
			return fmt.Errorf("failed to get target CA cert secret: %w", err)
		}
		var ok bool
		targetCACert, ok = targetCASecret.Data["ca.crt"]
		if !ok || len(targetCACert) == 0 {
			return fmt.Errorf("target CA cert secret %q is missing the \"ca.crt\" key", cluster.VaultAuth.TargetCACertSecretRef.Name)
		}
	}

	// Fetch token from Vault
	token, err := n.VaultTokenFetcher(ctx, string(saToken), *cluster.VaultAuth, vaultCACert)
	if err != nil {
		return fmt.Errorf("failed to fetch Vault token for cluster %s: %w", cluster.Name, err)
	}

	// Build kubeconfig
	kubeconfig := buildVaultKubeconfig(cluster.VaultAuth.Server, token, targetCACert)

	// Create/update kubeconfig Secret
	desired := VaultKubeconfigSecret(n.Mondoo, cluster.Name, kubeconfig)
	obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	if _, err := k8s.CreateOrUpdate(ctx, n.KubeClient, obj, n.Mondoo, logger, func() error {
		obj.Labels = desired.Labels
		obj.Data = desired.Data
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// syncWIFServiceAccount syncs a ServiceAccount with cloud-specific annotations for Workload Identity Federation
func (n *DeploymentHandler) syncWIFServiceAccount(ctx context.Context, cluster v1alpha2.ExternalCluster) error {
	desired := WIFServiceAccount(cluster, n.Mondoo)

	obj := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	if _, err := k8s.CreateOrUpdate(ctx, n.KubeClient, obj, n.Mondoo, logger, func() error {
		obj.Labels = desired.Labels
		obj.Annotations = desired.Annotations
		return nil
	}); err != nil {
		return err
	}

	return nil
}
