/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package container_image

import (
	"fmt"
	"strings"
	"time"

	"go.mondoo.com/cnquery/motor/asset"
	v1 "go.mondoo.com/cnquery/motor/inventory/v1"
	"go.mondoo.com/cnquery/motor/providers"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/feature_flags"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"gopkg.in/yaml.v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

const (
	// TODO: remove in next version
	OldCronJobNameSuffix = "-k8s-images-scan"

	CronJobNameSuffix      = "-containers-scan"
	InventoryConfigMapBase = "-containers-inventory"
)

func CronJob(image, integrationMrn, clusterUid, privateImageScanningSecretName string, m v1alpha2.MondooAuditConfig) *batchv1.CronJob {
	ls := CronJobLabels(m)

	cmd := []string{
		"cnspec", "scan", "k8s",
		"--config", "/etc/opt/mondoo/mondoo.yml",
		"--inventory-file", "/etc/opt/mondoo/inventory.yml",
		"--score-threshold", "0",
	}

	if m.Spec.HttpProxy != nil {
		cmd = append(cmd, []string{"--api-proxy", *m.Spec.HttpProxy}...)
	}

	// We want to start the cron job one minute after it was enabled.
	cronStart := time.Now().Add(1 * time.Minute)
	cronTab := fmt.Sprintf("%d %d * * *", cronStart.Minute(), cronStart.Hour())

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CronJobName(m.Name),
			Namespace: m.Namespace,
			Labels:    ls,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          cronTab,
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
										AllowPrivilegeEscalation: pointer.Bool(false),
										ReadOnlyRootFilesystem:   pointer.Bool(true),
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{
												"ALL",
											},
										},
										RunAsNonRoot: pointer.Bool(true),
										// This is needed to prevent:
										// Error: container has runAsNonRoot and image has non-numeric user (mondoo), cannot verify user is non-root ...
										RunAsUser:  pointer.Int64(101),
										Privileged: pointer.Bool(false),
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "config",
											ReadOnly:  true,
											MountPath: "/etc/opt/",
										},
										{
											Name:      "temp",
											MountPath: "/tmp",
										},
									},
									Env: feature_flags.AllFeatureFlagsAsEnv(),
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
											DefaultMode: pointer.Int32(corev1.ProjectedVolumeSourceDefaultMode),
											Sources: []corev1.VolumeProjection{
												{
													ConfigMap: &corev1.ConfigMapProjection{
														LocalObjectReference: corev1.LocalObjectReference{Name: ConfigMapName(m.Name)},
														Items: []corev1.KeyToPath{{
															Key:  "inventory",
															Path: "mondoo/inventory.yml",
														}},
													},
												},
												{
													Secret: &corev1.SecretProjection{
														LocalObjectReference: m.Spec.MondooCredsSecretRef,
														Items: []corev1.KeyToPath{{
															Key:  "config",
															Path: "mondoo/mondoo.yml",
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
			SuccessfulJobsHistoryLimit: pointer.Int32(1),
			FailedJobsHistoryLimit:     pointer.Int32(1),
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
					DefaultMode: pointer.Int32(0o440),
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

func ConfigMap(integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig) (*corev1.ConfigMap, error) {
	inv, err := Inventory(integrationMRN, clusterUID, m)
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

func Inventory(integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig) (string, error) {
	inv := &v1.Inventory{
		Metadata: &v1.ObjectMeta{
			Name: "mondoo-k8s-containers-inventory",
		},
		Spec: &v1.InventorySpec{
			Assets: []*asset.Asset{
				{
					Connections: []*providers.Config{
						{
							Backend: providers.ProviderType_K8S,
							Options: map[string]string{
								"namespaces":         strings.Join(m.Spec.Filtering.Namespaces.Include, ","),
								"namespaces-exclude": strings.Join(m.Spec.Filtering.Namespaces.Exclude, ","),
							},
							Discover: &providers.Discovery{
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

	invBytes, err := yaml.Marshal(inv)
	if err != nil {
		return "", err
	}

	return string(invBytes), nil
}
