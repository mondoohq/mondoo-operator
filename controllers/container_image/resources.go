// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package container_image

import (
	"fmt"
	"strings"

	// That's the mod k8s relies on https://github.com/kubernetes/kubernetes/blob/master/go.mod#L63

	"go.mondoo.com/cnquery/v11/providers-sdk/v1/inventory"
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
	// TODO: remove in next version
	OldCronJobNameSuffix = "-k8s-images-scan"

	CronJobNameSuffix      = "-containers-scan"
	InventoryConfigMapBase = "-containers-inventory"
)

func CronJob(image, integrationMrn, clusterUid, privateImageScanningSecretName string, m *v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) *batchv1.CronJob {
	ls := CronJobLabels(*m)

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
	envVars = append(envVars, corev1.EnvVar{Name: "MONDOO_DISABLE_PROVIDER_UPDATES", Value: "true"})
	envVars = k8s.MergeEnv(envVars, m.Spec.Containers.Env)

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CronJobName(m.Name),
			Namespace: m.Namespace,
			Labels:    ls,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          m.Spec.Containers.Schedule,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: ls},
				Spec: batchv1.JobSpec{
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
									Resources:       k8s.ResourcesRequirementsWithDefaults(m.Spec.Containers.Resources, k8s.DefaultContainerScanningResources),
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

	if privateImageScanningSecretName != "" {
		// mount secret needed to pull images from private registries
		cronjob.Spec.JobTemplate.Spec.Template.Spec.Volumes = append(cronjob.Spec.JobTemplate.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "pull-secrets",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: privateImageScanningSecretName,
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

		cronjob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts = append(cronjob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "pull-secrets",
			ReadOnly:  true,
			MountPath: "/etc/opt/mondoo/docker",
		})

		cronjob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env = append(cronjob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  "DOCKER_CONFIG",
			Value: "/etc/opt/mondoo/docker", // the client automatically adds '/config.json' to the path
		})
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
	return fmt.Sprintf("%s%s", prefix, CronJobNameSuffix)
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
							Type: "k8s",
							Options: map[string]string{
								"namespaces":         strings.Join(m.Spec.Filtering.Namespaces.Include, ","),
								"namespaces-exclude": strings.Join(m.Spec.Filtering.Namespaces.Exclude, ","),
							},
							Discover: &inventory.Discovery{
								Targets: []string{"container-images"},
							},
						},
					},
					Labels: map[string]string{
						"k8s.mondoo.com/kind": "node",
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
