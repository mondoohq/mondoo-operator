/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package scanapi

import (
	"fmt"

	"github.com/google/uuid"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

const (
	DeploymentSuffix = "-scan-api"
	ServiceSuffix    = "-scan-api"
	SecretSuffix     = "-scan-api-token"
	Port             = 8080
)

func ScanApiSecret(mondoo v1alpha2.MondooAuditConfig) *corev1.Secret {
	// Generate a token. It will only be saved on initial Secret creation.
	token := uuid.New()

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TokenSecretName(mondoo.Name),
			Namespace: mondoo.Namespace,
		},
		StringData: map[string]string{
			"token": token.String(),
		},
	}
}

func ScanApiDeployment(ns, image string, m v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig, privateImageScanningSecretName string, deployOnOpenShift bool) *appsv1.Deployment {
	labels := DeploymentLabels(m)

	name := "cnspec"
	cmd := []string{
		"cnspec", "serve-api",
		"--address", "0.0.0.0",
		"--config", "/etc/opt/mondoo/config/mondoo.yml",
		"--http-timeout", "1800",
	}

	if cfg.Spec.HttpProxy != nil {
		cmd = append(cmd, []string{"--api-proxy", *cfg.Spec.HttpProxy}...)
	}

	healthcheckEndpoint := "/Scan/HealthCheck"

	scanApiDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DeploymentName(m.Name),
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: m.Spec.Scanner.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:     image,
						Name:      name,
						Command:   cmd,
						Resources: k8s.ResourcesRequirementsWithDefaults(m.Spec.Scanner.Resources, k8s.DefaultCnspecResources),
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: healthcheckEndpoint,
									Port: intstr.FromInt(Port),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       10,
							TimeoutSeconds:      5,
						},
						StartupProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: healthcheckEndpoint,
									Port: intstr.FromInt(Port),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       5,
							FailureThreshold:    5,
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: healthcheckEndpoint,
									Port: intstr.FromInt(Port),
								},
							},
							InitialDelaySeconds: 10,
							PeriodSeconds:       10,
							TimeoutSeconds:      5,
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: pointer.Bool(false),
							ReadOnlyRootFilesystem:   pointer.Bool(true),
							RunAsNonRoot:             pointer.Bool(true),
							// This is needed to prevent:
							// Error: container has runAsNonRoot and image has non-numeric user (mondoo), cannot verify user is non-root ...
							RunAsUser: pointer.Int64(101),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
							Privileged: pointer.Bool(false),
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "config",
								ReadOnly:  true,
								MountPath: "/etc/opt/mondoo/config",
							},
							{
								Name:      "token",
								ReadOnly:  true,
								MountPath: "/etc/opt/mondoo/token",
							},
							{
								Name:      "temp",
								MountPath: "/tmp",
							},
						},
						Ports: []corev1.ContainerPort{
							{ContainerPort: Port, Protocol: corev1.ProtocolTCP},
						},
						Env: []corev1.EnvVar{
							{Name: "DEBUG", Value: "false"},
							{Name: "MONDOO_PROCFS", Value: "on"},
							{Name: "PORT", Value: fmt.Sprintf("%d", Port)},

							// Required so the scan API knows it is running as a Kubernetes integration
							{Name: "KUBERNETES_ADMISSION_CONTROLLER", Value: "true"},
						},
					}},
					ServiceAccountName: m.Spec.Scanner.ServiceAccountName,
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
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
									DefaultMode: pointer.Int32(0o444),
								},
							},
						},
						{
							Name: "token",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: TokenSecretName(m.Name),
												},
												Items: []corev1.KeyToPath{
													{
														Key:  "token",
														Path: "token",
													},
												},
											},
										},
									},
									DefaultMode: pointer.Int32(0o444),
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
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: labels,
										},
										TopologyKey: "kubernetes.io/hostname",
									},
									Weight: int32(100),
								},
							},
						},
					},
				},
			},
		},
	}

	if privateImageScanningSecretName != "" {
		// mount secret needed to pull images from private registries
		scanApiDeployment.Spec.Template.Spec.Volumes = append(scanApiDeployment.Spec.Template.Spec.Volumes, corev1.Volume{
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

		scanApiDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(scanApiDeployment.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "pull-secrets",
			ReadOnly:  true,
			MountPath: "/etc/opt/mondoo/docker",
		})

		scanApiDeployment.Spec.Template.Spec.Containers[0].Env = append(scanApiDeployment.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  "DOCKER_CONFIG",
			Value: "/etc/opt/mondoo/docker", // the client automatically adds '/config.json' to the path
		})
	}

	// Merge the operator env for the scanner with the one provided in the MondooAuditConfig
	scanApiDeployment.Spec.Template.Spec.Containers[0].Env = k8s.MergeEnv(scanApiDeployment.Spec.Template.Spec.Containers[0].Env, m.Spec.Scanner.Env)

	if deployOnOpenShift {
		// OpenShift will set its own UID in the range assinged to the Namespace the Pod is running
		// in; so clear out our 101 UID otherwise OpenShift SCCs will fail the Pod do to it not using
		// a UID in the assigned range.
		scanApiDeployment.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser = nil
	}

	return scanApiDeployment
}

func ScanApiService(ns string, m v1alpha2.MondooAuditConfig) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName(m.Name),
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       int32(Port),
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(Port),
				},
			},
			Selector: DeploymentLabels(m),
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
}

func ScanApiServiceUrl(m v1alpha2.MondooAuditConfig) string {
	// The URL to communicate with will be http://ScanAPIServiceName-ScanAPIServiceNamespace.svc:ScanAPIPort
	return fmt.Sprintf("http://%s.%s.svc:%d", ServiceName(m.Name), m.Namespace, Port)
}

func DeploymentLabels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo-scan-api",
		"mondoo_cr": m.Name,
	}
}

func ServiceName(prefix string) string {
	return prefix + ServiceSuffix
}

func DeploymentName(prefix string) string {
	return prefix + DeploymentSuffix
}

func TokenSecretName(prefix string) string {
	return prefix + SecretSuffix
}
