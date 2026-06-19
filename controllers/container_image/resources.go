// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package container_image

import (
	"fmt"
	"maps"
	"strings"

	// That's the mod k8s relies on https://github.com/kubernetes/kubernetes/blob/master/go.mod#L63

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/feature_flags"
	"go.mondoo.com/mondoo-operator/pkg/utils/gomemlimit"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	mondoo "go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mql/v13/providers-sdk/v1/inventory"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

const (
	// TODO: remove in next version
	OldCronJobNameSuffix = "-k8s-images-scan"

	CronJobNameSuffix      = "-containers-scan"
	InventoryConfigMapBase = "-containers-inventory"

	// Container image scanning loads tar headers into memory for every connected
	// image. The default maxConnections (50) can load all target images at once,
	// causing OOM. This limit keeps at most N image filesystems resident.
	// Not applied to k8s-resource or node scanning — those scan API objects or a
	// single local filesystem, not container images.
	defaultMaxProviderConnections = "10"
)

func CronJob(image, integrationMrn, clusterUid, privateRegistrySecretName string, m *v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) *batchv1.CronJob {
	ls := CronJobLabels(*m)

	cmd := []string{
		"cnspec", "scan", "k8s",
		"--config", "/etc/opt/mondoo/config/mondoo.yml",
		"--inventory-file", "/etc/opt/mondoo/config/inventory.yml",
		"--report-type", "none",
	}

	// Only add proxy settings if SkipProxyForCnspec is false
	// cnspec-based components may not properly handle NO_PROXY for internal domains
	if !cfg.Spec.SkipProxyForCnspec {
		if apiProxy := k8s.APIProxyURL(cfg); apiProxy != nil {
			cmd = append(cmd, "--api-proxy", *apiProxy)
		}
	}

	containerResources := k8s.ResourcesRequirementsWithDefaults(m.Spec.Containers.Resources, k8s.DefaultContainerScanningResources)
	gcLimit := gomemlimit.CalculateGoMemLimit(containerResources)

	envVars := feature_flags.AllFeatureFlagsAsEnv()
	envVars = append(envVars, corev1.EnvVar{Name: "MONDOO_AUTO_UPDATE", Value: "false"})
	envVars = append(envVars, corev1.EnvVar{Name: "MONDOO_TMP_DIR", Value: "/tmp"})
	envVars = append(envVars, corev1.EnvVar{Name: "GOMEMLIMIT", Value: gcLimit})
	envVars = append(envVars, corev1.EnvVar{Name: "MONDOO_MAX_PROVIDER_CONNECTIONS", Value: defaultMaxProviderConnections})

	// Add proxy environment variables from MondooOperatorConfig only if SkipProxyForCnspec is false
	if !cfg.Spec.SkipProxyForCnspec {
		envVars = append(envVars, k8s.ProxyEnvVars(cfg)...)
	}

	envVars = k8s.MergeEnv(envVars, m.Spec.Containers.Env)

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CronJobName(m.Name),
			Namespace: m.Namespace,
			Labels:    ls,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          m.Spec.Containers.Schedule,
			Suspend:           ptr.To(m.Spec.Containers.Suspend || m.Status.ScanningPaused),
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
									Name:            "mondoo-containers-scan",
									Command:         cmd,
									Resources:       containerResources,
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
									Env:                      envVars,
									TerminationMessagePath:   "/dev/termination-log",
									TerminationMessagePolicy: corev1.TerminationMessageReadFile,
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
														LocalObjectReference: k8s.ConfigSecretRef(*m),
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

	if d := m.Spec.Containers.ActiveDeadline; d != nil {
		seconds := int64(d.Seconds())
		cronjob.Spec.JobTemplate.Spec.ActiveDeadlineSeconds = &seconds
	}

	// Add WIF support for cloud registry authentication
	if wif := m.Spec.Containers.WorkloadIdentity; wif != nil {
		podSpec := &cronjob.Spec.JobTemplate.Spec.Template.Spec

		// Use WIF ServiceAccount
		podSpec.ServiceAccountName = WIFServiceAccountName(m.Name)
		podSpec.AutomountServiceAccountToken = ptr.To(true)

		// Add docker config volume (generated by init container)
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: "docker-config",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})

		// Mount docker config in main container
		podSpec.Containers[0].VolumeMounts = append(
			podSpec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "docker-config",
				ReadOnly:  true,
				MountPath: "/etc/opt/mondoo/docker",
			},
		)
		podSpec.Containers[0].Env = append(
			podSpec.Containers[0].Env,
			corev1.EnvVar{Name: "DOCKER_CONFIG", Value: "/etc/opt/mondoo/docker"},
		)

		// Add init container for registry credential generation
		podSpec.InitContainers = append(podSpec.InitContainers, k8s.RegistryWIFInitContainer(wif))

		// AKS Workload Identity webhook requires this label on the pod template only.
		// Copy labels so we don't mutate the CronJob/Job metadata.
		if wif.Provider == v1alpha2.CloudProviderAKS {
			podLabels := make(map[string]string, len(ls)+1)
			maps.Copy(podLabels, ls)
			podLabels["azure.workload.identity/use"] = "true"
			cronjob.Spec.JobTemplate.Spec.Template.Labels = podLabels
		}
	} else {
		// Add private registry secret if specified (static credentials path)
		k8s.AddPrivateRegistryPullSecretToSpec(&cronjob.Spec.JobTemplate.Spec.Template.Spec, privateRegistrySecretName)
	}

	// Append imagePullSecrets from MondooOperatorConfig (don't overwrite existing secrets)
	if len(cfg.Spec.ImagePullSecrets) > 0 {
		cronjob.Spec.JobTemplate.Spec.Template.Spec.ImagePullSecrets = append(
			cronjob.Spec.JobTemplate.Spec.Template.Spec.ImagePullSecrets,
			cfg.Spec.ImagePullSecrets...,
		)
	}

	return cronjob
}

func CronJobLabels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo-container-scan",
		"scan":      "k8s",
		"mondoo_cr": m.Name,
	}
}

// TODO: remove in next version
func OldCronJobName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, OldCronJobNameSuffix)
}

func CronJobName(prefix string) string {
	return k8s.CronJobName("container-scan", prefix)
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

func ConfigMapName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, InventoryConfigMapBase)
}

func Inventory(integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) (string, error) {
	inv := &inventory.Inventory{
		Metadata: &inventory.ObjectMeta{
			Name: "mondoo-k8s-containers-inventory",
		},
		Spec: &inventory.InventorySpec{
			Assets: []*inventory.Asset{
				{
					Connections: []*inventory.Config{
						{
							Type:    "k8s",
							Options: containerImageOptions(m),
							Discover: &inventory.Discovery{
								Targets: []string{"container-images"},
							},
						},
					},
					Labels: map[string]string{
						"k8s.mondoo.com/kind": "node",
					},
					ManagedBy: mondoo.ManagedByLabel(clusterUID),
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

	// Add user-defined annotations first, then operator-managed annotations.
	// Operator annotations go last so they cannot be overwritten by user values.
	if len(m.Spec.Annotations) > 0 {
		for i := range inv.Spec.Assets {
			inv.Spec.Assets[i].AddAnnotations(m.Spec.Annotations)
		}
	}
	for i := range inv.Spec.Assets {
		inv.Spec.Assets[i].AddAnnotations(constants.AuditConfigAnnotations(m.Name, m.Namespace))
	}

	invBytes, err := yaml.Marshal(inv)
	if err != nil {
		return "", err
	}

	return string(invBytes), nil
}

// WIFServiceAccountName returns the name for the container registry WIF ServiceAccount
func WIFServiceAccountName(prefix string) string {
	return fmt.Sprintf("%s-cr-wif", prefix)
}

// WIFServiceAccount creates a ServiceAccount with cloud-specific annotations for container registry WIF
func WIFServiceAccount(m *v1alpha2.MondooAuditConfig) *corev1.ServiceAccount {
	wif := m.Spec.Containers.WorkloadIdentity
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        WIFServiceAccountName(m.Name),
			Namespace:   m.Namespace,
			Labels:      CronJobLabels(*m),
			Annotations: make(map[string]string),
		},
	}

	switch wif.Provider {
	case v1alpha2.CloudProviderGKE:
		sa.Annotations["iam.gke.io/gcp-service-account"] = wif.GKE.GoogleServiceAccount
	case v1alpha2.CloudProviderEKS:
		sa.Annotations["eks.amazonaws.com/role-arn"] = wif.EKS.RoleARN
	case v1alpha2.CloudProviderAKS:
		sa.Annotations["azure.workload.identity/client-id"] = wif.AKS.ClientID
		if sa.Labels == nil {
			sa.Labels = make(map[string]string)
		}
		sa.Labels["azure.workload.identity/use"] = "true"
	}

	return sa
}

// validateContainerRegistryWIF validates the container registry WIF configuration
func validateContainerRegistryWIF(wif *v1alpha2.WorkloadIdentityConfig) error {
	if wif == nil {
		return nil
	}

	switch wif.Provider {
	case v1alpha2.CloudProviderGKE:
		if wif.GKE == nil {
			return fmt.Errorf("containers.workloadIdentity: gke config required when provider is gke")
		}
		if wif.GKE.GoogleServiceAccount == "" {
			return fmt.Errorf("containers.workloadIdentity: gke.googleServiceAccount is required for container registry WIF")
		}
	case v1alpha2.CloudProviderEKS:
		if wif.EKS == nil {
			return fmt.Errorf("containers.workloadIdentity: eks config required when provider is eks")
		}
		if wif.EKS.Region == "" {
			return fmt.Errorf("containers.workloadIdentity: eks.region is required for container registry WIF")
		}
	case v1alpha2.CloudProviderAKS:
		if wif.AKS == nil {
			return fmt.Errorf("containers.workloadIdentity: aks config required when provider is aks")
		}
		if wif.AKS.LoginServer == "" {
			return fmt.Errorf("containers.workloadIdentity: aks.loginServer is required for container registry WIF")
		}
	}

	return nil
}

func containerImageOptions(m v1alpha2.MondooAuditConfig) map[string]string {
	opts := map[string]string{
		"namespaces":         strings.Join(m.Spec.Filtering.Namespaces.Include, ","),
		"namespaces-exclude": strings.Join(m.Spec.Filtering.Namespaces.Exclude, ","),
		"disable-cache":      "false",
	}
	if len(m.Spec.Containers.Repositories.Include) > 0 {
		opts["images"] = strings.Join(m.Spec.Containers.Repositories.Include, ",")
	}
	if len(m.Spec.Containers.Repositories.Exclude) > 0 {
		opts["images-exclude"] = strings.Join(m.Spec.Containers.Repositories.Exclude, ",")
	}
	return opts
}
