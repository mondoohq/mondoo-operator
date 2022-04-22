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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mondoo-operator/pkg/version"
)

var (
	webhookLog = ctrl.Log.WithName("webhook")
	// Embed the Service/Desployment/ValidatingWebhookConfiguration payloads
	//go:embed webhook-manifests.yaml
	webhookManifestsyaml []byte
)

type Webhooks struct {
	Mondoo               *mondoov1alpha1.MondooAuditConfig
	KubeClient           client.Client
	TargetNamespace      string
	OperatorImage        string
	ClientImage          string
	MondooOperatorConfig *mondoov1alpha1.MondooOperatorConfig
}

// syncValidatingWebhookConfiguration will create/update the ValidatingWebhookConfiguration
func (n *Webhooks) syncValidatingWebhookConfiguration(ctx context.Context,
	vwc *webhooksv1.ValidatingWebhookConfiguration,
	annotationKey, annotationValue string) error {

	// Override the default/generic name to allow for multiple MondooAudicConfig resources
	// to each have their own Webhook
	vwcName, err := validatingWebhookName(n.Mondoo)
	if err != nil {
		webhookLog.Error(err, "failed to generate Webhook name")
		return err
	}
	vwc.SetName(vwcName)

	// And update the Webhook entries to point to the right namespace/name for the Service
	// receiving the webhook calls
	for i := range vwc.Webhooks {
		vwc.Webhooks[i].ClientConfig.Service.Name = webhookServiceName(n.Mondoo.Name)
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
	desiredService := WebhookService(n.TargetNamespace, *n.Mondoo)

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

	tlsSecretName := GetTLSCertificatesSecretName(n.Mondoo.Name)
	if !k8s.AreServicesEqual(*desiredService, *service) ||
		(n.Mondoo.Spec.Webhooks.CertificateConfig.InjectionStyle == string(mondoov1alpha1.OpenShift) &&
			(!metav1.HasAnnotation(service.ObjectMeta, openShiftServiceAnnotationKey) ||
				service.Annotations[openShiftServiceAnnotationKey] != tlsSecretName)) {
		if n.Mondoo.Spec.Webhooks.CertificateConfig.InjectionStyle == string(mondoov1alpha1.OpenShift) {
			metav1.SetMetaDataAnnotation(&service.ObjectMeta, openShiftServiceAnnotationKey, tlsSecretName)
		}
		k8s.UpdateService(service, *desiredService)
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

	desiredDeployment := WebhookDeployment(n.TargetNamespace, n.OperatorImage, mode, *n.Mondoo)
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
		k8s.UpdateDeployment(deployment, *desiredDeployment)
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
		n.OperatorImage = mondooOperatorImage

		mondooImage, err := mondoo.ResolveMondooImage(webhookLog, "", "", skipResolveImage)
		if err != nil {
			return ctrl.Result{}, err
		}
		n.ClientImage = mondooImage

		if err := scanapi.Deploy(ctx, n.KubeClient, n.TargetNamespace, n.ClientImage, *n.Mondoo); err != nil {
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
			Name:      webhookServiceName(n.Mondoo.Name),
			Namespace: n.TargetNamespace,
		},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, service); err != nil {
		webhookLog.Error(err, "failed to clean up webhook Service resource")
		return ctrl.Result{}, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webhookDeploymentName(n.Mondoo.Name),
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

	vwcName, err := validatingWebhookName(n.Mondoo)
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
