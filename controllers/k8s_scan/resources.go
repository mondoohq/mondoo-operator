// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"fmt"
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

	if cfg.Spec.HttpProxy != nil {
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
									Env: envVars,
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

	if cfg.Spec.HttpProxy != nil {
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
							RestartPolicy: corev1.RestartPolicyNever,
							// No need for k8s service account token since we're using kubeconfig
							AutomountServiceAccountToken: ptr.To(false),
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
									VolumeMounts: []corev1.VolumeMount{
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
									},
									Env: envVars,
								},
							},
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
								{
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
								},
							},
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
	targets := K8sDiscoveryTargets
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
