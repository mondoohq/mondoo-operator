package controllers

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	scanApiDeploymentName = "mondoo-scan-api"
	scanApiServiceName    = "mondoo-scan-api"
)

func ScanApiDeployment(ns string, m *v1alpha1.MondooAuditConfig) appsv1.Deployment {
	ls := map[string]string{"app": "mondoo-scan-api"}
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scanApiDeploymentName,
			Namespace: ns,
			Labels:    ls,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:     mondooImage,
						Name:      "mondoo-client",
						Command:   []string{"mondoo", "serve", "--api", "--config", "/etc/opt/mondoo/mondoo.yml"},
						Resources: getResourcesRequirements(m.Spec.Workloads.Resources),
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/Health/Check",
									Port: intstr.FromInt(8989), // TODO: this fails because currently the API binds to 127.0.0.1
								},
							},
							InitialDelaySeconds: 10,
							PeriodSeconds:       300,
							TimeoutSeconds:      5,
						},
						VolumeMounts: []corev1.VolumeMount{
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
					}},
					ServiceAccountName: m.Spec.Workloads.ServiceAccount,
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: m.Spec.MondooSecretRef,
												},
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
	}
}

func ScanApiService(ns string) corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scanApiServiceName,
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       int32(8989),
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(8989),
				},
			},
			Selector: map[string]string{
				webhookLabelKey: webhookLabelValue,
			},
		},
	}
}
