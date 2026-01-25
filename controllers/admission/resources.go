// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package admission

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/feature_flags"
)

const (
	webhookDeploymentLabelKey   = "app.kubernetes.io/name"
	webhookDeploymentLabelValue = "mondoo-operator-webhook"

	// openShiftServiceAnnotationKey is how we annotate a Service so that OpenShift
	// will create TLS certificates for the webhook Service.
	openShiftServiceAnnotationKey = "service.beta.openshift.io/serving-cert-secret-name"

	// openShiftWebhookAnnotationKey is how we annotate a webhook so that OpenShift
	// injects the cluster-wide CA data for auto-generated TLS certificates.
	openShiftWebhookAnnotationKey = "service.beta.openshift.io/inject-cabundle"

	// manualTLSAnnotationKey is for when there is no explicit 'injection-style' defined in
	// the MondooAuditConfig. We treat this to mean that the user will provide their own certs.
	manualTLSAnnotationKey = "mondoo.com/tls-mode"
)

// webhookAnnotationList is a list of all the possible annotations we could set on a Webhook.
// It is needed so we can be sure to clear out any previous annotations in the event the
// injection-style has changed during runtime.
var webhookAnnotationList = []string{certManagerAnnotationKey, openShiftWebhookAnnotationKey, manualTLSAnnotationKey}

// GetTLSCertificatesSecretName takes the name of a MondooAuditConfig resources
// and returns the expected Secret name where the TLS certs will be stored.
func GetTLSCertificatesSecretName(mondooAuditConfigName string) string {
	// webhookTLSSecretNameTemplate is intended to store the MondooAuditConfig Name for Secret
	// name uniqueness per-Namespace.
	webhookTLSSecretNameTemplate := `%s-webhook-server-cert` // #nosec G101

	return fmt.Sprintf(webhookTLSSecretNameTemplate, mondooAuditConfigName)
}

func WebhookDeployment(ns, image string, m mondoov1alpha2.MondooAuditConfig, cfg mondoov1alpha2.MondooOperatorConfig, integrationMRN, clusterID string) *appsv1.Deployment {
	scanApiUrl := scanapi.ScanApiServiceUrl(m)

	containerArgs := []string{
		"webhook",
		"--token-file-path",
		"/etc/scanapi/token",
		"--enforcement-mode",
		string(m.Spec.Admission.Mode),
		"--scan-api-url",
		scanApiUrl,
		"--cluster-id",
		clusterID,
		"--namespaces",
		strings.Join(m.Spec.Filtering.Namespaces.Include, ","),
		"--namespaces-exclude",
		strings.Join(m.Spec.Filtering.Namespaces.Exclude, ","),
	}

	if integrationMRN != "" {
		containerArgs = append(containerArgs, []string{"--integration-mrn", integrationMRN}...)
	}

	// Build proxy environment variables
	var proxyEnvVars []corev1.EnvVar
	if cfg.Spec.HttpProxy != nil {
		proxyEnvVars = append(proxyEnvVars, corev1.EnvVar{Name: "HTTP_PROXY", Value: *cfg.Spec.HttpProxy})
		proxyEnvVars = append(proxyEnvVars, corev1.EnvVar{Name: "http_proxy", Value: *cfg.Spec.HttpProxy})
	}
	if cfg.Spec.HttpsProxy != nil {
		proxyEnvVars = append(proxyEnvVars, corev1.EnvVar{Name: "HTTPS_PROXY", Value: *cfg.Spec.HttpsProxy})
		proxyEnvVars = append(proxyEnvVars, corev1.EnvVar{Name: "https_proxy", Value: *cfg.Spec.HttpsProxy})
	}
	if cfg.Spec.NoProxy != nil {
		proxyEnvVars = append(proxyEnvVars, corev1.EnvVar{Name: "NO_PROXY", Value: *cfg.Spec.NoProxy})
		proxyEnvVars = append(proxyEnvVars, corev1.EnvVar{Name: "no_proxy", Value: *cfg.Spec.NoProxy})
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webhookDeploymentName(m.Name),
			Namespace: ns,
			Labels:    WebhookDeploymentLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: m.Spec.Admission.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: WebhookDeploymentLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: WebhookDeploymentLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Command: []string{
								"/mondoo-operator",
							},
							Args:            containerArgs,
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(8081),
									},
								},
								InitialDelaySeconds: int32(30),
								PeriodSeconds:       int32(20),
								TimeoutSeconds:      int32(10),
								FailureThreshold:    int32(3),
							},
							Env:  feature_flags.AllFeatureFlagsAsEnv(),
							Name: "webhook",
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.FromInt(8081),
									},
								},
								InitialDelaySeconds: int32(10),
								PeriodSeconds:       int32(10),
								TimeoutSeconds:      int32(5),
								FailureThreshold:    int32(3),
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								ReadOnlyRootFilesystem:   ptr.To(true),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{
										"ALL",
									},
								},
								Privileged: ptr.To(false),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									// This is just the default path if no specific mountpoint is set.
									// It is not exported anywhere in controller-manager.
									// https://github.com/kubernetes-sigs/controller-runtime/blob/master/pkg/webhook/server.go
									MountPath: "/tmp/k8s-webhook-server/serving-certs",
									Name:      "cert",
									ReadOnly:  true,
								},
								{
									Name:      "token",
									MountPath: "/etc/scanapi",
									ReadOnly:  true,
								},
							},
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
					},
					// service account used during the initial bringup of the webhook
					// by the supporting controller-runtime packages
					ServiceAccountName:            m.Spec.Admission.ServiceAccountName,
					TerminationGracePeriodSeconds: ptr.To(int64(10)),
					Volumes: []corev1.Volume{
						{
							Name: "cert",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									DefaultMode: ptr.To(int32(420)),
									SecretName:  GetTLSCertificatesSecretName(m.Name),
								},
							},
						},
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
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: WebhookDeploymentLabels(),
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

	// Add proxy environment variables
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, proxyEnvVars...)

	// Add imagePullSecrets from MondooOperatorConfig
	if len(cfg.Spec.ImagePullSecrets) > 0 {
		deployment.Spec.Template.Spec.ImagePullSecrets = cfg.Spec.ImagePullSecrets
	}

	return deployment
}

func WebhookDeploymentLabels() map[string]string {
	return map[string]string{
		webhookDeploymentLabelKey: webhookDeploymentLabelValue,
	}
}

func WebhookService(ns string, m mondoov1alpha2.MondooAuditConfig) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webhookServiceName(m.Name),
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       int32(443),
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(webhook.DefaultPort),
				},
			},
			Type:     corev1.ServiceTypeClusterIP,
			Selector: WebhookDeploymentLabels(),
		},
	}
}

func webhookServiceName(prefix string) string {
	return prefix + "-webhook-service"
}

func webhookDeploymentName(prefix string) string {
	return prefix + "-webhook-manager"
}

func validatingWebhookName(mondooAuditConfig *mondoov1alpha2.MondooAuditConfig) (string, error) {
	if mondooAuditConfig == nil {
		return "", fmt.Errorf("cannot generate webhook name from nil MondooAuditConfig")
	}
	return fmt.Sprintf("%s-%s-mondoo", mondooAuditConfig.Namespace, mondooAuditConfig.Name), nil
}
