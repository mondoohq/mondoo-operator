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

package admission

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

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

var (
	webhookLog = ctrl.Log.WithName("webhook")
	// Embed the Service/Desployment/ValidatingWebhookConfiguration payloads
	//go:embed webhook-manifests.yaml
	webhookManifestsyaml []byte
)

type DeploymentHandler struct {
	Mondoo                 *mondoov1alpha2.MondooAuditConfig
	KubeClient             client.Client
	TargetNamespace        string
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *mondoov1alpha2.MondooOperatorConfig
}

// syncValidatingWebhookConfiguration will create/update the ValidatingWebhookConfiguration
func (n *DeploymentHandler) syncValidatingWebhookConfiguration(ctx context.Context,
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

	return nil
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

func (n *DeploymentHandler) syncWebhookService(ctx context.Context) error {
	desiredService := WebhookService(n.TargetNamespace, *n.Mondoo)

	// Annotate the Service if the Mondoo config is asking for OpenShift-style TLS certificate management.
	if n.Mondoo.Spec.Admission.CertificateProvisioning.Mode == mondoov1alpha2.OpenShiftProvisioning {
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
		(n.Mondoo.Spec.Admission.CertificateProvisioning.Mode == mondoov1alpha2.OpenShiftProvisioning &&
			(!metav1.HasAnnotation(service.ObjectMeta, openShiftServiceAnnotationKey) ||
				service.Annotations[openShiftServiceAnnotationKey] != tlsSecretName)) {
		if n.Mondoo.Spec.Admission.CertificateProvisioning.Mode == mondoov1alpha2.OpenShiftProvisioning {
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

func (n *DeploymentHandler) syncWebhookDeployment(ctx context.Context) error {

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
		},
	}
	if err := n.KubeClient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {
		webhookLog.Error(err, "Failed to get cluster ID from kube-system Namespace")
		return err
	}
	clusterID := string(namespace.UID)

	integrationMRN, err := k8s.GetIntegrationMrnForAuditConfig(ctx, n.KubeClient, *n.Mondoo)
	if err != nil {
		webhookLog.Error(err,
			"failed to retrieve integration-mrn for MondooAuditConfig", "namespace", n.Mondoo.Namespace, "name", n.Mondoo.Name)
		return err
	}

	mondooOperatorImage, err := n.ContainerImageResolver.MondooOperatorImage(
		n.Mondoo.Spec.Admission.Image.Name, n.Mondoo.Spec.Admission.Image.Tag, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		return err
	}

	desiredDeployment := WebhookDeployment(n.TargetNamespace, mondooOperatorImage, *n.Mondoo, integrationMRN, clusterID)
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
	updateAdmissionConditions(n.Mondoo, deployment.Status.Replicas != deployment.Status.ReadyReplicas)

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

func (n *DeploymentHandler) prepareValidatingWebhook(ctx context.Context, vwc *webhooksv1.ValidatingWebhookConfiguration) error {

	var annotationKey, annotationValue string

	switch n.Mondoo.Spec.Admission.CertificateProvisioning.Mode {
	case mondoov1alpha2.CertManagerProvisioning:
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

	case mondoov1alpha2.OpenShiftProvisioning:
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

func (n *DeploymentHandler) applyWebhooks(ctx context.Context) (ctrl.Result, error) {
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
			webhookLog.Error(err, "failed to decode RawExtension")
			return ctrl.Result{}, err
		}

		webhookLog.Info("Decoding object", "GVK", gvk)

		// Cast the runtime to the actual type and apply the resources
		var syncErr error
		switch gvk.Kind {
		case "ValidatingWebhookConfiguration":
			vwc, ok := obj.(*webhooksv1.ValidatingWebhookConfiguration)
			if !ok {
				return ctrl.Result{}, fmt.Errorf("failed to convert to ValidatingWebhookConfiguration")
			}
			syncErr = n.prepareValidatingWebhook(ctx, vwc)
		default:
			err := fmt.Errorf("unexpected type %s to decode", gvk.Kind)
			webhookLog.Error(err, "Failed to convert type")
			return ctrl.Result{}, err
		}

		if syncErr != nil {
			return ctrl.Result{}, err
		}

	}
	return ctrl.Result{}, nil
}

func (n *DeploymentHandler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	if n.Mondoo.Spec.Admission.Enable && n.Mondoo.DeletionTimestamp.IsZero() {
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

func (n *DeploymentHandler) down(ctx context.Context) (ctrl.Result, error) {
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
			webhookLog.Error(err, "failed to decode object")
			return ctrl.Result{}, err
		}

		// Decode into a runtime Object
		obj, gvk, err := objectDecoder.Decode(rawObject.Raw, nil, nil)
		if err != nil {
			webhookLog.Error(err, "failed to decode RawExtension")
			return ctrl.Result{}, err
		}

		webhookLog.Info("decoding object", "GVK", gvk)

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
			err := fmt.Errorf("unexpected type %s to decode", gvk.Kind)
			webhookLog.Error(err, "Failed to convert type")
			return ctrl.Result{}, err
		}

		if !conversionOK {
			return ctrl.Result{}, fmt.Errorf("failed to convert to resource")
		}

		if err := k8s.DeleteIfExists(ctx, n.KubeClient, genericObject); err != nil {
			webhookLog.Error(err, "failed to clean up resource")
			return ctrl.Result{}, err
		}

	}

	// Make sure to clear any degraded status
	updateAdmissionConditions(n.Mondoo, false)

	return ctrl.Result{}, nil
}

func (n *DeploymentHandler) setControllerRef(obj client.Object) error {
	if err := ctrl.SetControllerReference(n.Mondoo, obj, n.KubeClient.Scheme()); err != nil {
		webhookLog.Error(err, "failed to set ControllerReference", "Object", obj)
		return err
	}
	return nil
}
