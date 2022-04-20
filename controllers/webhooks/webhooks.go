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

package webhooks

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"reflect"

	webhooksv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mondoo-operator/pkg/version"
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

var (
	webhookLog = ctrl.Log.WithName("webhook")

	// webhookAnnotationList is a list of all the possible annotations we could set on a Webhook.
	// It is needed so we can be sure to clear out any previous annotations in the event the
	// injection-style has changed during runtime.
	webhookAnnotationList = []string{certManagerAnnotationKey, openShiftWebhookAnnotationKey, manualTLSAnnotationKey}
)

// Embed the Service/Desployment/ValidatingWebhookConfiguration payloads
//go:embed webhook-manifests.yaml
var webhookManifestsyaml []byte

type Webhooks struct {
	Mondoo               *mondoov1alpha1.MondooAuditConfig
	KubeClient           client.Client
	TargetNamespace      string
	Image                string
	MondooOperatorConfig *mondoov1alpha1.MondooOperatorConfig
}

// syncValidatingWebhookConfiguration will create/update the ValidatingWebhookConfiguration
func (n *Webhooks) syncValidatingWebhookConfiguration(ctx context.Context,
	vwc *webhooksv1.ValidatingWebhookConfiguration,
	annotationKey, annotationValue string) error {

	// Override the default/generic name to allow for multiple MondooAudicConfig resources
	// to each have their own Webhook
	vwcName, err := getValidatingWebhookName(n.Mondoo)
	if err != nil {
		webhookLog.Error(err, "failed to generate Webhook name")
		return err
	}
	vwc.SetName(vwcName)

	// And update the Webhook entries to point to the right namespace/name for the Service
	// receiving the webhook calls
	for i := range vwc.Webhooks {
		vwc.Webhooks[i].ClientConfig.Service.Name = getWebhookServiceName(n.Mondoo.Name)
		vwc.Webhooks[i].ClientConfig.Service.Namespace = n.Mondoo.Namespace

		if vwc.Webhooks[i].ClientConfig.Service.Port == nil {
			vwc.Webhooks[i].ClientConfig.Service.Port = pointer.Int32(443)
		}
	}

	metav1.SetMetaDataAnnotation(&vwc.ObjectMeta, annotationKey, annotationValue)
	existingVWC := &webhooksv1.ValidatingWebhookConfiguration{}

	created, err := k8s.CreateIfNotExist(ctx, n.KubeClient, existingVWC, vwc)
	if err != nil {
		webhookLog.Error(err, "Failed to create ValidatingWebhookConfiguration resource")
		return err
	}

	if created {
		webhookLog.Info("ValidatingWebhookConfiguration created")
		return nil
	}

	if !deepEqualsValidatingWebhookConfiguration(existingVWC, vwc, annotationKey) {
		existingVWC.Webhooks = vwc.Webhooks

		if existingVWC.Annotations == nil {
			existingVWC.Annotations = map[string]string{}
		}
		// Clear any previously set annotations in case we have changed from one mode to another
		for _, key := range webhookAnnotationList {
			delete(existingVWC.Annotations, key)
		}

		existingVWC.Annotations[annotationKey] = annotationValue

		if err := n.KubeClient.Update(ctx, existingVWC); err != nil {
			webhookLog.Error(err, "Failed to update existing ValidatingWebhookConfiguration resource")
			return err
		}
	}

	return n.cleanupOldWebhook(ctx)
}

// For determining whether there is a meaningful difference between the existing Webhook and our desired configuration,
// just check the fields that we would care about. For the many auto-populated fields, we'll just ignore the difference between
// our desired configuration and the actual Webhook fields.
func deepEqualsValidatingWebhookConfiguration(existing, desired *webhooksv1.ValidatingWebhookConfiguration, annotationKey string) bool {

	// We always set an annotation, so if there is none set, we need to update the existing webhook
	if existing.Annotations == nil {
		return false
	}

	if existing.Annotations[annotationKey] != desired.Annotations[annotationKey] {
		return false
	}

	if len(existing.Webhooks) != len(desired.Webhooks) {
		return false
	}

	for i := range existing.Webhooks {
		if !reflect.DeepEqual(existing.Webhooks[i].ClientConfig.Service, desired.Webhooks[i].ClientConfig.Service) {
			return false
		}

		if existing.Webhooks[i].Name != desired.Webhooks[i].Name {
			return false
		}
	}

	return true
}

func (n *Webhooks) syncWebhookService(ctx context.Context) error {

	desiredService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getWebhookServiceName(n.Mondoo.Name),
			Namespace: n.TargetNamespace,
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

	// Annotate the Service if the Mondoo config is asking for OpenShift-style TLS certificate management.
	if n.Mondoo.Spec.Webhooks.CertificateConfig.InjectionStyle == string(mondoov1alpha1.OpenShift) {
		// Just set the value to the name of the Secret the webhook Deployment mounts in.
		metav1.SetMetaDataAnnotation(&desiredService.ObjectMeta, openShiftServiceAnnotationKey, GetTLSCertificatesSecretName(n.Mondoo.Name))
	}

	if err := n.setControllerRef(desiredService); err != nil {
		return err
	}

	service := &corev1.Service{}
	created, err := k8s.CreateIfNotExist(ctx, n.KubeClient, service, desiredService)
	if err != nil {
		webhookLog.Error(err, "failed to create Service for webhook")
		return err
	}

	if created {
		webhookLog.Info("Created webhook service")
		return nil
	}

	if !k8s.AreServicesEqual(*desiredService, *service) ||
		(n.Mondoo.Spec.Webhooks.CertificateConfig.InjectionStyle == string(mondoov1alpha1.OpenShift) &&
			(!metav1.HasAnnotation(service.ObjectMeta, openShiftServiceAnnotationKey) ||
				service.Annotations[openShiftServiceAnnotationKey] != webhookTLSSecretName)) {
		if n.Mondoo.Spec.Webhooks.CertificateConfig.InjectionStyle == string(mondoov1alpha1.OpenShift) {
			metav1.SetMetaDataAnnotation(&service.ObjectMeta, openShiftServiceAnnotationKey, GetTLSCertificatesSecretName(n.Mondoo.Name))
		}
		service.Spec.Ports = desiredService.Spec.Ports
		service.Spec.Selector = desiredService.Spec.Selector
		if err := n.KubeClient.Update(ctx, service); err != nil {
			webhookLog.Error(err, "failed to update existing webhook Service")
			return err
		}
	}

	return nil
}

func (n *Webhooks) syncWebhookDeployment(ctx context.Context) error {

	// "permissive" by default if Spec.Webhooks.Mode is ""
	mode := n.Mondoo.Spec.Webhooks.Mode
	if mode == "" {
		mode = string(mondoov1alpha1.Permissive)
	}

	desiredDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getWebhookDeploymentName(n.Mondoo.Name),
			Namespace: n.TargetNamespace,
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
							Env: []corev1.EnvVar{
								{
									Name:  mondoov1alpha1.WebhookModeEnvVar,
									Value: mode,
								},
							},
							Image:           n.Image,
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
									SecretName:  GetTLSCertificatesSecretName(n.Mondoo.Name),
								},
							},
						},
					},
				},
			},
		},
	}

	if err := n.setControllerRef(desiredDeployment); err != nil {
		return err
	}

	deployment := &appsv1.Deployment{}
	created, err := k8s.CreateIfNotExist(ctx, n.KubeClient, deployment, desiredDeployment)
	if err != nil {
		webhookLog.Error(err, "failed to create Deployment for webhook")
		return err
	}

	if created {
		webhookLog.Info("Created Deployment for webhook")
		return nil
	}
	updateWebhooksConditions(n.Mondoo, deployment.Status.Replicas != deployment.Status.ReadyReplicas)

	// Not a full check for whether someone has modified our Deployment, but checking for some important bits so we know
	// if an Update() is needed.
	if !k8s.AreDeploymentsEqual(*deployment, *desiredDeployment) {
		deployment.Spec = desiredDeployment.Spec
		if err := n.KubeClient.Update(ctx, deployment); err != nil {
			webhookLog.Error(err, "failed to update existing webhook Deployment")
			return err
		}
	}

	return nil
}

func (n *Webhooks) prepareValidatingWebhook(ctx context.Context, vwc *webhooksv1.ValidatingWebhookConfiguration) error {

	var annotationKey, annotationValue string

	switch n.Mondoo.Spec.Webhooks.CertificateConfig.InjectionStyle {
	case string(mondoov1alpha1.CertManager):
		cm := &CertManagerHandler{
			KubeClient:      n.KubeClient,
			TargetNamespace: n.TargetNamespace,
			Mondoo:          n.Mondoo,
			Scheme:          n.KubeClient.Scheme(),
		}

		var err error
		annotationKey, annotationValue, err = cm.Setup(ctx)
		if err != nil {
			return err
		}

	case string(mondoov1alpha1.OpenShift):
		// For OpenShift we just annotate the webhook so that the necessary CA data is injected
		// into the webhook.
		annotationKey = openShiftWebhookAnnotationKey
		annotationValue = "true"

	default:
		// Consider this "manual" mode where the user is responsible for populating the Secret with
		// appropriate TLS certificates. Populating the Secret will unblock the Pod and allow it to run.
		// User also needs to populate the CA data on the webhook.
		// So just apply the ValidatingWebhookConfiguration as-is
		annotationKey = manualTLSAnnotationKey
		annotationValue = "manual"
	}

	return n.syncValidatingWebhookConfiguration(ctx, vwc, annotationKey, annotationValue)
}

func (n *Webhooks) applyWebhooks(ctx context.Context) (ctrl.Result, error) {
	if err := n.syncWebhookService(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if err := n.syncWebhookDeployment(ctx); err != nil {
		return ctrl.Result{}, err
	}

	r := bytes.NewReader(webhookManifestsyaml)
	yamlDecoder := yamlutil.NewYAMLOrJSONDecoder(r, 4096)
	objectDecoder := scheme.Codecs.UniversalDeserializer()

	// Go through each YAML object, convert as needed to Create/Update
	for {
		// First just read a single YAML object from the list
		rawObject := runtime.RawExtension{}

		if err := yamlDecoder.Decode(&rawObject); err != nil {
			if err == io.EOF {
				break
			}
			webhookLog.Error(err, "Failed to decode object")
			return ctrl.Result{}, err
		}

		// Decode into a runtime Object
		obj, gvk, err := objectDecoder.Decode(rawObject.Raw, nil, nil)
		if err != nil {
			webhookLog.Error(err, "Failed to decode RawExtension")
			return ctrl.Result{}, err
		}

		webhookLog.Info("Decoding object", "GVK", gvk)

		// Cast the runtime to the actual type and apply the resources
		var syncErr error
		switch gvk.Kind {
		case "ValidatingWebhookConfiguration":
			vwc, ok := obj.(*webhooksv1.ValidatingWebhookConfiguration)
			if !ok {
				return ctrl.Result{}, fmt.Errorf("Failed to convert to ValidatingWebhookConfiguration")
			}
			syncErr = n.prepareValidatingWebhook(ctx, vwc)
		default:
			err := fmt.Errorf("Unexpected type %s to decode", gvk.Kind)
			webhookLog.Error(err, "Failed to convert type")
			return ctrl.Result{}, err
		}

		if syncErr != nil {
			return ctrl.Result{}, err
		}

	}
	return ctrl.Result{}, nil
}

func (n *Webhooks) Reconcile(ctx context.Context) (ctrl.Result, error) {
	if n.Mondoo.Spec.Webhooks.Enable && n.Mondoo.DeletionTimestamp == nil {
		// On a normal mondoo-operator build, the Version variable will be set at build time to match
		// the $VERSION being built (or default to the git SHA). In the event that someone did a manual
		// build of mondoo-operator and failed to set the Version variable, we will pass an empty string
		// down to resolve the image which will result in the 'latest' tag being used as a fallback.
		imageTag := version.Version
		// Allow user to override the tag if specified
		if n.Mondoo.Spec.Webhooks.Image.Tag != "" {
			imageTag = n.Mondoo.Spec.Webhooks.Image.Tag
		}
		skipResolveImage := n.MondooOperatorConfig.Spec.SkipContainerResolution
		mondooOperatorImage, err := mondoo.ResolveMondooOperatorImage(webhookLog, n.Mondoo.Spec.Webhooks.Image.Name, imageTag, skipResolveImage)
		if err != nil {
			return ctrl.Result{}, err
		}
		n.Image = mondooOperatorImage

		if err := scanapi.Deploy(ctx, n.KubeClient, n.TargetNamespace, *n.Mondoo); err != nil {
			return ctrl.Result{}, err
		}

		result, err := n.applyWebhooks(ctx)
		if err != nil || result.Requeue {
			return result, err
		}
	} else {
		cm := CertManagerHandler{
			TargetNamespace: n.TargetNamespace,
			KubeClient:      n.KubeClient,
			Mondoo:          n.Mondoo,
			Scheme:          n.KubeClient.Scheme(),
		}
		if err := cm.Cleanup(ctx); err != nil {
			return ctrl.Result{}, err
		}

		result, err := n.down(ctx)
		if err != nil || result.Requeue {
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

func (n *Webhooks) down(ctx context.Context) (ctrl.Result, error) {
	// NOTE: If we ever remove a resource that was previously deployed for a webhook,
	// we would need to add custom code here to clean it up as it will no longer be
	// included in the webhook-manifests.yaml embedded list of resources.

	// Check for every possible object we could have created, and delete it

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getWebhookServiceName(n.Mondoo.Name),
			Namespace: n.TargetNamespace,
		},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, service); err != nil {
		webhookLog.Error(err, "failed to clean up webhook Service resource")
		return ctrl.Result{}, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getWebhookDeploymentName(n.Mondoo.Name),
			Namespace: n.TargetNamespace,
		},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, deployment); err != nil {
		webhookLog.Error(err, "failed to clean up webhook Deployment resource")
		return ctrl.Result{}, err
	}

	if err := scanapi.Cleanup(ctx, n.KubeClient, n.TargetNamespace, *n.Mondoo); err != nil {
		webhookLog.Error(err, "failed to clean up scan API resources")
		return ctrl.Result{}, err
	}

	if err := n.cleanupOldWebhook(ctx); err != nil {
		return ctrl.Result{}, err
	}

	// Cleanup ValidatingWebhooks
	r := bytes.NewReader(webhookManifestsyaml)
	yamlDecoder := yamlutil.NewYAMLOrJSONDecoder(r, 4096)
	objectDecoder := scheme.Codecs.UniversalDeserializer()

	vwcName, err := getValidatingWebhookName(n.Mondoo)
	if err != nil {
		webhookLog.Error(err, "failed to generate Webhook name")
		return ctrl.Result{}, err
	}

	// Go through each YAML object, convert as needed to Delete()
	for {
		// First just read a single YAML object from the list
		rawObject := runtime.RawExtension{}

		if err := yamlDecoder.Decode(&rawObject); err != nil {
			if err == io.EOF {
				break
			}
			webhookLog.Error(err, "Failed to decode object")
			return ctrl.Result{}, err
		}

		// Decode into a runtime Object
		obj, gvk, err := objectDecoder.Decode(rawObject.Raw, nil, nil)
		if err != nil {
			webhookLog.Error(err, "Failed to decode RawExtension")
			return ctrl.Result{}, err
		}

		webhookLog.Info("Decoding object", "GVK", gvk)

		// Cast the runtime to the actual type and delete the resources
		var genericObject client.Object
		var conversionOK bool
		switch gvk.Kind {
		case "ValidatingWebhookConfiguration":
			genericObject, conversionOK = obj.(*webhooksv1.ValidatingWebhookConfiguration)
			if conversionOK {
				genericObject.SetName(vwcName)
			}
		default:
			err := fmt.Errorf("Unexpected type %s to decode", gvk.Kind)
			webhookLog.Error(err, "Failed to convert type")
			return ctrl.Result{}, err
		}

		if !conversionOK {
			return ctrl.Result{}, fmt.Errorf("Failed to convert to resource")
		}

		if err := k8s.DeleteIfExists(ctx, n.KubeClient, genericObject); err != nil {
			webhookLog.Error(err, "Failed to clean up resource")
			return ctrl.Result{}, err
		}

	}

	// Make sure to clear any degraded status
	updateWebhooksConditions(n.Mondoo, false)

	return ctrl.Result{}, nil
}

func (n *Webhooks) setControllerRef(obj client.Object) error {
	if err := ctrl.SetControllerReference(n.Mondoo, obj, n.KubeClient.Scheme()); err != nil {
		webhookLog.Error(err, "Failed to set ControllerReference", "Object", obj)
		return err
	}
	return nil
}

func getWebhookServiceName(prefix string) string {
	return prefix + "-webhook-service"
}

func getWebhookDeploymentName(prefix string) string {
	return prefix + "-webhook-manager"
}

func getValidatingWebhookName(mondooAuditConfig *mondoov1alpha1.MondooAuditConfig) (string, error) {
	if mondooAuditConfig == nil {
		return "", fmt.Errorf("cannot generate webhook name from nil MondooAuditConfig")
	}
	return fmt.Sprintf("%s-%s-mondoo", mondooAuditConfig.Namespace, mondooAuditConfig.Name), nil
}

func updateWebhooksConditions(config *mondoov1alpha1.MondooAuditConfig, degradedStatus bool) {
	msg := "Webhook is available"
	reason := "WebhookAailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	if degradedStatus {
		msg = "Webhook is Unavailable"
		reason = "WebhhookUnvailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(config.Status.Conditions, mondoov1alpha1.WebhookDegraded, status, reason, msg, updateCheck)

}

// TODO: This cleanup can be removed afer we are sure no more old-style
// ValidatingWebhooks exist.
// With the switch to naming the Webhook from MONDOOAUDIT_CONFIG_NAME-mondoo-webhook
// to NAMESPACE-NAME-mondoo , we should try to clean up any old Webhooks so that they
// are not orphaned.
func (n *Webhooks) cleanupOldWebhook(ctx context.Context) error {
	oldWebhook := &webhooksv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: n.Mondoo.Name + "-mondoo-webhook",
		},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, oldWebhook); err != nil {
		webhookLog.Error(err, "failed trying to clean up old-style webhook")
		return err
	}
	return nil
}

// GetTLSCertificatesSecretName takes the name of a MondooAuditConfig resources
// and returns the expected Secret name where the TLS certs will be stored.
func GetTLSCertificatesSecretName(mondooAuditConfigName string) string {
	// webhookTLSSecretNameTemplate is intended to store the MondooAuditConfig Name for Secret
	// name uniqueness per-Namespace.
	webhookTLSSecretNameTemplate := `%s-webhook-server-cert`

	return fmt.Sprintf(webhookTLSSecretNameTemplate, mondooAuditConfigName)
}
