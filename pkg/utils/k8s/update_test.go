package k8s

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestUpdateService(t *testing.T) {
	current := corev1.Service{
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
		name       string
		desired    corev1.Service
		validation func(*testing.T, corev1.Service, corev1.Service)
	}{
		{
			name: "should update ports",
			desired: func() corev1.Service {
				s := *current.DeepCopy()
				s.Spec.Ports = append(s.Spec.Ports, s.Spec.Ports[0])
				return s
			}(),
			validation: func(t *testing.T, a, b corev1.Service) {
				assert.Equal(t, a.Spec.Ports, b.Spec.Ports)
			},
		},
		{
			name: "should update selector",
			desired: func() corev1.Service {
				s := *current.DeepCopy()
				s.Spec.Selector["key"] = "value"
				return s
			}(),
			validation: func(t *testing.T, a, b corev1.Service) {
				assert.Equal(t, a.Spec.Selector, b.Spec.Selector)
			},
		},
		{
			name: "should update type",
			desired: func() corev1.Service {
				s := *current.DeepCopy()
				s.Spec.Type = corev1.ServiceTypeLoadBalancer
				return s
			}(),
			validation: func(t *testing.T, a, b corev1.Service) {
				assert.Equal(t, a.Spec.Type, b.Spec.Type)
			},
		},
		{
			name: "should update owner references",
			desired: func() corev1.Service {
				s := current.DeepCopy()
				ctrl.SetControllerReference(&current, s, runtime.NewScheme())
				return *s
			}(),
			validation: func(t *testing.T, a, b corev1.Service) {
				assert.Equal(t, a.GetOwnerReferences(), b.GetOwnerReferences())
			},
		},
		{
			name: "should not update labels",
			desired: func() corev1.Service {
				s := *current.DeepCopy()
				metav1.SetMetaDataLabel(&s.ObjectMeta, "key", "value")
				return s
			}(),
			validation: func(t *testing.T, a, b corev1.Service) {
				assert.Equal(t, current.Labels, a.Labels)
			},
		},
		{
			name: "should not update annotations",
			desired: func() corev1.Service {
				s := *current.DeepCopy()
				metav1.SetMetaDataAnnotation(&s.ObjectMeta, "key", "value")
				return s
			}(),
			validation: func(t *testing.T, a, b corev1.Service) {
				assert.Equal(t, current.Annotations, a.Annotations)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := current.DeepCopy()
			UpdateService(c, test.desired)
			test.validation(t, *c, test.desired)
		})
	}
}

func TestUpdateDeployment(t *testing.T) {
	labels := map[string]string{"label": "value"}
	current := appsv1.Deployment{
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
		name       string
		desired    appsv1.Deployment
		validation func(*testing.T, appsv1.Deployment, appsv1.Deployment)
	}{
		{
			name: "should update spec",
			desired: func() appsv1.Deployment {
				d := *current.DeepCopy()
				d.Spec = appsv1.DeploymentSpec{}
				return d
			}(),
			validation: func(t *testing.T, a, b appsv1.Deployment) {
				assert.Equal(t, a.Spec, b.Spec)
			},
		},
		{
			name: "should update owner references",
			desired: func() appsv1.Deployment {
				d := current.DeepCopy()
				ctrl.SetControllerReference(&current, d, runtime.NewScheme())
				return *d
			}(),
			validation: func(t *testing.T, a, b appsv1.Deployment) {
				assert.Equal(t, a.GetOwnerReferences(), b.GetOwnerReferences())
			},
		},
		{
			name: "should not update labels",
			desired: func() appsv1.Deployment {
				s := *current.DeepCopy()
				metav1.SetMetaDataLabel(&s.ObjectMeta, "key", "value")
				return s
			}(),
			validation: func(t *testing.T, a, b appsv1.Deployment) {
				assert.Equal(t, current.Labels, a.Labels)
			},
		},
		{
			name: "should not update annotations",
			desired: func() appsv1.Deployment {
				s := *current.DeepCopy()
				metav1.SetMetaDataAnnotation(&s.ObjectMeta, "key", "value")
				return s
			}(),
			validation: func(t *testing.T, a, b appsv1.Deployment) {
				assert.Equal(t, current.Annotations, a.Annotations)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := current.DeepCopy()
			UpdateDeployment(c, test.desired)
			test.validation(t, *c, test.desired)
		})
	}
}
