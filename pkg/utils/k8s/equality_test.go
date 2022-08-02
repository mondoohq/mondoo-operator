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

package k8s

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestAreDeploymentsEqual(t *testing.T) {
	labels := map[string]string{"label": "value"}
	a := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deployment",
			Namespace: "ns",
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
						Image:     "test-image:latest",
						Name:      "mondoo-client",
						Command:   []string{"mondoo", "serve", "--api", "--config", "/etc/opt/mondoo/mondoo.yml"},
						Args:      []string{"argA", "argB", "argC"},
						Resources: DefaultMondooClientResources,
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/Health/Check",
									Port: intstr.FromInt(443),
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
									Port: intstr.FromInt(443),
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
							{ContainerPort: 443, Protocol: corev1.ProtocolTCP},
						},
						Env: []corev1.EnvVar{
							{Name: "DEBUG", Value: "false"},
							{Name: "MONDOO_PROCFS", Value: "on"},
							{Name: "PORT", Value: fmt.Sprintf("%d", 443)},
						},
					}},
					ServiceAccountName: "service-account",
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "secret",
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

	tests := []struct {
		name          string
		createB       func(appsv1.Deployment) appsv1.Deployment
		shouldBeEqual bool
	}{
		{
			name: "should be equal when identical",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				return *a.DeepCopy()
			},
			shouldBeEqual: true,
		},
		{
			name: "should not be equal when container count differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				b.Spec.Template.Spec.Containers = append(
					b.Spec.Template.Spec.Containers, b.Spec.Template.Spec.Containers[0])
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when replicas differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				b.Spec.Replicas = pointer.Int32(3)
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when selectors differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				b.Spec.Selector.MatchLabels["newLabel"] = "newValue"
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when service accounts differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				b.Spec.Template.Spec.ServiceAccountName = "test"
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when container images differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				b.Spec.Template.Spec.Containers[0].Image = "test"
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when container commands differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				b.Spec.Template.Spec.Containers[0].Command = []string{"test"}
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when volume mounts differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				b.Spec.Template.Spec.Containers[0].VolumeMounts = make([]corev1.VolumeMount, 0)
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when env vars differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				b.Spec.Template.Spec.Containers[0].Env = make([]corev1.EnvVar, 0)
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when container resource requirements differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				b.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("233m")
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when owner references differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				assert.NoError(t, ctrl.SetControllerReference(&a, &b, scheme.Scheme))
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when container args differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				b.Spec.Template.Spec.Containers[0].Args = []string{"some", "different", "args"}
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when Pod volume definition(s) differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				b.Spec.Template.Spec.Volumes[0].VolumeSource.Projected.Sources[0].Secret.Items[0].Key = "differentkey"
				return b
			},
			shouldBeEqual: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.shouldBeEqual {
				assert.True(t, AreDeploymentsEqual(a, test.createB(a)))
			} else {
				assert.False(t, AreDeploymentsEqual(a, test.createB(a)))
			}
		})
	}
}

func TestAreServicesEqual(t *testing.T) {
	a := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "ns",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       443,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(443),
				},
			},
			Selector: map[string]string{"label": "value"},
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	tests := []struct {
		name          string
		createB       func(corev1.Service) corev1.Service
		shouldBeEqual bool
	}{
		{
			name: "should be equal when identical",
			createB: func(a corev1.Service) corev1.Service {
				return *a.DeepCopy()
			},
			shouldBeEqual: true,
		},
		{
			name: "should not be equal when ports differ",
			createB: func(a corev1.Service) corev1.Service {
				b := *a.DeepCopy()
				b.Spec.Ports[0].Name = "test"
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when selectors differ",
			createB: func(a corev1.Service) corev1.Service {
				b := *a.DeepCopy()
				b.Spec.Selector["newLabel"] = "newValue"
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when types differ",
			createB: func(a corev1.Service) corev1.Service {
				b := *a.DeepCopy()
				b.Spec.Type = corev1.ServiceTypeExternalName
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when owner references differ",
			createB: func(a corev1.Service) corev1.Service {
				b := *a.DeepCopy()
				assert.NoError(t, ctrl.SetControllerReference(&a, &b, scheme.Scheme))
				return b
			},
			shouldBeEqual: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.shouldBeEqual {
				assert.True(t, AreServicesEqual(a, test.createB(a)))
			} else {
				assert.False(t, AreServicesEqual(a, test.createB(a)))
			}
		})
	}
}

func TestAreCronJobsEqual(t *testing.T) {
	labels := map[string]string{"label": "value"}
	a := batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cronjob",
			Namespace: "ns",
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          "0 * * * *",
			ConcurrencyPolicy: batchv1.AllowConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec: corev1.PodSpec{
							NodeName:      "node01",
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Tolerations: []corev1.Toleration{{
								Key:    "key",
								Effect: corev1.TaintEffectNoExecute,
								Value:  "value",
							}},
							// The node scanning does not use the Kubernetes API at all, therefore the service account token
							// should not be mounted at all.
							AutomountServiceAccountToken: pointer.Bool(false),
							Containers: []corev1.Container{
								{
									Image: "test-image:latest",
									Name:  "mondoo-client",
									Command: []string{
										"mondoo", "scan",
										"--config", "/etc/opt/mondoo/mondoo.yml",
										"--inventory-file", "/etc/opt/mondoo/inventory.yml",
										"--exit-0-on-success",
									},
									Resources: DefaultMondooClientResources,
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
									Name: "config",
									VolumeSource: corev1.VolumeSource{
										Projected: &corev1.ProjectedVolumeSource{
											Sources: []corev1.VolumeProjection{
												{
													ConfigMap: &corev1.ConfigMapProjection{
														LocalObjectReference: corev1.LocalObjectReference{Name: "configMap"},
														Items: []corev1.KeyToPath{{
															Key:  "inventory",
															Path: "mondoo/inventory.yml",
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

	tests := []struct {
		name          string
		createB       func(batchv1.CronJob) batchv1.CronJob
		shouldBeEqual bool
	}{
		{
			name: "should be equal when identical",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				return *a.DeepCopy()
			},
			shouldBeEqual: true,
		},
		{
			name: "should not be equal when container count differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.JobTemplate.Spec.Template.Spec.Containers = append(
					b.Spec.JobTemplate.Spec.Template.Spec.Containers, b.Spec.JobTemplate.Spec.Template.Spec.Containers[0])
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when service accounts differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName = "test"
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when tolerations differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.JobTemplate.Spec.Template.Spec.Tolerations = append(b.Spec.JobTemplate.Spec.Template.Spec.Tolerations, b.Spec.JobTemplate.Spec.Template.Spec.Tolerations[0])
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when node names differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.JobTemplate.Spec.Template.Spec.NodeName = "test-node"
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when container images differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image = "test"
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when container commands differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command = []string{"test"}
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when volume mounts differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts = make([]corev1.VolumeMount, 0)
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when env vars differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env = make([]corev1.EnvVar, 0)
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when container resource requirements differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("233m")
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when owner references differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				assert.NoError(t, ctrl.SetControllerReference(&a, &b, scheme.Scheme))
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when container args differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Args = []string{"some", "different", "args"}
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when Pod volume definition(s) differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.JobTemplate.Spec.Template.Spec.Volumes[0].VolumeSource.Projected.Sources[0].ConfigMap.Items[0].Key = "differentkey"
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when successful jobs history limits differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.SuccessfulJobsHistoryLimit = pointer.Int32(100)
				return b
			},
			shouldBeEqual: false,
		},
		{
			name: "should not be equal when failed jobs history limits differ",
			createB: func(a batchv1.CronJob) batchv1.CronJob {
				b := *a.DeepCopy()
				b.Spec.FailedJobsHistoryLimit = pointer.Int32(100)
				return b
			},
			shouldBeEqual: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.shouldBeEqual {
				assert.True(t, AreCronJobsEqual(a, test.createB(a)))
			} else {
				assert.False(t, AreCronJobsEqual(a, test.createB(a)))
			}
		})
	}
}

func TestAreResouceRequirementsEqual(t *testing.T) {
	r := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("1G"),
			corev1.ResourceCPU:    resource.MustParse("500m"),
		},

		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("500M"), // 50% of the limit
			corev1.ResourceCPU:    resource.MustParse("50m"),  // 10% of the limit
		},
	}

	assert.True(t, AreResouceRequirementsEqual(r, r))
	assert.True(t, AreResouceRequirementsEqual(r, corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("1G"),
			corev1.ResourceCPU:    resource.MustParse("0.5"), // used instead of 500m
		},
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("500M"), // 50% of the limit
			corev1.ResourceCPU:    resource.MustParse("50m"),  // 10% of the limit
		},
	}))
}

func TestAreEnvVarsEqual(t *testing.T) {
	a := []corev1.EnvVar{
		{Name: "a", Value: "2"},
		{Name: "a1", Value: "3"},
	}

	b := []corev1.EnvVar{
		{Name: "a", Value: "2"},
		{Name: "a1", Value: "3"},
	}
	assert.True(t, AreEnvVarsEqual(a, b))
}

func TestAreEnvVarsEqual_DifferentOrder(t *testing.T) {
	a := []corev1.EnvVar{
		{Name: "a", Value: "2"},
		{Name: "a1", Value: "3"},
	}

	b := []corev1.EnvVar{
		{Name: "a1", Value: "3"},
		{Name: "a", Value: "2"},
	}
	assert.True(t, AreEnvVarsEqual(a, b))
}
