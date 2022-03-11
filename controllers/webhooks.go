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
	"os"
	"reflect"
	"strings"

	webhooksv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
)

var webhookLog = ctrl.Log.WithName("webhook")

const (
	webhookTLSSecretName = "webhook-server-cert"
	// Keep a field to allow inserting the namespace so that we can have
	// multiple MondooAuditConfig resources each with their own webhook registered.
	webhookNameTemplate = `%s-mondoo-webhook`

	// openShiftServiceAnnotationKey is how we annotate a Service so that OpenShift
	// will create TLS certificates for the webhook Service.
	openShiftServiceAnnotationKey = "service.beta.openshift.io/serving-cert-secret-name"

	// openShiftWebhookAnnotationKey is how we annotate a webhook so that OpenShift
	// injects the cluster-wide CA data for auto-generated TLS certificates.
	openShiftWebhookAnnotationKey = "service.beta.openshift.io/inject-cabundle"

	mondooOperatorImageEnvVar = "MONDOO_OPERATOR_IMAGE"
)

// Embed the Service/Desployment/ValidatingWebhookConfiguration payloads
//go:embed webhook-manifests.yaml
var webhookManifestsyaml []byte

type Webhooks struct {
	Mondoo          *mondoov1alpha1.MondooAuditConfig
	KubeClient      client.Client
	TargetNamespace string
	Scheme          *runtime.Scheme
}

// syncValidatingWebhookConfiguration will create/update the ValidatingWebhookConfiguration
func (n *Webhooks) syncValidatingWebhookConfiguration(ctx context.Context,
	vwc *webhooksv1.ValidatingWebhookConfiguration,
	annotationKey, annotationValue string) error {

	// Override the default name to allow for multiple MondooAudicConfig resources
	// to each have their own Webhook
	vwc.Name = fmt.Sprintf(webhookNameTemplate, n.Mondoo.Namespace)

	// And update the Webhook entries to point to the right namespace for the Service
	// receiving the webhook calls
	for i := range vwc.Webhooks {
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

func (n *Webhooks) syncWebhookService(ctx context.Context, service *corev1.Service) error {

	desiredService := service.DeepCopy()

	// Annotate the Service if the Mondo config is asking for OpenShift-style TLS certificate management.
	if n.Mondoo.Spec.Webhooks.CertificateConfig.InjectionStyle == string(mondoov1alpha1.OpenShift) {
		if service.Annotations == nil {
			service.Annotations = map[string]string{}
		}

		// Just set the value to the name of the Secret the webhook Deployment mounts in.
		service.Annotations[openShiftServiceAnnotationKey] = webhookTLSSecretName
	}

	service.SetNamespace(n.TargetNamespace)

	if err := n.setControllerRef(service); err != nil {
		return err
	}

	if err := n.KubeClient.Get(ctx, client.ObjectKeyFromObject(service), service); err != nil {
		if errors.IsNotFound(err) {
			if err := n.KubeClient.Create(ctx, service); err != nil {
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

func (n *Webhooks) syncWebhookDeployment(ctx context.Context, deployment *appsv1.Deployment) error {

	desiredDeployment := deployment.DeepCopy()

	deployment.SetNamespace(n.TargetNamespace)
	if err := n.setControllerRef(deployment); err != nil {
		return err
	}

	webhookImage, exists := os.LookupEnv(mondooOperatorImageEnvVar)
	if exists {
		setWebhookImage(deployment, webhookImage)
	}

	if err := n.KubeClient.Get(ctx, client.ObjectKeyFromObject(deployment), deployment); err != nil {
		if errors.IsNotFound(err) {
			if err := n.KubeClient.Create(ctx, deployment); err != nil {
				webhookLog.Error(err, "failed to create Deployment for webhook")
				return err
			}
			return nil
		} else {
			webhookLog.Error(err, "failed to check for existing webhook Deployment")
		}
	}

	// Not a full check for whether someone has modified our Deployment, but checking for some important bits so we know
	// if an Update() is needed.
	if len(deployment.Spec.Template.Spec.Containers) != len(desiredDeployment.Spec.Template.Spec.Containers) ||
		!reflect.DeepEqual(deployment.Spec.Replicas, desiredDeployment.Spec.Replicas) ||
		!reflect.DeepEqual(deployment.Spec.Selector, desiredDeployment.Spec.Selector) ||
		!reflect.DeepEqual(deployment.Spec.Template.Spec.Containers[0].Image, desiredDeployment.Spec.Template.Spec.Containers[0].Image) ||
		!reflect.DeepEqual(deployment.Spec.Template.Spec.Containers[0].Command, desiredDeployment.Spec.Template.Spec.Containers[0].Command) ||
		!reflect.DeepEqual(deployment.Spec.Template.Spec.Containers[0].VolumeMounts, desiredDeployment.Spec.Template.Spec.Containers[0].VolumeMounts) {
		deployment.Spec = desiredDeployment.Spec
		if err := n.KubeClient.Update(ctx, deployment); err != nil {
			webhookLog.Error(err, "failed to update existing webhook Deployment")
			return err
		}
	}

	return nil

}

// setWebhookImage will set the MONDOO_OPERATOR_IMAGE environment variable on all
// containers in the Deployment.
func setWebhookImage(deployment *appsv1.Deployment, image string) {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		setEnvVar := false

		for i := range container.Env {
			if container.Env[i].Name == mondooOperatorImageEnvVar {
				container.Env[i].Value = image
				setEnvVar = true
			}
		}

		if !setEnvVar {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  mondooOperatorImageEnvVar,
				Value: image,
			})
		}
	}
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
		case "Service":
			service, ok := obj.(*corev1.Service)
			if !ok {
				return ctrl.Result{}, fmt.Errorf("Failed to convert to Service")
			}
			syncErr = n.syncWebhookService(ctx, service)
		case "Deployment":
			deployment, ok := obj.(*appsv1.Deployment)
			if !ok {
				return ctrl.Result{}, fmt.Errorf("Failed to convert to Deployment")
			}
			syncErr = n.syncWebhookDeployment(ctx, deployment)
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

	// Cleanup standard manifests that are always applied
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
		case "Service":
			genericObject, conversionOK = obj.(*corev1.Service)
			genericObject.SetNamespace(n.TargetNamespace)
		case "Deployment":
			genericObject, conversionOK = obj.(*appsv1.Deployment)
			genericObject.SetNamespace(n.TargetNamespace)
		case "ValidatingWebhookConfiguration":
			genericObject, conversionOK = obj.(*webhooksv1.ValidatingWebhookConfiguration)
			// Generate the namespace-specific name for the webhook that needs to be cleaned up
			genericObject.SetName(fmt.Sprintf(webhookNameTemplate, n.Mondoo.Namespace))
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
