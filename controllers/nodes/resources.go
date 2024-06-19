// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"crypto/sha256"
	"fmt"
	"math"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	// That's the mod k8s relies on https://github.com/kubernetes/kubernetes/blob/master/go.mod#L63

	"go.mondoo.com/cnquery/v11/providers-sdk/v1/inventory"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/feature_flags"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

const (
	CronJobNameBase               = "-node-"
	DeploymentNameBase            = "-node-"
	DaemonSetNameBase             = "-node"
	GarbageCollectCronJobNameBase = "-node-gc"
	InventoryConfigMapBase        = "-node-inventory"

	ignoreQueryAnnotationPrefix = "policies.k8s.mondoo.com/"

	ignoreAnnotationValue = "ignore"
)

func UpdateCronJob(cj *batchv1.CronJob, image string, node corev1.Node, m *v1alpha2.MondooAuditConfig, isOpenshift bool, cfg v1alpha2.MondooOperatorConfig) {
	ls := NodeScanningLabels(*m)
	cmd := []string{
		"cnspec", "scan", "local",
		"--config", "/etc/opt/mondoo/mondoo.yml",
		"--inventory-file", "/etc/opt/mondoo/inventory.yml",
		"--score-threshold", "0",
	}

	if cfg.Spec.HttpProxy != nil {
		cmd = append(cmd, []string{"--api-proxy", *cfg.Spec.HttpProxy}...)
	}

	cj.Labels = ls
	cj.Annotations = map[string]string{
		ignoreQueryAnnotationPrefix + "mondoo-kubernetes-security-cronjob-runasnonroot": ignoreAnnotationValue,
	}
	cj.Spec.Schedule = m.Spec.Nodes.Schedule
	cj.Spec.ConcurrencyPolicy = batchv1.ForbidConcurrent
	cj.Spec.SuccessfulJobsHistoryLimit = ptr.To(int32(1))
	cj.Spec.FailedJobsHistoryLimit = ptr.To(int32(1))
	cj.Spec.JobTemplate.Annotations = map[string]string{
		ignoreQueryAnnotationPrefix + "mondoo-kubernetes-security-job-runasnonroot": ignoreAnnotationValue,
	}
	cj.Spec.JobTemplate.Labels = ls
	cj.Spec.JobTemplate.Spec.Template.Annotations = map[string]string{
		ignoreQueryAnnotationPrefix + "mondoo-kubernetes-security-pod-runasnonroot": ignoreAnnotationValue,
	}
	cj.Spec.JobTemplate.Spec.Template.Labels = ls
	cj.Spec.JobTemplate.Spec.Template.Spec.NodeName = node.Name
	cj.Spec.JobTemplate.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
	cj.Spec.JobTemplate.Spec.Template.Spec.Tolerations = k8s.TaintsToTolerations(node.Spec.Taints)
	// The node scanning does not use the Kubernetes API at all, therefore the service account token
	// should not be mounted at all.
	cj.Spec.JobTemplate.Spec.Template.Spec.AutomountServiceAccountToken = ptr.To(false)
	cj.Spec.JobTemplate.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Image:     image,
			Name:      "cnspec",
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
			Env: k8s.MergeEnv([]corev1.EnvVar{
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
				{
					Name: "NODE_NAME",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "spec.nodeName",
						},
					},
				},
			}, m.Spec.Nodes.Env),
			TerminationMessagePath:   "/dev/termination-log",
			TerminationMessagePolicy: corev1.TerminationMessageReadFile,
			ImagePullPolicy:          corev1.PullIfNotPresent,
		},
	}
	cj.Spec.JobTemplate.Spec.Template.Spec.Volumes = []corev1.Volume{
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
	}
}

func UpdateDaemonSet(
	ds *appsv1.DaemonSet,
	m v1alpha2.MondooAuditConfig,
	isOpenshift bool,
	image string,
	cfg v1alpha2.MondooOperatorConfig,
) {
	labels := NodeScanningLabels(m)
	cmd := []string{
		"cnspec", "serve",
		"--config", "/etc/opt/mondoo/mondoo.yml",
		"--inventory-file", "/etc/opt/mondoo/inventory.yml",
		"--timer", fmt.Sprintf("%d", m.Spec.Nodes.IntervalTimer),
	}
	if cfg.Spec.HttpProxy != nil {
		cmd = append(cmd, []string{"--api-proxy", *cfg.Spec.HttpProxy}...)
	}

	ds.Labels = labels
	if ds.Annotations == nil {
		ds.Annotations = map[string]string{}
	}
	ds.Annotations[ignoreQueryAnnotationPrefix+"mondoo-kubernetes-security-deployment-runasnonroot"] = ignoreAnnotationValue
	ds.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}
	ds.Spec.Template.Labels = labels
	if ds.Spec.Template.Annotations == nil {
		ds.Spec.Template.Annotations = map[string]string{}
	}
	ds.Spec.Template.Annotations[ignoreQueryAnnotationPrefix+"mondoo-kubernetes-security-pod-runasnonroot"] = ignoreAnnotationValue
	ds.Spec.Template.Spec.PriorityClassName = m.Spec.Nodes.PriorityClassName
	// The node scanning does not use the Kubernetes API at all, therefore the service account token
	// should not be mounted at all.
	ds.Spec.Template.Spec.AutomountServiceAccountToken = ptr.To(false)
	ds.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Image:     image,
			Name:      "cnspec",
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
			TerminationMessagePath:   "/dev/termination-log",
			TerminationMessagePolicy: corev1.TerminationMessageReadFile,
			ImagePullPolicy:          corev1.PullIfNotPresent,
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
			Env: k8s.MergeEnv([]corev1.EnvVar{
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
				{
					Name: "NODE_NAME",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "spec.nodeName",
						},
					},
				},
			}, m.Spec.Nodes.Env),
		},
	}
	ds.Spec.Template.Spec.Volumes = []corev1.Volume{
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
	}
}

func UpdateGarbageCollectCronJob(cj *batchv1.CronJob, image, clusterUid string, m v1alpha2.MondooAuditConfig) {
	ls := NodeScanningLabels(m)

	cronTab := fmt.Sprintf("%d */12 * * *", time.Now().Add(1*time.Minute).Minute())
	scanApiUrl := scanapi.ScanApiServiceUrl(m)
	containerArgs := []string{
		"garbage-collect",
		"--scan-api-url", scanApiUrl,
		"--token-file-path", "/etc/scanapi/token",

		// The job runs hourly and we need to make sure that the previous one is killed before the new one is started so we don't stack them.
		"--timeout", "55",
		// Cleanup any resources more than 48 hours old
		"--filter-older-than", "48h",
		"--labels", "k8s.mondoo.com/kind=node",
	}

	if clusterUid != "" {
		scannedAssetsManagedBy := "mondoo-operator-" + clusterUid
		containerArgs = append(containerArgs, []string{"--filter-managed-by", scannedAssetsManagedBy}...)
	}

	cj.Labels = GarbageCollectCronJobLabels(m)
	cj.Spec.Schedule = cronTab
	cj.Spec.ConcurrencyPolicy = batchv1.ForbidConcurrent
	cj.Spec.SuccessfulJobsHistoryLimit = ptr.To(int32(1))
	cj.Spec.FailedJobsHistoryLimit = ptr.To(int32(1))
	cj.Spec.JobTemplate.Labels = ls
	cj.Spec.JobTemplate.Spec.Template.Labels = ls
	cj.Spec.JobTemplate.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
	// The node scanning does not use the Kubernetes API at all, therefore the service account token
	// should not be mounted at all.
	cj.Spec.JobTemplate.Spec.Template.Spec.AutomountServiceAccountToken = ptr.To(false)
	cj.Spec.JobTemplate.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Image:                    image,
			ImagePullPolicy:          corev1.PullIfNotPresent,
			Name:                     "gc",
			Command:                  []string{"/mondoo-operator"},
			Args:                     containerArgs,
			TerminationMessagePath:   "/dev/termination-log",
			TerminationMessagePolicy: corev1.TerminationMessageReadFile,
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
	}
	cj.Spec.JobTemplate.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "token",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: ptr.To(int32(0o444)),
					SecretName:  scanapi.TokenSecretName(m.Name),
				},
			},
		},
	}
}

func UpdateConfigMap(cm *corev1.ConfigMap, integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig) error {
	inv, err := Inventory(integrationMRN, clusterUID, m)
	if err != nil {
		return err
	}
	cm.Data = map[string]string{"inventory": inv}
	return nil
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
	// If the name becomes longer than 52 chars, then we hash the suffix and trim
	// it such that the full name fits within 52 chars. This is needed because in
	// manager Kubernetes services such as EKS or GKE the node names can be very long.
	return fmt.Sprintf("%s%s", prefix, DaemonSetNameBase)
}

func GarbageCollectCronJobName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, GarbageCollectCronJobNameBase)
}

func ConfigMapName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, InventoryConfigMapBase)
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
							PlatformId: fmt.Sprintf("//platformid.api.mondoo.app/runtime/k8s/uid/%s/node/{{- getenv NODE_NAME -}}", clusterUID),
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
