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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certmanagerrefv1 "github.com/jetstack/cert-manager/pkg/apis/meta/v1"

	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
)

var webhookLog = ctrl.Log.WithName("webhook")

const (
	webhookTLSSecretName = "webhook-server-cert"

	certManagerCertificateName = "webhook-serving-cert"
	certManagerIssuerName      = "mondoo-operator-selfsigned-issuer"
	certManagerAnnotationKey   = "cert-manager.io/inject-ca-from"
)

// Embed the Service/Desployment/ValidatingWebhookConfiguration payloads
//go:embed webhook-manifests.yaml
var webhookManifestsyaml []byte

type Webhooks struct {
	Enable          bool
	Mondoo          *mondoov1alpha1.MondooAuditConfig
	KubeClient      client.Client
	TargetNamespace string
	Scheme          *runtime.Scheme
	ForceCleanup    bool
}

// syncCertManagerIssuer will create/update Issuer resource for cert-manager
func (n *Webhooks) syncCertManagerIssuer(ctx context.Context) error {
	issuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certManagerIssuerName,
			Namespace: n.TargetNamespace,
		},
	}

	if err := n.setControllerRef(issuer); err != nil {
		return err
	}

	issuerSpec := certmanagerv1.IssuerSpec{
		IssuerConfig: certmanagerv1.IssuerConfig{
			SelfSigned: &certmanagerv1.SelfSignedIssuer{},
		},
	}

	if err := n.KubeClient.Get(ctx, client.ObjectKeyFromObject(issuer), issuer); err != nil {
		if errors.IsNotFound(err) {
			issuer.Spec = issuerSpec
			if err := n.KubeClient.Create(ctx, issuer); err != nil {
				webhookLog.Error(err, "Failed to create cert-manager Issuer resource")
				return err
			}
			// Creation succeeded
			return nil
		} else {
			webhookLog.Error(err, "Failed to check for existing cert-manager Issuer resource")
			return err
		}
	}

	if !reflect.DeepEqual(issuer.Spec, issuerSpec) {
		issuer.Spec = issuerSpec
		if err := n.KubeClient.Update(ctx, issuer); err != nil {
			webhookLog.Error(err, "Failed to update existing cert-manager Issuer resource")
			return err
		}
	}

	return nil
}

// syncCertManagerCertificate will create/update the cert-manager Certificate resource
func (n *Webhooks) syncCertManagerCertificate(ctx context.Context) error {

	certificate := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certManagerCertificateName,
			Namespace: n.TargetNamespace,
		},
	}

	if err := n.setControllerRef(certificate); err != nil {
		return err
	}

	certificateSpec := certmanagerv1.CertificateSpec{
		DNSNames: []string{
			fmt.Sprintf("mondoo-operator-webhook-service.%s.svc", n.TargetNamespace),
			fmt.Sprintf("mondoo-operator-webhook-service.%s.svc.cluster.local", n.TargetNamespace),
		},
		IssuerRef: certmanagerrefv1.ObjectReference{
			Kind: "Issuer",
			Name: certManagerIssuerName,
		},
		SecretName: webhookTLSSecretName,
	}

	if err := n.KubeClient.Get(ctx, client.ObjectKeyFromObject(certificate), certificate); err != nil {
		if errors.IsNotFound(err) {
			certificate.Spec = certificateSpec
			if err := n.KubeClient.Create(ctx, certificate); err != nil {
				webhookLog.Error(err, "Failed to create cert-manager Certificate resource")
				return err
			}
			// Creation succeeded
			return nil
		} else {
			webhookLog.Error(err, "Failed to check for existing cert-manager Certificate resource")
			return err
		}
	}

	if !reflect.DeepEqual(certificate.Spec, certificateSpec) {
		certificate.Spec = certificateSpec
		if err := n.KubeClient.Update(ctx, certificate); err != nil {
			webhookLog.Error(err, "Failed to update existing cert-manager Certificate resource")
			return err
		}
	}

	return nil
}

// syncValidatingWebhookConfiguration will create/update the ValidatingWebhookConfiguration
func (n *Webhooks) syncValidatingWebhookConfiguration(ctx context.Context,
	vwc *webhooksv1.ValidatingWebhookConfiguration,
	annotationKey, annotationValue string) error {

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

	if !reflect.DeepEqual(service.Spec.Ports, desiredService.Spec.Ports) || !reflect.DeepEqual(service.Spec.Selector, desiredService.Spec.Selector) {
		service.Spec.Ports = desiredService.Spec.Ports
		service.Spec.Selector = desiredService.Spec.Selector
		if err := n.KubeClient.Update(ctx, service); err != nil {
			webhookLog.Error(err, "failed to update existing webhook Service")
			return err
		}
	}

	return nil
}

func (n *Webhooks) syncWebhookDeployment(ctx context.Context, deployment *appsv1.Deployment) error {

	desiredDeployment := deployment.DeepCopy()

	deployment.SetNamespace(n.TargetNamespace)
	if err := n.setControllerRef(deployment); err != nil {
		return err
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

func (n *Webhooks) prepareValidatingWebhook(ctx context.Context, vwc *webhooksv1.ValidatingWebhookConfiguration) error {

	switch n.Mondoo.Spec.Webhooks.CertificateConfig.InjectionStyle {
	case string(mondoov1alpha1.CertManager):
		if err := n.syncCertManagerIssuer(ctx); err != nil {
			return err
		}

		if err := n.syncCertManagerCertificate(ctx); err != nil {
			return err
		}

		// format for cert-manager annotation value is namespace/nameOfCertManagerCertificate
		annotationValue := n.TargetNamespace + "/" + certManagerCertificateName

		if err := n.syncValidatingWebhookConfiguration(ctx, vwc, certManagerAnnotationKey, annotationValue); err != nil {
			return err
		}

		return nil

	default:
		// Consider this "manual" mode where the user is responsible for populating the Secret with
		// appropriate TLS certificates. Populating the Secret will unblock the Pod and allow it to run.
		// User also needs to populate the CA data on the webhook.
		// So just apply the ValidatingWebhookConfiguration as-is
		if err := n.syncValidatingWebhookConfiguration(ctx, vwc, "mondoo.com/tls-mode", "manual"); err != nil {
			return err
		}
		return nil
	}
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
	if n.Mondoo.Spec.Webhooks.Enable {
		result, err := n.applyWebhooks(ctx)
		if err != nil || result.Requeue {
			return result, err
		}
	} else {
		n.down(ctx, req)
	}
	return ctrl.Result{}, nil
}

func (n *Webhooks) down(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Check for every possible object we could have created, and delete it

	// Cleanup cert-manager Certificate and Issuer
	certificate := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certManagerCertificateName,
			Namespace: n.TargetNamespace,
		},
	}

	if err := n.genericDelete(ctx, certificate); err != nil {
		webhookLog.Error(err, "Failed to clean up cert-manager Certificate resource")
		return ctrl.Result{}, err
	}

	issuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certManagerIssuerName,
			Namespace: n.TargetNamespace,
		},
	}

	if err := n.genericDelete(ctx, issuer); err != nil {
		webhookLog.Error(err, "Failed to clean up cert-manager Issuer resource")
		return ctrl.Result{}, err
	}

	// Cleanup standard manifests that are always applied
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
		default:
			err := fmt.Errorf("Unexpected type %s to decode", gvk.Kind)
			webhookLog.Error(err, "Failed to convert type")
			return ctrl.Result{}, err
		}

		if !conversionOK {
			return ctrl.Result{}, fmt.Errorf("Failed to convert to resource")
		}

		if err := n.genericDelete(ctx, genericObject); err != nil {
			webhookLog.Error(err, "Failed to clean up resource")
			return ctrl.Result{}, err
		}

	}

	return ctrl.Result{}, nil
}

func (n *Webhooks) genericDelete(ctx context.Context, object client.Object) error {
	if err := n.KubeClient.Delete(ctx, object); err != nil {
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
