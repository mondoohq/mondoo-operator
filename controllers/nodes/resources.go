// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"crypto/sha256"
	"fmt"
	"math"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"go.mondoo.com/cnquery/providers-sdk/v1/inventory"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/feature_flags"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

const (
	CronJobNameBase               = "-node-"
	GarbageCollectCronJobNameBase = "-node-gc"
	InventoryConfigMapBase        = "-node-inventory-"

	ignoreQueryAnnotationPrefix = "policies.k8s.mondoo.com/"

	ignoreAnnotationValue = "ignore"
)

func CronJob(image string, node corev1.Node, m v1alpha2.MondooAuditConfig, isOpenshift bool, cfg v1alpha2.MondooOperatorConfig) *batchv1.CronJob {
	ls := CronJobLabels(m)

	cronTab := fmt.Sprintf("%d * * * *", time.Now().Add(1*time.Minute).Minute())
	unsetHostPath := corev1.HostPathUnset

	name := "cnspec"
	cmd := []string{
		"cnspec", "scan", "local",
		"--config", "/etc/opt/mondoo/mondoo.yml",
		"--inventory-file", "/etc/opt/mondoo/inventory.yml",
		"--score-threshold", "0",
	}

	if cfg.Spec.HttpProxy != nil {
		cmd = append(cmd, []string{"--api-proxy", *cfg.Spec.HttpProxy}...)
	}

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				ignoreQueryAnnotationPrefix + "mondoo-kubernetes-security-cronjob-runasnonroot": ignoreAnnotationValue,
			},
			Name:      CronJobName(m.Name, node.Name),
			Namespace: m.Namespace,
			Labels:    CronJobLabels(m),
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          cronTab,
			ConcurrencyPolicy: batchv1.AllowConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ignoreQueryAnnotationPrefix + "mondoo-kubernetes-security-job-runasnonroot": ignoreAnnotationValue,
					},
					Labels: ls,
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								ignoreQueryAnnotationPrefix + "mondoo-kubernetes-security-pod-runasnonroot": ignoreAnnotationValue,
							},
							Labels: ls,
						},
						Spec: corev1.PodSpec{
							NodeName:      node.Name,
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Tolerations:   k8s.TaintsToTolerations(node.Spec.Taints),
							// The node scanning does not use the Kubernetes API at all, therefore the service account token
							// should not be mounted at all.
							AutomountServiceAccountToken: ptr.To(false),
							Containers: []corev1.Container{
								{
									Image:     image,
									Name:      name,
									Command:   cmd,
									Resources: k8s.ResourcesRequirementsWithDefaults(m.Spec.Nodes.Resources, k8s.DefaultNodeScanningResources),
									SecurityContext: &corev1.SecurityContext{
										AllowPrivilegeEscalation: ptr.To(isOpenshift),
										ReadOnlyRootFilesystem:   ptr.To(true),
										RunAsNonRoot:             ptr.To(false),
										RunAsUser:                ptr.To(int64(0)),
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{
												"ALL",
											},
										},
										// RHCOS requires to run as privileged to properly do node scanning. If the container
										// is not privileged, then we have no access to /proc.
										Privileged: ptr.To(isOpenshift),
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "root",
											ReadOnly:  true,
											MountPath: "/mnt/host/",
										},
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
									Env: []corev1.EnvVar{
										{
											Name:  "DEBUG",
											Value: "false",
										},
										{
											Name:  "MONDOO_PROCFS",
											Value: "on",
										},
										{
											Name:  "MONDOO_AUTO_UPDATE",
											Value: "false",
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "root",
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{Path: "/", Type: &unsetHostPath},
									},
								},
								{
									Name: "config",
									VolumeSource: corev1.VolumeSource{
										Projected: &corev1.ProjectedVolumeSource{
											DefaultMode: ptr.To(corev1.ProjectedVolumeSourceDefaultMode),
											Sources: []corev1.VolumeProjection{
												{
													ConfigMap: &corev1.ConfigMapProjection{
														LocalObjectReference: corev1.LocalObjectReference{Name: ConfigMapName(m.Name, node.Name)},
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
								{
									Name: "temp",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{},
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
}

func GarbageCollectCronJob(image, clusterUid string, m v1alpha2.MondooAuditConfig) *batchv1.CronJob {
	ls := CronJobLabels(m)

	cronTab := fmt.Sprintf("%d */2 * * *", time.Now().Add(1*time.Minute).Minute())
	scanApiUrl := scanapi.ScanApiServiceUrl(m)
	containerArgs := []string{
		"garbage-collect",
		"--scan-api-url", scanApiUrl,
		"--token-file-path", "/etc/scanapi/token",

		// The job runs hourly and we need to make sure that the previous one is killed before the new one is started so we don't stack them.
		"--timeout", "55",
		// Cleanup any resources more than 2 hours old
		"--filter-older-than", "2h",
		"--labels", "k8s.mondoo.com/kind=node",
	}

	if clusterUid != "" {
		scannedAssetsManagedBy := "mondoo-operator-" + clusterUid
		containerArgs = append(containerArgs, []string{"--filter-managed-by", scannedAssetsManagedBy}...)
	}

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GarbageCollectCronJobName(m.Name),
			Namespace: m.Namespace,
			Labels:    GarbageCollectCronJobLabels(m),
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          cronTab,
			ConcurrencyPolicy: batchv1.AllowConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: ls,
						},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							// The node scanning does not use the Kubernetes API at all, therefore the service account token
							// should not be mounted at all.
							AutomountServiceAccountToken: ptr.To(false),
							Containers: []corev1.Container{
								{
									Image:           image,
									ImagePullPolicy: corev1.PullIfNotPresent,
									Name:            "gc",
									Command:         []string{"/mondoo-operator"},
									Args:            containerArgs,
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("30Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("50m"),
											corev1.ResourceMemory: resource.MustParse("20Mi"),
										},
									},
									SecurityContext: &corev1.SecurityContext{
										AllowPrivilegeEscalation: ptr.To(false),
										ReadOnlyRootFilesystem:   ptr.To(true),
										RunAsNonRoot:             ptr.To(true),
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{
												"ALL",
											},
										},
										Privileged: ptr.To(false),
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "token",
											MountPath: "/etc/scanapi",
											ReadOnly:  true,
										},
									},
									Env: feature_flags.AllFeatureFlagsAsEnv(),
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "token",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											DefaultMode: ptr.To(int32(0o444)),
											SecretName:  scanapi.TokenSecretName(m.Name),
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
}

func ConfigMap(node corev1.Node, integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig) (*corev1.ConfigMap, error) {
	inv, err := Inventory(node, integrationMRN, clusterUID, m)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: m.Namespace,
			Name:      ConfigMapName(m.Name, node.Name),
		},
		Data: map[string]string{"inventory": inv},
	}, nil
}

func CronJobName(prefix, suffix string) string {
	// If the name becomes longer than 52 chars, then we hash the suffix and trim
	// it such that the full name fits within 52 chars. This is needed because in
	// manager Kubernetes services such as EKS or GKE the node names can be very long.
	base := fmt.Sprintf("%s%s", prefix, CronJobNameBase)
	return fmt.Sprintf("%s%s", base, NodeNameOrHash(k8s.ResourceNameMaxLength-len(base), suffix))
}

func GarbageCollectCronJobName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, GarbageCollectCronJobNameBase)
}

func ConfigMapName(prefix, nodeName string) string {
	base := fmt.Sprintf("%s%s", prefix, InventoryConfigMapBase)
	return fmt.Sprintf("%s%s", base, NodeNameOrHash(k8s.ResourceNameMaxLength-len(base), nodeName))
}

func Inventory(node corev1.Node, integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig) (string, error) {
	inv := &inventory.Inventory{
		Metadata: &inventory.ObjectMeta{
			Name: "mondoo-node-inventory",
			Labels: map[string]string{
				"environment": "production",
			},
		},
		Spec: &inventory.InventorySpec{
			Assets: []*inventory.Asset{
				{
					Id:   "host",
					Name: node.Name,
					Connections: []*inventory.Config{
						{
							Host:       "/mnt/host",
							PlatformId: fmt.Sprintf("//platformid.api.mondoo.app/runtime/k8s/uid/%s/node/%s", clusterUID, node.UID),
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

	if feature_flags.GetEnableV9() {
		inv.Spec.Assets[0].Connections[0].Type = "fs"
	} else {
		inv.Spec.Assets[0].Connections[0].Backend = inventory.ProviderType_FS
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

func CronJobLabels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo",
		"scan":      "nodes",
		"mondoo_cr": m.Name,
	}
}

func GarbageCollectCronJobLabels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo",
		"gc":        "nodes",
		"mondoo_cr": m.Name,
	}
}

func NodeNameOrHash(allowedLen int, nodeName string) string {
	if len(nodeName) > allowedLen {
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(nodeName)))
		return hash[:int(math.Min(float64(allowedLen), float64(len(hash))))]
	}
	return nodeName
}
