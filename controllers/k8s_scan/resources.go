// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"fmt"
	"strings"

	// That's the mod k8s relies on https://github.com/kubernetes/kubernetes/blob/master/go.mod#L63

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/feature_flags"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const CronJobNameSuffix = "-k8s-scan"

func CronJob(image, integrationMrn, clusterUid string, m *v1alpha2.MondooAuditConfig) *batchv1.CronJob {
	ls := CronJobLabels(*m)
	scanApiUrl := scanapi.ScanApiServiceUrl(*m)

	containerArgs := []string{
		"k8s-scan",
		"--scan-api-url", scanApiUrl,
		"--token-file-path", "/etc/scanapi/token",

		// The job runs hourly and we need to make sure that the previous one is killed before the new one is started so we don't stack them.
		"--timeout", "55",
		// Cleanup any resources more than 2 hours old
		"--cleanup-assets-older-than", "2h",
		"--namespaces", strings.Join(m.Spec.Filtering.Namespaces.Include, ","),
		"--namespaces-exclude", strings.Join(m.Spec.Filtering.Namespaces.Exclude, ","),
	}

	if integrationMrn != "" {
		containerArgs = append(containerArgs, []string{"--integration-mrn", integrationMrn}...)
	}

	// use the clusterUid to uniquely set the managedBy field on assets managed by this instance of the mondoo-operator
	if clusterUid == "" {
		logger.Info("no clusterUid provided, will not set ManagedBy field on scanned/discovered assets")
	} else {
		scannedAssetsManagedBy := "mondoo-operator-" + clusterUid

		containerArgs = append(containerArgs, []string{"--set-managed-by", scannedAssetsManagedBy}...)
	}

	return &batchv1.CronJob{
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
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: ls},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							// Triggering the Kubernetes resources scan does not require any API access, therefore no service account
							// is needed.
							AutomountServiceAccountToken: ptr.To(false),
							Containers: []corev1.Container{
								{
									Image:           image,
									ImagePullPolicy: corev1.PullIfNotPresent,
									Name:            "mondoo-k8s-scan",
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

func CronJobLabels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo-k8s-scan",
		"scan":      "k8s",
		"mondoo_cr": m.Name,
	}
}

func CronJobName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, CronJobNameSuffix)
}
