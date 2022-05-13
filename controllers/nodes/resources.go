package nodes

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"time"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

const (
	CronJobNameBase = "-node-"

	// TODO: remove in a follow-up version
	OldCronJobNameBase = "-node-scanning-"

	// Execute hourly
	CronTab                  = "0 * * * *"
	InventoryConfigMapSuffix = "-node-scanning-inventory"
)

var (
	//go:embed inventory.yaml
	inventoryYaml []byte
)

func CronJob(image string, node v1.Node, m v1alpha2.MondooAuditConfig) *batchv1.CronJob {
	ls := CronJobLabels(m)

	cronTab := fmt.Sprintf("%d * * * *", time.Now().Add(1*time.Minute).Minute())

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CronJobName(m.Name, node.Name),
			Namespace: m.Namespace,
			Labels:    CronJobLabels(m),
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          cronTab,
			ConcurrencyPolicy: batchv1.AllowConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: ls},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: ls},
						Spec: corev1.PodSpec{
							NodeName:      node.Name,
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Tolerations:   k8s.TaintsToTolerations(node.Spec.Taints),
							// The node scanning does not use the Kubernetes API at all, therefore the service account token
							// should not be mounted at all.
							AutomountServiceAccountToken: pointer.Bool(false),
							Containers: []corev1.Container{
								{
									Image: image,
									Name:  "mondoo-client",
									Command: []string{
										"mondoo", "scan",
										"--config", "/etc/opt/mondoo/mondoo.yml",
										"--inventory-file", "/etc/opt/mondoo/inventory.yml",
										"--exit-0-on-success",
									},
									Resources: k8s.ResourcesRequirementsWithDefaults(m.Spec.Scanner.Resources),
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
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "root",
									VolumeSource: corev1.VolumeSource{
										HostPath: &corev1.HostPathVolumeSource{Path: "/"},
									},
								},
								{
									Name: "config",
									VolumeSource: corev1.VolumeSource{
										Projected: &corev1.ProjectedVolumeSource{
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
}

func ConfigMap(m v1alpha2.MondooAuditConfig) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: m.Namespace,
			Name:      ConfigMapName(m.Name),
		},
		Data: map[string]string{"inventory": Inventory(m)},
	}
}

func CronJobName(prefix string, suffix string) string {
	name := fmt.Sprintf("%s%s%s", prefix, CronJobNameBase, suffix)

	// If the name becomes longer than 52 chars, then we hash the suffix and trim
	// it such that the full name fits within 52 chars. This is needed because in
	// manager Kubernetes services such as EKS or GKE the node names can be very long.
	if len(name) > 52 {
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(suffix)))

		base := fmt.Sprintf("%s%s", prefix, CronJobNameBase)
		return fmt.Sprintf("%s%s", base, hash[:52-len(base)])
	}
	return name
}

func OldCronJobName(prefix string, suffix string) string {
	return fmt.Sprintf("%s%s%s", prefix, OldCronJobNameBase, suffix)
}

func ConfigMapName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, InventoryConfigMapSuffix)
}

func Inventory(m v1alpha2.MondooAuditConfig) string {
	return string(inventoryYaml)
}

func CronJobLabels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo",
		"scan":      "nodes",
		"mondoo_cr": m.Name,
	}
}
