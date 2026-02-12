// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"crypto/sha256"
	"fmt"
	"math"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	// That's the mod k8s relies on https://github.com/kubernetes/kubernetes/blob/master/go.mod#L63

	"go.mondoo.com/cnquery/v12/providers-sdk/v1/inventory"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/gomemlimit"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

const (
	CronJobNameBase                = "-node-"
	DeploymentNameBase             = "-node-"
	DaemonSetNameBase              = "-node"
	GarbageCollectCronJobNameBase  = "-node-gc"
	InventoryConfigMapBase         = "-node-inventory"
	InventoryConfigMapWithNodeBase = "-node-inventory-"

	ignoreQueryAnnotationPrefix = "policies.k8s.mondoo.com/"

	ignoreAnnotationValue = "ignore"
)

// CronJob creates a CronJob for node scanning
func CronJob(image string, node corev1.Node, m *v1alpha2.MondooAuditConfig, isOpenshift bool, cfg v1alpha2.MondooOperatorConfig) *batchv1.CronJob {
	ls := NodeScanningLabels(*m)
	cmd := []string{
		"cnspec", "scan", "local",
		"--config", "/etc/opt/mondoo/mondoo.yml",
		"--inventory-template", "/etc/opt/mondoo/inventory_template.yml",
	}

	if cfg.Spec.HttpProxy != nil {
		cmd = append(cmd, []string{"--api-proxy", *cfg.Spec.HttpProxy}...)
	}

	containerResources := k8s.ResourcesRequirementsWithDefaults(m.Spec.Nodes.Resources, k8s.DefaultNodeScanningResources)
	gcLimit := gomemlimit.CalculateGoMemLimit(containerResources)

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CronJobName(m.Name, node.Name),
			Namespace: m.Namespace,
			Labels:    ls,
			Annotations: map[string]string{
				ignoreQueryAnnotationPrefix + "mondoo-kubernetes-security-cronjob-runasnonroot": ignoreAnnotationValue,
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   m.Spec.Nodes.Schedule,
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			SuccessfulJobsHistoryLimit: ptr.To(int32(1)),
			FailedJobsHistoryLimit:     ptr.To(int32(1)),
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
					Annotations: map[string]string{
						ignoreQueryAnnotationPrefix + "mondoo-kubernetes-security-job-runasnonroot": ignoreAnnotationValue,
					},
				},
				Spec: batchv1.JobSpec{
					// Allow one retry for node scanning (transient issues possible)
					BackoffLimit: ptr.To(int32(1)),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: ls,
							Annotations: map[string]string{
								ignoreQueryAnnotationPrefix + "mondoo-kubernetes-security-pod-runasnonroot": ignoreAnnotationValue,
							},
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
									Name:      "cnspec",
									Command:   cmd,
									Resources: containerResources,
									SecurityContext: &corev1.SecurityContext{
										AllowPrivilegeEscalation: ptr.To(isOpenshift),
										ReadOnlyRootFilesystem:   ptr.To(true),
										RunAsNonRoot:             ptr.To(false),
										RunAsUser:                ptr.To(int64(0)),
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{"ALL"},
										},
										// RHCOS requires to run as privileged to properly do node scanning. If the container
										// is not privileged, then we have no access to /proc.
										Privileged: ptr.To(isOpenshift),
									},
									VolumeMounts: []corev1.VolumeMount{
										{Name: "root", ReadOnly: true, MountPath: "/mnt/host/"},
										{Name: "config", ReadOnly: true, MountPath: "/etc/opt/"},
										{Name: "temp", MountPath: "/tmp"},
									},
									Env: k8s.MergeEnv([]corev1.EnvVar{
										{Name: "DEBUG", Value: "false"},
										{Name: "MONDOO_PROCFS", Value: "on"},
										{Name: "MONDOO_AUTO_UPDATE", Value: "false"},
										{Name: "NODE_NAME", Value: node.Name},
										{Name: "GOMEMLIMIT", Value: gcLimit},
									}, m.Spec.Nodes.Env),
									TerminationMessagePath:   "/dev/termination-log",
									TerminationMessagePolicy: corev1.TerminationMessageReadFile,
									ImagePullPolicy:          corev1.PullIfNotPresent,
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "root",
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{Path: "/", Type: ptr.To(corev1.HostPathUnset)},
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
														LocalObjectReference: corev1.LocalObjectReference{Name: ConfigMapName(m.Name)},
														Items:                []corev1.KeyToPath{{Key: "inventory", Path: "mondoo/inventory_template.yml"}},
													},
												},
												{
													Secret: &corev1.SecretProjection{
														LocalObjectReference: m.Spec.MondooCredsSecretRef,
														Items:                []corev1.KeyToPath{{Key: "config", Path: "mondoo/mondoo.yml"}},
													},
												},
											},
										},
									},
								},
								{
									Name:         "temp",
									VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
								},
							},
						},
					},
				},
			},
		},
	}
}

// DaemonSet creates a DaemonSet for node scanning
func DaemonSet(m v1alpha2.MondooAuditConfig, isOpenshift bool, image string, cfg v1alpha2.MondooOperatorConfig, tolerations []corev1.Toleration) *appsv1.DaemonSet {
	labels := NodeScanningLabels(m)
	cmd := []string{
		"cnspec", "serve",
		"--config", "/etc/opt/mondoo/mondoo.yml",
		"--inventory-template", "/etc/opt/mondoo/inventory_template.yml",
		"--timer", fmt.Sprintf("%d", m.Spec.Nodes.IntervalTimer),
	}
	if cfg.Spec.HttpProxy != nil {
		cmd = append(cmd, []string{"--api-proxy", *cfg.Spec.HttpProxy}...)
	}

	containerResources := k8s.ResourcesRequirementsWithDefaults(m.Spec.Nodes.Resources, k8s.DefaultNodeScanningResources)
	gcLimit := gomemlimit.CalculateGoMemLimit(containerResources)

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DaemonSetName(m.Name),
			Namespace: m.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				ignoreQueryAnnotationPrefix + "mondoo-kubernetes-security-deployment-runasnonroot": ignoreAnnotationValue,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						ignoreQueryAnnotationPrefix + "mondoo-kubernetes-security-pod-runasnonroot": ignoreAnnotationValue,
					},
				},
				Spec: corev1.PodSpec{
					PriorityClassName: m.Spec.Nodes.PriorityClassName,
					// The node scanning does not use the Kubernetes API at all, therefore the service account token
					// should not be mounted at all.
					AutomountServiceAccountToken: ptr.To(false),
					Tolerations:                  tolerations,
					Containers: []corev1.Container{
						{
							Image:     image,
							Name:      "cnspec",
							Command:   cmd,
							Resources: containerResources,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(isOpenshift),
								ReadOnlyRootFilesystem:   ptr.To(true),
								RunAsNonRoot:             ptr.To(false),
								RunAsUser:                ptr.To(int64(0)),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								// RHCOS requires to run as privileged to properly do node scanning. If the container
								// is not privileged, then we have no access to /proc.
								Privileged: ptr.To(isOpenshift),
							},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							ImagePullPolicy:          corev1.PullIfNotPresent,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "root", ReadOnly: true, MountPath: "/mnt/host/"},
								{Name: "config", ReadOnly: true, MountPath: "/etc/opt/"},
								{Name: "temp", MountPath: "/tmp"},
							},
							Env: k8s.MergeEnv([]corev1.EnvVar{
								{Name: "DEBUG", Value: "false"},
								{Name: "MONDOO_PROCFS", Value: "on"},
								{Name: "MONDOO_AUTO_UPDATE", Value: "false"},
								{Name: "GOMEMLIMIT", Value: gcLimit},
								{Name: "NODE_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
							}, m.Spec.Nodes.Env),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "root",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{Path: "/", Type: ptr.To(corev1.HostPathUnset)},
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
												LocalObjectReference: corev1.LocalObjectReference{Name: ConfigMapName(m.Name)},
												Items:                []corev1.KeyToPath{{Key: "inventory", Path: "mondoo/inventory_template.yml"}},
											},
										},
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: m.Spec.MondooCredsSecretRef,
												Items:                []corev1.KeyToPath{{Key: "config", Path: "mondoo/mondoo.yml"}},
											},
										},
									},
								},
							},
						},
						{
							Name:         "temp",
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
						},
					},
				},
			},
		},
	}
}

// ConfigMap creates a ConfigMap for node scanning inventory
func ConfigMap(integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig) (*corev1.ConfigMap, error) {
	inv, err := Inventory(integrationMRN, clusterUID, m)
	if err != nil {
		return nil, err
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName(m.Name),
			Namespace: m.Namespace,
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

func DeploymentName(prefix, suffix string) string {
	// If the name becomes longer than 52 chars, then we hash the suffix and trim
	// it such that the full name fits within 52 chars. This is needed because in
	// manager Kubernetes services such as EKS or GKE the node names can be very long.
	base := fmt.Sprintf("%s%s", prefix, DeploymentNameBase)
	return fmt.Sprintf("%s%s", base, NodeNameOrHash(k8s.ResourceNameMaxLength-len(base), suffix))
}

func DaemonSetName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, DaemonSetNameBase)
}

func GarbageCollectCronJobName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, GarbageCollectCronJobNameBase)
}

func ConfigMapName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, InventoryConfigMapBase)
}

func ConfigMapNameWithNode(prefix, nodeName string) string {
	base := fmt.Sprintf("%s%s", prefix, InventoryConfigMapWithNodeBase)
	return fmt.Sprintf("%s%s", base, NodeNameOrHash(k8s.ResourceNameMaxLength-len(base), nodeName))
}

func Inventory(integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig) (string, error) {
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
					Name: `{{ getenv "NODE_NAME" }}`,
					Connections: []*inventory.Config{
						{
							Type:       "filesystem",
							Host:       "/mnt/host",
							PlatformId: fmt.Sprintf(`{{ printf "//platformid.api.mondoo.app/runtime/k8s/uid/%%s/node/%%s" "%s" (getenv "NODE_NAME")}}`, clusterUID),
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

	// Add user-defined annotations to all assets
	if len(m.Spec.Annotations) > 0 {
		for i := range inv.Spec.Assets {
			inv.Spec.Assets[i].AddAnnotations(m.Spec.Annotations)
		}
	}

	invBytes, err := yaml.Marshal(inv)
	if err != nil {
		return "", err
	}

	return string(invBytes), nil
}

func NodeScanningLabels(m v1alpha2.MondooAuditConfig) map[string]string {
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
