package k8s

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
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
			name: "should not be equal when owner references differ",
			createB: func(a appsv1.Deployment) appsv1.Deployment {
				b := *a.DeepCopy()
				ctrl.SetControllerReference(&a, &b, scheme.Scheme)
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
				ctrl.SetControllerReference(&a, &b, scheme.Scheme)
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

func TestRquriementComparison(t *testing.T) {
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
