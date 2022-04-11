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

package controllers

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"reflect"
	"strings"

	webhooksv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
)

var (
	webhookLog = ctrl.Log.WithName("webhook")

	Version string
)

const (
	webhookLabelKey   = "control-plane"
	webhookLabelValue = "webhook-manager"

	webhookTLSSecretName = "webhook-server-cert"

	// openShiftServiceAnnotationKey is how we annotate a Service so that OpenShift
	// will create TLS certificates for the webhook Service.
	openShiftServiceAnnotationKey = "service.beta.openshift.io/serving-cert-secret-name"

	// openShiftWebhookAnnotationKey is how we annotate a webhook so that OpenShift
	// injects the cluster-wide CA data for auto-generated TLS certificates.
	openShiftWebhookAnnotationKey = "service.beta.openshift.io/inject-cabundle"
)

// Embed the Service/Desployment/ValidatingWebhookConfiguration payloads
//go:embed webhook-manifests.yaml
var webhookManifestsyaml []byte

type Webhooks struct {
	Mondoo               *mondoov1alpha1.MondooAuditConfig
	KubeClient           client.Client
	TargetNamespace      string
	Scheme               *runtime.Scheme
	Image                string
	MondooOperatorConfig *mondoov1alpha1.MondooOperatorConfig
}

// syncValidatingWebhookConfiguration will create/update the ValidatingWebhookConfiguration
func (n *Webhooks) syncValidatingWebhookConfiguration(ctx context.Context,
	vwc *webhooksv1.ValidatingWebhookConfiguration,
	annotationKey, annotationValue string) error {

	// Override the default name to allow for multiple MondooAudicConfig resources
	// to each have their own Webhook
	vwc.Name = getValidatingWebhookName(n.Mondoo.Name)

	// And update the Webhook entries to point to the right namespace/name for the Service
	// receiving the webhook calls
	for i := range vwc.Webhooks {
		vwc.Webhooks[i].ClientConfig.Service.Name = getWebhookServiceName(n.Mondoo.Name)
		vwc.Webhooks[i].ClientConfig.Service.Namespace = n.Mondoo.Namespace
	}

	existingVWC := &webhooksv1.ValidatingWebhookConfiguration{}

	if err := n.KubeClient.Get(ctx, client.ObjectKeyFromObject(vwc), existingVWC); err != nil {
		if errors.IsNotFound(err) {
			if vwc.Annotations == nil {
				vwc.Annotations = map[string]string{}
			}
			vwc.Annotations[annotationKey] = annotationValue
			if err := n.KubeClient.Create(ctx, vwc); err != nil {
				webhookLog.Error(err, "Failed to create ValidatingWebhookConfiguration resource")
				return err
			}
			// Creation succeeded
			return nil
		} else {
			webhookLog.Error(err, "Failed to check for existing ValidatingWebhookConfiguration resource")
			return err
		}
	}

	// Doing a DeepEquals comparison of the Webhook is a nightmare. For example: many fields are auto-populated which differ from our
	// "vanilla" version we generate, and the cert-manager will inject the CA data. The sizes of arrays will be different as well...
	// So lets just make sure at least the annotation is set and update the object based on that information.
	// If you want to "force" an update, just change the annotation.
	if existingVWC.Annotations == nil || existingVWC.Annotations[annotationKey] != annotationValue {
		existingVWC.Webhooks = vwc.Webhooks

		if existingVWC.Annotations == nil {
			existingVWC.Annotations = map[string]string{}
		}
		existingVWC.Annotations[annotationKey] = annotationValue

		if err := n.KubeClient.Update(ctx, existingVWC); err != nil {
			webhookLog.Error(err, "Failed to update existing cert-manager Certificate resource")
			return err
		}
	}

	return nil
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
			Selector: map[string]string{
				webhookLabelKey: webhookLabelValue,
			},
		},
	}

	// Annotate the Service if the Mondo config is asking for OpenShift-style TLS certificate management.
	if n.Mondoo.Spec.Webhooks.CertificateConfig.InjectionStyle == string(mondoov1alpha1.OpenShift) {
		if desiredService.Annotations == nil {
			desiredService.Annotations = map[string]string{}
		}

		// Just set the value to the name of the Secret the webhook Deployment mounts in.
		desiredService.Annotations[openShiftServiceAnnotationKey] = webhookTLSSecretName
	}

	if err := n.setControllerRef(desiredService); err != nil {
		return err
	}

	service := &corev1.Service{}
	if err := n.KubeClient.Get(ctx, client.ObjectKeyFromObject(desiredService), service); err != nil {
		if errors.IsNotFound(err) {
			if err := n.KubeClient.Create(ctx, desiredService); err != nil {
				webhookLog.Error(err, "failed to create Service for webhook")
				return err
			}
			return nil
		} else {
			webhookLog.Error(err, "failed to check for existing webhook Service")
		}
	}

	if n.webhookServiceNeedsUpdate(desiredService, service) {
		if n.Mondoo.Spec.Webhooks.CertificateConfig.InjectionStyle == string(mondoov1alpha1.OpenShift) {
			if service.Annotations == nil {
				service.Annotations = map[string]string{}
			}
			service.Annotations[openShiftServiceAnnotationKey] = webhookTLSSecretName
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

func (n *Webhooks) webhookServiceNeedsUpdate(desired, existing *corev1.Service) bool {
	if !reflect.DeepEqual(desired.Spec.Ports, existing.Spec.Ports) {
		return true
	}
	if !reflect.DeepEqual(desired.Spec.Selector, existing.Spec.Selector) {
		return true
	}

	if n.Mondoo.Spec.Webhooks.CertificateConfig.InjectionStyle == string(mondoov1alpha1.OpenShift) {
		if existing.Annotations == nil || existing.Annotations[openShiftServiceAnnotationKey] != webhookTLSSecretName {
			return true
		}
	}

	return false
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
				webhookLabelKey: webhookLabelValue,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					webhookLabelKey: webhookLabelValue,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						webhookLabelKey: webhookLabelValue,
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
									SecretName:  webhookTLSSecretName,
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
	if err := n.KubeClient.Get(ctx, client.ObjectKeyFromObject(desiredDeployment), deployment); err != nil {
		if errors.IsNotFound(err) {
			if err := n.KubeClient.Create(ctx, desiredDeployment); err != nil {
				webhookLog.Error(err, "failed to create Deployment for webhook")
				return err
			}
			return nil
		} else {
			webhookLog.Error(err, "failed to check for existing webhook Deployment")
		}
	}
	updateWebhooksConditions(n.Mondoo, deployment.Status.Replicas != deployment.Status.ReadyReplicas)

	// Not a full check for whether someone has modified our Deployment, but checking for some important bits so we know
	// if an Update() is needed.
	if len(deployment.Spec.Template.Spec.Containers) != len(desiredDeployment.Spec.Template.Spec.Containers) ||
		!reflect.DeepEqual(deployment.Spec.Replicas, desiredDeployment.Spec.Replicas) ||
		!reflect.DeepEqual(deployment.Spec.Selector, desiredDeployment.Spec.Selector) ||
		!reflect.DeepEqual(deployment.Spec.Template.Spec.Containers[0].Image, desiredDeployment.Spec.Template.Spec.Containers[0].Image) ||
		!reflect.DeepEqual(deployment.Spec.Template.Spec.Containers[0].Command, desiredDeployment.Spec.Template.Spec.Containers[0].Command) ||
		!reflect.DeepEqual(deployment.Spec.Template.Spec.Containers[0].VolumeMounts, desiredDeployment.Spec.Template.Spec.Containers[0].VolumeMounts) ||
		!reflect.DeepEqual(deployment.Spec.Template.Spec.Containers[0].Env, desiredDeployment.Spec.Template.Spec.Containers[0].Env) {
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
			Scheme:          n.Scheme,
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
		annotationKey = "mondoo.com/tls-mode"
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
		imageTag := Version
		// Allow user to override the tag if specified
		if n.Mondoo.Spec.Webhooks.Image.Tag != "" {
			imageTag = n.Mondoo.Spec.Webhooks.Image.Tag
		}
		skipResolveImage := n.MondooOperatorConfig.Spec.SkipContainerResolution
		mondooOperatorImage, err := resolveMondooOperatorImage(webhookLog, n.Mondoo.Spec.Webhooks.Image.Name, imageTag, skipResolveImage)
		if err != nil {
			return ctrl.Result{}, err
		}
		n.Image = mondooOperatorImage
		result, err := n.applyWebhooks(ctx)
		if err != nil || result.Requeue {
			return result, err
		}
	} else {
		cm := CertManagerHandler{
			TargetNamespace: n.TargetNamespace,
			KubeClient:      n.KubeClient,
			Mondoo:          n.Mondoo,
			Scheme:          n.Scheme,
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
	if err := genericDelete(ctx, n.KubeClient, service); err != nil {
		webhookLog.Error(err, "failed to clean up webhook Service resource")
		return ctrl.Result{}, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getWebhookDeploymentName(n.Mondoo.Name),
			Namespace: n.TargetNamespace,
		},
	}
	if err := genericDelete(ctx, n.KubeClient, deployment); err != nil {
		webhookLog.Error(err, "failed to clean up webhook Deployment resource")
		return ctrl.Result{}, err
	}

	// Cleanup ValidatingWebhooks
	r := bytes.NewReader(webhookManifestsyaml)
	yamlDecoder := yamlutil.NewYAMLOrJSONDecoder(r, 4096)
	objectDecoder := scheme.Codecs.UniversalDeserializer()

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
			genericObject.SetName(getValidatingWebhookName(n.Mondoo.Name))
		default:
			err := fmt.Errorf("Unexpected type %s to decode", gvk.Kind)
			webhookLog.Error(err, "Failed to convert type")
			return ctrl.Result{}, err
		}

		if !conversionOK {
			return ctrl.Result{}, fmt.Errorf("Failed to convert to resource")
		}

		if err := genericDelete(ctx, n.KubeClient, genericObject); err != nil {
			webhookLog.Error(err, "Failed to clean up resource")
			return ctrl.Result{}, err
		}

	}

	// Make sure to clear any degraded status
	updateWebhooksConditions(n.Mondoo, false)

	return ctrl.Result{}, nil
}

func genericDelete(ctx context.Context, kubeClient client.Client, object client.Object) error {
	if err := kubeClient.Delete(ctx, object); err != nil {
		if errors.IsNotFound(err) || strings.Contains(err.Error(), "no matches for kind") {
			return nil
		}
		return err
	}
	return nil
}

func (n *Webhooks) setControllerRef(obj client.Object) error {
	if err := ctrl.SetControllerReference(n.Mondoo, obj, n.Scheme); err != nil {
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

func getValidatingWebhookName(prefix string) string {
	return prefix + "-mondoo-webhook"
}

func updateWebhooksConditions(config *mondoov1alpha1.MondooAuditConfig, degradedStatus bool) {
	msg := "Webhook is available"
	reason := "WebhookAailable"
	status := corev1.ConditionFalse
	updateCheck := UpdateConditionIfReasonOrMessageChange
	if degradedStatus {
		msg = "Webhook is Unavailable"
		reason = "WebhhookUnvailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = SetMondooAuditCondition(config.Status.Conditions, mondoov1alpha1.WebhookDegraded, status, reason, msg, updateCheck)

}
