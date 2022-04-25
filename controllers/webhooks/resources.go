package webhooks

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
)

const (
	WebhookLabelKey   = "control-plane"
	WebhookLabelValue = "webhook-manager"

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
	webhookTLSSecretNameTemplate := `%s-webhook-server-cert`

	return fmt.Sprintf(webhookTLSSecretNameTemplate, mondooAuditConfigName)
}

func WebhookDeployment(ns, image string, mode mondoov1alpha2.AdmissionMode, m mondoov1alpha2.MondooAuditConfig, clusterID string) *appsv1.Deployment {
	// The URL to communicate with will be http://ScanAPIServiceName-ScanAPIServiceNamespace.svc:ScanAPIPort
	scanAPIURL := fmt.Sprintf("http://%s.%s.svc:%d", scanapi.ServiceName(m.Name), m.Namespace, scanapi.Port)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webhookDeploymentName(m.Name),
			Namespace: ns,
			Labels: map[string]string{
				WebhookLabelKey: WebhookLabelValue,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					WebhookLabelKey: WebhookLabelValue,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						WebhookLabelKey: WebhookLabelValue,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Command: []string{
								"/webhook",
							},
							Args: []string{
								"--token-file-path",
								"/etc/webhook/token",
								"--enforcement-mode",
								string(mode),
								"--scan-api-url",
								scanAPIURL,
								"--cluster-id",
								clusterID,
							},
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(8081),
									},
								},
								InitialDelaySeconds: int32(15),
								PeriodSeconds:       int32(20),
							},
							Name: "webhook",
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.FromInt(8081),
									},
								},
								InitialDelaySeconds: int32(5),
								PeriodSeconds:       int32(10),
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("100Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("20Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: pointer.Bool(false),
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
									MountPath: "/etc/webhook",
									ReadOnly:  true,
								},
							},
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: pointer.Bool(true),
					},
					TerminationGracePeriodSeconds: pointer.Int64(10),
					Volumes: []corev1.Volume{
						{
							Name: "cert",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									DefaultMode: pointer.Int32(420),
									SecretName:  GetTLSCertificatesSecretName(m.Name),
								},
							},
						},
						{
							Name: "token",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									DefaultMode: pointer.Int32(0444),
									SecretName:  scanapi.SecretName(m.Name),
								},
							},
						},
					},
				},
			},
		},
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
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				WebhookLabelKey: WebhookLabelValue,
			},
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
