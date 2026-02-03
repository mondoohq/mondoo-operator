// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"fmt"
	"path/filepath"
	"strings"

	// That's the mod k8s relies on https://github.com/kubernetes/kubernetes/blob/master/go.mod#L63

	"go.mondoo.com/cnquery/v12/providers-sdk/v1/inventory"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/feature_flags"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"gopkg.in/yaml.v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	CronJobNameSuffix      = "-k8s-scan"
	InventoryConfigMapBase = "-k8s-inventory"
)

// K8sDiscoveryTargets defines explicit targets for K8s resource scanning
// (excludes container-images which is handled by the separate containers controller)
var K8sDiscoveryTargets = []string{
	"clusters",
	"pods",
	"jobs",
	"cronjobs",
	"statefulsets",
	"deployments",
	"replicasets",
	"daemonsets",
	"ingresses",
	"namespaces",
	"services",
}

const (
	// GarbageCollectOlderThan is the default duration for garbage collection of stale assets
	GarbageCollectOlderThan = "2h"
)

func CronJob(image, integrationMrn, clusterUid string, m *v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) *batchv1.CronJob {
	ls := CronJobLabels(*m)

	managedBy := "mondoo-operator-" + clusterUid
	cmd := []string{
		"/mondoo-operator", "k8s-scan",
		"--config", "/etc/opt/mondoo/config/mondoo.yml",
		"--inventory-file", "/etc/opt/mondoo/config/inventory.yml",
		"--cleanup-assets-older-than", GarbageCollectOlderThan,
		"--set-managed-by", managedBy,
	}

	// Only add proxy if configured and not skipped for cnspec
	if cfg.Spec.HttpProxy != nil && !cfg.Spec.SkipProxyForCnspec {
		cmd = append(cmd, []string{"--api-proxy", *cfg.Spec.HttpProxy}...)
	}

	envVars := feature_flags.AllFeatureFlagsAsEnv()
	envVars = append(envVars, corev1.EnvVar{Name: "MONDOO_AUTO_UPDATE", Value: "false"})

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CronJobName(m.Name),
			Namespace: m.Namespace,
			Labels:    ls,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          m.Spec.KubernetesResources.Schedule,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: ls},
				Spec: batchv1.JobSpec{
					// Don't retry failed scans - re-running won't fix the issue
					BackoffLimit: ptr.To(int32(0)),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: ls},
						Spec: corev1.PodSpec{
							// The scan can fail when an asset has an error. However, re-scanning won't result in the error
							// being fixed. Therefore, we don't want to restart the job.
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Image:           image,
									ImagePullPolicy: corev1.PullIfNotPresent,
									Name:            "mondoo-k8s-scan",
									Command:         cmd,
									Resources:       k8s.ResourcesRequirementsWithDefaults(m.Spec.Scanner.Resources, k8s.DefaultK8sResourceScanningResources),
									SecurityContext: &corev1.SecurityContext{
										AllowPrivilegeEscalation: ptr.To(false),
										ReadOnlyRootFilesystem:   ptr.To(true),
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{
												"ALL",
											},
										},
										RunAsNonRoot: ptr.To(true),
										// This is needed to prevent:
										// Error: container has runAsNonRoot and image has non-numeric user (mondoo), cannot verify user is non-root ...
										RunAsUser:  ptr.To(int64(101)),
										Privileged: ptr.To(false),
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "config",
											ReadOnly:  true,
											MountPath: "/etc/opt/mondoo/config",
										},
										{
											Name:      "temp",
											MountPath: "/tmp",
										},
									},
									// Env: envVars,
									Env: buildEnvVars(cfg),
								},
							},
							ServiceAccountName: m.Spec.Scanner.ServiceAccountName,
							Volumes: []corev1.Volume{
								{
									Name: "temp",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: "config",
									VolumeSource: corev1.VolumeSource{
										Projected: &corev1.ProjectedVolumeSource{
											DefaultMode: ptr.To(int32(corev1.ProjectedVolumeSourceDefaultMode)),
											Sources: []corev1.VolumeProjection{
												{
													ConfigMap: &corev1.ConfigMapProjection{
														LocalObjectReference: corev1.LocalObjectReference{Name: ConfigMapName(m.Name)},
														Items: []corev1.KeyToPath{{
															Key:  "inventory",
															Path: "inventory.yml",
														}},
													},
												},
												{
													Secret: &corev1.SecretProjection{
														LocalObjectReference: m.Spec.MondooCredsSecretRef,
														Items: []corev1.KeyToPath{{
															Key:  "config",
															Path: "mondoo.yml",
														}},
													},
												},
											},
										},
									},
								},
							},
							ImagePullSecrets: cfg.Spec.ImagePullSecrets,
						},
					},
				},
			},
			SuccessfulJobsHistoryLimit: ptr.To(int32(1)),
			FailedJobsHistoryLimit:     ptr.To(int32(1)),
		},
	}

	return cronjob
}

// ExternalClusterCronJob creates a CronJob for scanning a remote K8s cluster
func ExternalClusterCronJob(image string, cluster v1alpha2.ExternalCluster, m *v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) *batchv1.CronJob {
	ls := ExternalClusterCronJobLabels(*m, cluster.Name)

	cmd := []string{
		"cnspec", "scan", "k8s",
		"--config", "/etc/opt/mondoo/config/mondoo.yml",
		"--inventory-file", "/etc/opt/mondoo/config/inventory.yml",
		"--score-threshold", "0",
	}

	// Only add proxy if configured and not skipped for cnspec
	if cfg.Spec.HttpProxy != nil && !cfg.Spec.SkipProxyForCnspec {
		cmd = append(cmd, []string{"--api-proxy", *cfg.Spec.HttpProxy}...)
	}

	envVars := feature_flags.AllFeatureFlagsAsEnv()
	envVars = append(envVars, corev1.EnvVar{Name: "MONDOO_AUTO_UPDATE", Value: "false"})
	// Point KUBECONFIG to the mounted kubeconfig file
	envVars = append(envVars, corev1.EnvVar{Name: "KUBECONFIG", Value: "/etc/opt/mondoo/kubeconfig/kubeconfig"})

	schedule := cluster.Schedule
	if schedule == "" {
		schedule = m.Spec.KubernetesResources.Schedule
	}

	// Base volumes and mounts
	volumes := []corev1.Volume{
		{
			Name: "temp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					DefaultMode: ptr.To(int32(corev1.ProjectedVolumeSourceDefaultMode)),
					Sources: []corev1.VolumeProjection{
						{
							ConfigMap: &corev1.ConfigMapProjection{
								LocalObjectReference: corev1.LocalObjectReference{Name: ExternalClusterConfigMapName(m.Name, cluster.Name)},
								Items: []corev1.KeyToPath{{
									Key:  "inventory",
									Path: "inventory.yml",
								}},
							},
						},
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: m.Spec.MondooCredsSecretRef,
								Items: []corev1.KeyToPath{{
									Key:  "config",
									Path: "mondoo.yml",
								}},
							},
						},
					},
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "config",
			ReadOnly:  true,
			MountPath: "/etc/opt/mondoo/config",
		},
		{
			Name:      "kubeconfig",
			ReadOnly:  true,
			MountPath: "/etc/opt/mondoo/kubeconfig",
		},
		{
			Name:      "temp",
			MountPath: "/tmp",
		},
	}

	var initContainers []corev1.Container
	serviceAccountName := ""
	autoMountServiceAccountToken := ptr.To(false)

	// Configure authentication method
	switch {
	case cluster.KubeconfigSecretRef != nil:
		// Kubeconfig auth: mount the kubeconfig secret directly
		volumes = append(volumes, corev1.Volume{
			Name: "kubeconfig",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  cluster.KubeconfigSecretRef.Name,
					DefaultMode: ptr.To(int32(0o440)),
					Items: []corev1.KeyToPath{{
						Key:  "kubeconfig",
						Path: "kubeconfig",
					}},
				},
			},
		})

	case cluster.ServiceAccountAuth != nil:
		// SA auth: mount credentials secret + generated kubeconfig ConfigMap
		volumes = append(volumes,
			corev1.Volume{
				Name: "sa-credentials",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  cluster.ServiceAccountAuth.CredentialsSecretRef.Name,
						DefaultMode: ptr.To(int32(0o440)),
					},
				},
			},
			corev1.Volume{
				Name: "kubeconfig",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: ExternalClusterSAKubeconfigName(m.Name, cluster.Name),
						},
					},
				},
			},
		)
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "sa-credentials",
			ReadOnly:  true,
			MountPath: "/etc/opt/mondoo/sa-credentials",
		})

	case cluster.WorkloadIdentity != nil:
		// WIF auth: use WIF ServiceAccount, init container to generate kubeconfig
		serviceAccountName = WIFServiceAccountName(m.Name, cluster.Name)
		autoMountServiceAccountToken = ptr.To(true)

		// Use emptyDir for kubeconfig since it's generated at runtime
		volumes = append(volumes, corev1.Volume{
			Name: "kubeconfig",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

		// Update kubeconfig volume mount to be writable for init container
		for i := range volumeMounts {
			if volumeMounts[i].Name == "kubeconfig" {
				volumeMounts[i].ReadOnly = false
			}
		}

		initContainers = append(initContainers, wifInitContainer(cluster))

	case cluster.SPIFFEAuth != nil:
		// SPIFFE auth: use sidecar to fetch certificates, generate kubeconfig
		serviceAccountName = m.Spec.Scanner.ServiceAccountName
		autoMountServiceAccountToken = ptr.To(true)

		socketPath := cluster.SPIFFEAuth.SocketPath
		if socketPath == "" {
			socketPath = "/run/spire/sockets/agent.sock"
		}

		// Mount SPIRE agent socket from host
		volumes = append(volumes, corev1.Volume{
			Name: "spire-agent-socket",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: filepath.Dir(socketPath),
					Type: ptr.To(corev1.HostPathDirectory),
				},
			},
		})

		// Mount trust bundle for remote cluster CA
		volumes = append(volumes, corev1.Volume{
			Name: "trust-bundle",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  cluster.SPIFFEAuth.TrustBundleSecretRef.Name,
					DefaultMode: ptr.To(int32(0o440)),
				},
			},
		})

		// EmptyDir for generated certificates
		volumes = append(volumes, corev1.Volume{
			Name: "spiffe-certs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: corev1.StorageMediumMemory,
				},
			},
		})

		// EmptyDir for generated kubeconfig
		volumes = append(volumes, corev1.Volume{
			Name: "kubeconfig",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

		// Add volume mounts for SPIFFE certs and trust bundle to main container
		volumeMounts = append(volumeMounts,
			corev1.VolumeMount{Name: "spiffe-certs", MountPath: "/etc/spiffe-certs", ReadOnly: true},
			corev1.VolumeMount{Name: "trust-bundle", MountPath: "/etc/trust-bundle", ReadOnly: true},
		)

		// Update kubeconfig mount to be writable for init container
		for i := range volumeMounts {
			if volumeMounts[i].Name == "kubeconfig" {
				volumeMounts[i].ReadOnly = false
			}
		}

		initContainers = append(initContainers, spiffeInitContainer(cluster))
	}

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ExternalClusterCronJobName(m.Name, cluster.Name),
			Namespace: m.Namespace,
			Labels:    ls,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          schedule,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: ls},
				Spec: batchv1.JobSpec{
					// Don't retry failed scans - re-running won't fix the issue
					BackoffLimit: ptr.To(int32(0)),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: ls},
						Spec: corev1.PodSpec{
							RestartPolicy:                corev1.RestartPolicyNever,
							AutomountServiceAccountToken: autoMountServiceAccountToken,
							ServiceAccountName:           serviceAccountName,
							InitContainers:               initContainers,
							Containers: []corev1.Container{
								{
									Image:           image,
									ImagePullPolicy: corev1.PullIfNotPresent,
									Name:            "mondoo-k8s-scan",
									Command:         cmd,
									Resources:       k8s.ResourcesRequirementsWithDefaults(m.Spec.Scanner.Resources, k8s.DefaultK8sResourceScanningResources),
									SecurityContext: &corev1.SecurityContext{
										AllowPrivilegeEscalation: ptr.To(false),
										ReadOnlyRootFilesystem:   ptr.To(true),
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{
												"ALL",
											},
										},
										RunAsNonRoot: ptr.To(true),
										RunAsUser:    ptr.To(int64(101)),
										Privileged:   ptr.To(false),
									},
									VolumeMounts: volumeMounts,
									Env:          envVars,
								},
							},
							Volumes: volumes,
						},
					},
				},
			},
			SuccessfulJobsHistoryLimit: ptr.To(int32(1)),
			FailedJobsHistoryLimit:     ptr.To(int32(1)),
		},
	}

	// Add private registry pull secrets if configured
	if cluster.PrivateRegistriesPullSecretRef != nil && cluster.PrivateRegistriesPullSecretRef.Name != "" {
		cronjob.Spec.JobTemplate.Spec.Template.Spec.Volumes = append(cronjob.Spec.JobTemplate.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "pull-secrets",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cluster.PrivateRegistriesPullSecretRef.Name,
								},
								Items: []corev1.KeyToPath{
									{
										Key:  ".dockerconfigjson",
										Path: "config.json",
									},
								},
							},
						},
					},
					DefaultMode: ptr.To(int32(0o440)),
				},
			},
		})

		cronjob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts = append(
			cronjob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "pull-secrets",
				ReadOnly:  true,
				MountPath: "/etc/opt/mondoo/docker",
			},
		)

		cronjob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env = append(
			cronjob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "DOCKER_CONFIG",
				Value: "/etc/opt/mondoo/docker",
			},
		)
	}

	return cronjob
}

func CronJobLabels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo-k8s-scan",
		"scan":      "k8s",
		"mondoo_cr": m.Name,
	}
}

func ExternalClusterCronJobLabels(m v1alpha2.MondooAuditConfig, clusterName string) map[string]string {
	return map[string]string{
		"app":          "mondoo-k8s-scan",
		"scan":         "k8s",
		"mondoo_cr":    m.Name,
		"cluster_name": clusterName,
	}
}

func CronJobName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, CronJobNameSuffix)
}

func ExternalClusterCronJobName(prefix, clusterName string) string {
	return fmt.Sprintf("%s%s-%s", prefix, CronJobNameSuffix, clusterName)
}

func ConfigMapName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, InventoryConfigMapBase)
}

func ExternalClusterConfigMapName(prefix, clusterName string) string {
	return fmt.Sprintf("%s%s-%s", prefix, InventoryConfigMapBase, clusterName)
}

// ExternalClusterSAKubeconfigName returns the name for the SA kubeconfig ConfigMap
func ExternalClusterSAKubeconfigName(prefix, clusterName string) string {
	return fmt.Sprintf("%s-sa-kubeconfig-%s", prefix, clusterName)
}

// WIFServiceAccountName returns the name for the WIF ServiceAccount
func WIFServiceAccountName(prefix, clusterName string) string {
	return fmt.Sprintf("%s-wif-%s", prefix, clusterName)
}

// ExternalClusterSAKubeconfig generates a kubeconfig that references mounted token and CA files
func ExternalClusterSAKubeconfig(cluster v1alpha2.ExternalCluster) string {
	var clusterConfig string
	if cluster.ServiceAccountAuth.SkipTLSVerify {
		clusterConfig = fmt.Sprintf(`    insecure-skip-tls-verify: true
    server: %s`, cluster.ServiceAccountAuth.Server)
	} else {
		clusterConfig = fmt.Sprintf(`    certificate-authority: /etc/opt/mondoo/sa-credentials/ca.crt
    server: %s`, cluster.ServiceAccountAuth.Server)
	}

	return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
%s
  name: external
contexts:
- context:
    cluster: external
    user: scanner
  name: default
current-context: default
users:
- name: scanner
  user:
    tokenFile: /etc/opt/mondoo/sa-credentials/token
`, clusterConfig)
}

// ExternalClusterSAKubeconfigConfigMap creates a ConfigMap containing the generated kubeconfig for SA auth
func ExternalClusterSAKubeconfigConfigMap(cluster v1alpha2.ExternalCluster, m *v1alpha2.MondooAuditConfig) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: m.Namespace,
			Name:      ExternalClusterSAKubeconfigName(m.Name, cluster.Name),
			Labels:    ExternalClusterCronJobLabels(*m, cluster.Name),
		},
		Data: map[string]string{
			"kubeconfig": ExternalClusterSAKubeconfig(cluster),
		},
	}
}

// WIFServiceAccount creates a ServiceAccount with cloud-specific annotations for Workload Identity Federation
func WIFServiceAccount(cluster v1alpha2.ExternalCluster, m *v1alpha2.MondooAuditConfig) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        WIFServiceAccountName(m.Name, cluster.Name),
			Namespace:   m.Namespace,
			Labels:      ExternalClusterCronJobLabels(*m, cluster.Name),
			Annotations: make(map[string]string),
		},
	}

	switch cluster.WorkloadIdentity.Provider {
	case v1alpha2.CloudProviderGKE:
		sa.Annotations["iam.gke.io/gcp-service-account"] = cluster.WorkloadIdentity.GKE.GoogleServiceAccount
	case v1alpha2.CloudProviderEKS:
		sa.Annotations["eks.amazonaws.com/role-arn"] = cluster.WorkloadIdentity.EKS.RoleARN
	case v1alpha2.CloudProviderAKS:
		sa.Annotations["azure.workload.identity/client-id"] = cluster.WorkloadIdentity.AKS.ClientID
		if sa.Labels == nil {
			sa.Labels = make(map[string]string)
		}
		sa.Labels["azure.workload.identity/use"] = "true"
	}

	return sa
}

// Container image versions for init containers (pinned for reproducibility)
const (
	// Google Cloud SDK image - slim variant for smaller size
	// https://cloud.google.com/sdk/docs/downloads-docker
	GCloudSDKImage = "gcr.io/google.com/cloudsdktool/google-cloud-cli:499.0.0-slim"

	// AWS CLI image
	// https://hub.docker.com/r/amazon/aws-cli
	AWSCLIImage = "amazon/aws-cli:2.22.0"

	// Azure CLI image
	// https://mcr.microsoft.com/en-us/artifact/mar/azure-cli/tags
	AzureCLIImage = "mcr.microsoft.com/azure-cli:2.67.0"

	// SPIFFE Helper image
	// https://github.com/spiffe/spiffe-helper/releases
	SPIFFEHelperImage = "ghcr.io/spiffe/spiffe-helper:0.8.0"
)

// wifInitContainer creates an init container that generates kubeconfig using cloud CLI tools
func wifInitContainer(cluster v1alpha2.ExternalCluster) corev1.Container {
	var image, script string
	var env []corev1.EnvVar

	// Common retry wrapper for transient failures
	retryWrapper := `
# Retry wrapper for transient failures
retry() {
  local max_attempts=3
  local delay=5
  local attempt=1
  while [ $attempt -le $max_attempts ]; do
    if "$@"; then
      return 0
    fi
    echo "Attempt $attempt failed, retrying in ${delay}s..."
    sleep $delay
    attempt=$((attempt + 1))
  done
  echo "All $max_attempts attempts failed"
  return 1
}
`

	switch cluster.WorkloadIdentity.Provider {
	case v1alpha2.CloudProviderGKE:
		image = GCloudSDKImage
		script = retryWrapper + `
retry gcloud container clusters get-credentials "$CLUSTER_NAME" \
  --project "$PROJECT_ID" \
  --location "$CLUSTER_LOCATION"
cp ~/.kube/config /etc/opt/mondoo/kubeconfig/kubeconfig
`
		env = []corev1.EnvVar{
			{Name: "HOME", Value: "/tmp"},
			{Name: "CLUSTER_NAME", Value: cluster.WorkloadIdentity.GKE.ClusterName},
			{Name: "PROJECT_ID", Value: cluster.WorkloadIdentity.GKE.ProjectID},
			{Name: "CLUSTER_LOCATION", Value: cluster.WorkloadIdentity.GKE.ClusterLocation},
		}

	case v1alpha2.CloudProviderEKS:
		image = AWSCLIImage
		script = retryWrapper + `
retry aws eks update-kubeconfig \
  --name "$CLUSTER_NAME" \
  --region "$AWS_REGION" \
  --kubeconfig /etc/opt/mondoo/kubeconfig/kubeconfig
`
		env = []corev1.EnvVar{
			{Name: "HOME", Value: "/tmp"},
			{Name: "CLUSTER_NAME", Value: cluster.WorkloadIdentity.EKS.ClusterName},
			{Name: "AWS_REGION", Value: cluster.WorkloadIdentity.EKS.Region},
		}

	case v1alpha2.CloudProviderAKS:
		image = AzureCLIImage
		script = retryWrapper + `
retry az aks get-credentials \
  --resource-group "$RESOURCE_GROUP" \
  --name "$CLUSTER_NAME" \
  --subscription "$SUBSCRIPTION_ID" \
  --file /etc/opt/mondoo/kubeconfig/kubeconfig
`
		env = []corev1.EnvVar{
			{Name: "HOME", Value: "/tmp"},
			{Name: "CLUSTER_NAME", Value: cluster.WorkloadIdentity.AKS.ClusterName},
			{Name: "RESOURCE_GROUP", Value: cluster.WorkloadIdentity.AKS.ResourceGroup},
			{Name: "SUBSCRIPTION_ID", Value: cluster.WorkloadIdentity.AKS.SubscriptionID},
		}

	default:
		// This should never happen if validation is correct, but handle gracefully
		image = "busybox:1.36"
		script = `echo "ERROR: Unknown workload identity provider"; exit 1`
		env = []corev1.EnvVar{}
	}

	return corev1.Container{
		Name:    "generate-kubeconfig",
		Image:   image,
		Command: []string{"/bin/sh", "-c", script},
		Env:     env,
		VolumeMounts: []corev1.VolumeMount{
			{Name: "kubeconfig", MountPath: "/etc/opt/mondoo/kubeconfig"},
			{Name: "temp", MountPath: "/tmp"},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			ReadOnlyRootFilesystem:   ptr.To(true),
			RunAsNonRoot:             ptr.To(true),
			// needed to prevent errors for the google CLI container which runs as root by default
			RunAsUser: ptr.To(int64(101)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}
}

// spiffeInitContainer creates an init container that fetches SPIFFE certificates
// and generates a kubeconfig for the remote cluster.
//
// Note: This implementation fetches certificates once during init and does not
// rotate them during the scan. SPIFFE SVIDs typically have a 1-hour TTL by default.
// For most K8s resource scans that complete within minutes, this is sufficient.
// If scans consistently exceed your SVID TTL, consider:
// - Increasing the SVID TTL in your SPIRE server configuration
// - Using a different authentication method (kubeconfig, service account token)
func spiffeInitContainer(cluster v1alpha2.ExternalCluster) corev1.Container {
	socketPath := cluster.SPIFFEAuth.SocketPath
	if socketPath == "" {
		socketPath = "/run/spire/sockets/agent.sock"
	}
	socketFile := filepath.Base(socketPath)

	// Use a static script that references environment variables for safety
	script := `#!/bin/sh
set -e

# Wait for SPIRE agent socket (timeout after 60 seconds)
echo "Waiting for SPIRE agent socket..."
SOCKET_WAIT=0
while [ ! -S "/spire-agent-socket/${SOCKET_FILE}" ]; do
    sleep 1
    SOCKET_WAIT=$((SOCKET_WAIT + 1))
    if [ $SOCKET_WAIT -ge 60 ]; then
        echo "ERROR: SPIRE agent socket not available within timeout"
        exit 1
    fi
done

# Fetch SVID using spiffe-helper
# The spiffe-helper writes certs to the specified directory
cat > /tmp/spiffe-helper.conf << CONF
agent_address = "/spire-agent-socket/${SOCKET_FILE}"
cmd = ""
cert_dir = "/etc/spiffe-certs"
svid_file_name = "svid.pem"
svid_key_file_name = "svid_key.pem"
svid_bundle_file_name = "svid_bundle.pem"
CONF

/usr/bin/spiffe-helper -config /tmp/spiffe-helper.conf &
HELPER_PID=$!

# Wait for certificates to be written
echo "Waiting for SPIFFE certificates..."
for i in $(seq 1 60); do
    if [ -f /etc/spiffe-certs/svid.pem ] && [ -f /etc/spiffe-certs/svid_key.pem ]; then
        break
    fi
    sleep 1
done

if [ ! -f /etc/spiffe-certs/svid.pem ]; then
    echo "ERROR: SPIFFE certificates not generated within timeout"
    exit 1
fi

# Generate kubeconfig using client certificates
cat > /etc/opt/mondoo/kubeconfig/kubeconfig << EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority: /etc/trust-bundle/ca.crt
    server: ${K8S_SERVER}
  name: external
contexts:
- context:
    cluster: external
    user: spiffe
  name: default
current-context: default
users:
- name: spiffe
  user:
    client-certificate: /etc/spiffe-certs/svid.pem
    client-key: /etc/spiffe-certs/svid_key.pem
EOF

echo "Kubeconfig generated successfully"

# Kill spiffe-helper (certs are already fetched)
kill $HELPER_PID 2>/dev/null || true
`

	return corev1.Container{
		Name:    "fetch-spiffe-certs",
		Image:   SPIFFEHelperImage,
		Command: []string{"/bin/sh", "-c", script},
		Env: []corev1.EnvVar{
			{Name: "SOCKET_FILE", Value: socketFile},
			{Name: "K8S_SERVER", Value: cluster.SPIFFEAuth.Server},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "spire-agent-socket", MountPath: "/spire-agent-socket"},
			{Name: "spiffe-certs", MountPath: "/etc/spiffe-certs"},
			{Name: "trust-bundle", MountPath: "/etc/trust-bundle", ReadOnly: true},
			{Name: "kubeconfig", MountPath: "/etc/opt/mondoo/kubeconfig"},
			{Name: "temp", MountPath: "/tmp"},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			ReadOnlyRootFilesystem:   ptr.To(true),
			RunAsNonRoot:             ptr.To(true),
			RunAsUser:                ptr.To(int64(101)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}
}

func ConfigMap(integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) (*corev1.ConfigMap, error) {
	inv, err := Inventory(integrationMRN, clusterUID, m, cfg)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: m.Namespace,
			Name:      ConfigMapName(m.Name),
		},
		Data: map[string]string{"inventory": inv},
	}, nil
}

func ExternalClusterConfigMap(integrationMRN, operatorClusterUID string, cluster v1alpha2.ExternalCluster, m v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) (*corev1.ConfigMap, error) {
	inv, err := ExternalClusterInventory(integrationMRN, operatorClusterUID, cluster, cfg)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: m.Namespace,
			Name:      ExternalClusterConfigMapName(m.Name, cluster.Name),
			Labels:    ExternalClusterCronJobLabels(m, cluster.Name),
		},
		Data: map[string]string{"inventory": inv},
	}, nil
}

func Inventory(integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) (string, error) {
	inv := &inventory.Inventory{
		Metadata: &inventory.ObjectMeta{
			Name: "mondoo-k8s-resources-inventory",
		},
		Spec: &inventory.InventorySpec{
			Assets: []*inventory.Asset{
				{
					Connections: []*inventory.Config{
						{
							Type: "k8s",
							Options: map[string]string{
								"namespaces":         strings.Join(m.Spec.Filtering.Namespaces.Include, ","),
								"namespaces-exclude": strings.Join(m.Spec.Filtering.Namespaces.Exclude, ","),
							},
							Discover: &inventory.Discovery{
								Targets: K8sDiscoveryTargets,
							},
						},
					},
					Labels: map[string]string{
						"k8s.mondoo.com/kind": "cluster",
					},
					ManagedBy: "mondoo-operator-" + clusterUID,
				},
			},
		},
	}

	if integrationMRN != "" {
		for i := range inv.Spec.Assets {
			inv.Spec.Assets[i].Labels[constants.MondooAssetsIntegrationLabel] = integrationMRN
		}
	}

	if cfg.Spec.ContainerProxy != nil {
		for i := range inv.Spec.Assets {
			inv.Spec.Assets[i].Connections[0].Options["container-proxy"] = *cfg.Spec.ContainerProxy
		}
	}

	invBytes, err := yaml.Marshal(inv)
	if err != nil {
		return "", err
	}

	return string(invBytes), nil
}

func ExternalClusterInventory(integrationMRN, operatorClusterUID string, cluster v1alpha2.ExternalCluster, cfg v1alpha2.MondooOperatorConfig) (string, error) {
	// Use cluster-specific filtering if provided, otherwise fall back to empty filtering
	filtering := cluster.Filtering

	// Determine discovery targets based on whether container image scanning is enabled
	// Make a copy to avoid mutating the shared slice
	targets := make([]string, len(K8sDiscoveryTargets))
	copy(targets, K8sDiscoveryTargets)
	if cluster.ContainerImageScanning {
		targets = append(targets, "container-images")
	}

	inv := &inventory.Inventory{
		Metadata: &inventory.ObjectMeta{
			Name: fmt.Sprintf("mondoo-k8s-external-%s-inventory", cluster.Name),
		},
		Spec: &inventory.InventorySpec{
			Assets: []*inventory.Asset{
				{
					Connections: []*inventory.Config{
						{
							Type: "k8s",
							Options: map[string]string{
								"namespaces":         strings.Join(filtering.Namespaces.Include, ","),
								"namespaces-exclude": strings.Join(filtering.Namespaces.Exclude, ","),
							},
							Discover: &inventory.Discovery{
								Targets: targets,
							},
						},
					},
					Labels: map[string]string{
						"k8s.mondoo.com/kind":      "cluster",
						"mondoo.com/cluster-name":  cluster.Name,
						"mondoo.com/external-scan": "true",
					},
					ManagedBy: "mondoo-operator-" + operatorClusterUID,
				},
			},
		},
	}

	if integrationMRN != "" {
		for i := range inv.Spec.Assets {
			inv.Spec.Assets[i].Labels[constants.MondooAssetsIntegrationLabel] = integrationMRN
		}
	}

	if cfg.Spec.ContainerProxy != nil {
		for i := range inv.Spec.Assets {
			inv.Spec.Assets[i].Connections[0].Options["container-proxy"] = *cfg.Spec.ContainerProxy
		}
	}

	invBytes, err := yaml.Marshal(inv)
	if err != nil {
		return "", err
	}

	return string(invBytes), nil
}


func buildEnvVars(cfg v1alpha2.MondooOperatorConfig) []corev1.EnvVar {
	envVars := feature_flags.AllFeatureFlagsAsEnv()

	// Add proxy environment variables from MondooOperatorConfig
	if cfg.Spec.HttpProxy != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "HTTP_PROXY", Value: *cfg.Spec.HttpProxy})
		envVars = append(envVars, corev1.EnvVar{Name: "http_proxy", Value: *cfg.Spec.HttpProxy})
	}
	if cfg.Spec.HttpsProxy != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "HTTPS_PROXY", Value: *cfg.Spec.HttpsProxy})
		envVars = append(envVars, corev1.EnvVar{Name: "https_proxy", Value: *cfg.Spec.HttpsProxy})
	}
	if cfg.Spec.NoProxy != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "NO_PROXY", Value: *cfg.Spec.NoProxy})
		envVars = append(envVars, corev1.EnvVar{Name: "no_proxy", Value: *cfg.Spec.NoProxy})
	}

	return envVars
}