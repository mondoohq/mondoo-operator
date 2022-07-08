package nodes

import (
	"crypto/sha256"
	"fmt"
	"math"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/yaml"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/inventory"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

const (
	CronJobNameBase        = "-node-"
	InventoryConfigMapBase = "-node-inventory-"

	// TODO: remove in a follow-up version
	OldCronJobNameBase          = "-node-scanning-"
	OldInventoryConfigMapSuffix = "-node-scanning-inventory"

	// Execute hourly
	CronTab = "0 * * * *"
)

func CronJob(image string, node corev1.Node, m v1alpha2.MondooAuditConfig) *batchv1.CronJob {
	ls := CronJobLabels(m)

	cronTab := fmt.Sprintf("%d * * * *", time.Now().Add(1*time.Minute).Minute())
	unsetHostPath := corev1.HostPathUnset
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
										"--score-threshold", "0",
									},
									Resources: k8s.ResourcesRequirementsWithDefaults(m.Spec.Nodes.Resources),
									SecurityContext: &corev1.SecurityContext{
										AllowPrivilegeEscalation: pointer.Bool(false),
										ReadOnlyRootFilesystem:   pointer.Bool(true),
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
										HostPath: &corev1.HostPathVolumeSource{Path: "/", Type: &unsetHostPath},
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

func ConfigMap(node corev1.Node, integrationMRN string, m v1alpha2.MondooAuditConfig) (*corev1.ConfigMap, error) {
	inv, err := Inventory(node, integrationMRN, m)
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
	return fmt.Sprintf("%s%s", base, NodeNameOrHash(52-len(base), suffix))
}

func OldCronJobName(prefix, suffix string) string {
	return fmt.Sprintf("%s%s%s", prefix, OldCronJobNameBase, suffix)
}

func ConfigMapName(prefix, nodeName string) string {
	base := fmt.Sprintf("%s%s", prefix, InventoryConfigMapBase)
	return fmt.Sprintf("%s%s", base, NodeNameOrHash(52-len(base), nodeName))
}

func OldConfigMapName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, OldInventoryConfigMapSuffix)
}

func Inventory(node corev1.Node, integrationMRN string, m v1alpha2.MondooAuditConfig) (string, error) {
	inv := inventory.MondooInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mondoo-node-inventory",
			Labels: map[string]string{
				"environment": "production",
			},
		},
		Spec: inventory.MondooInventorySpec{
			Assets: []inventory.Asset{
				{
					Id:   "host",
					Name: node.Name,
					IdDetector: []string{
						"machine-id",
					},
					Connections: []inventory.TransportConfig{
						{
							Host:    "/mnt/host",
							Backend: inventory.TransportBackend_CONNECTION_FS,
						},
					},
					Labels: map[string]string{},
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

func CronJobLabels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo",
		"scan":      "nodes",
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
