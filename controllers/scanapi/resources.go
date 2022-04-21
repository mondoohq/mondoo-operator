/*
Copyright 2022 Mondoo, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scanapi

import (
	"fmt"

	"go.mondoo.com/mondoo-operator/api/v1alpha1"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

const (
	DeploymentSuffix = "-scan-api"
	ServiceSuffix    = "-scan-api"
	Port             = 8080
)

func ScanApiDeployment(ns, image string, m v1alpha1.MondooAuditConfig) *appsv1.Deployment {
	labels := DeploymentLabels(m)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DeploymentName(m.Name),
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: pointer.Int32(1),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:     image,
						Name:      "mondoo-client",
						Command:   []string{"mondoo", "serve", "--api", "--config", "/etc/opt/mondoo/mondoo.yml"},
						Resources: k8s.DefaultMondooClientResources,
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/Health/Check",
									Port: intstr.FromInt(Port),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       300,
							TimeoutSeconds:      5,
						},
						StartupProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/Health/Check",
									Port: intstr.FromInt(Port),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       5,
							FailureThreshold:    5,
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "config",
								ReadOnly:  true,
								MountPath: "/etc/opt/",
							},
						},
						Ports: []corev1.ContainerPort{
							{ContainerPort: Port, Protocol: corev1.ProtocolTCP},
						},
						Env: []corev1.EnvVar{
							{Name: "DEBUG", Value: "false"},
							{Name: "MONDOO_PROCFS", Value: "on"},
							{Name: "PORT", Value: fmt.Sprintf("%d", Port)},
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

func ScanApiService(ns string, m v1alpha1.MondooAuditConfig) *corev1.Service {
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

func DeploymentLabels(m v1alpha1.MondooAuditConfig) map[string]string {
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
