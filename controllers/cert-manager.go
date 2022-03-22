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
	"context"
	_ "embed"
	"fmt"
	"reflect"

	"go.mondoo.com/mondoo-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certmanagerrefv1 "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
)

var certManagerLog = ctrl.Log.WithName("cert-manager")

const (
	certManagerCertificateName = "webhook-serving-cert"
	certManagerIssuerName      = "mondoo-operator-selfsigned-issuer"
	certManagerAnnotationKey   = "cert-manager.io/inject-ca-from"
)

type CertManagerHandler struct {
	Mondoo          *v1alpha1.MondooAuditConfig
	KubeClient      client.Client
	TargetNamespace string
	Scheme          *runtime.Scheme
}

func (c *CertManagerHandler) Setup(ctx context.Context) (string, string, error) {
	// Modify the webhook annotation so that cert-manager mutates it with the certificate authority data.
	// The format for cert-manager annotation value is namespace/nameOfCertManagerCertificate
	annotationValue := c.TargetNamespace + "/" + certManagerCertificateName

	if err := c.syncCertManagerIssuer(ctx); err != nil {
		return certManagerAnnotationKey, annotationValue, err
	}

	if err := c.syncCertManagerCertificate(ctx); err != nil {
		return certManagerAnnotationKey, annotationValue, err
	}

	return certManagerAnnotationKey, annotationValue, nil
}

func (c *CertManagerHandler) Cleanup(ctx context.Context) error {
	// Cleanup cert-manager Certificate and Issuer
	certificate := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certManagerCertificateName,
			Namespace: c.TargetNamespace,
		},
	}

	if err := genericDelete(ctx, c.KubeClient, certificate); err != nil {
		certManagerLog.Error(err, "Failed to clean up cert-manager Certificate resource")
		return err
	}

	issuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certManagerIssuerName,
			Namespace: c.TargetNamespace,
		},
	}

	if err := genericDelete(ctx, c.KubeClient, issuer); err != nil {
		certManagerLog.Error(err, "Failed to clean up cert-manager Issuer resource")
		return err
	}

	return nil
}

// syncCertManagerIssuer will create/update Issuer resource for cert-manager
func (c *CertManagerHandler) syncCertManagerIssuer(ctx context.Context) error {
	issuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certManagerIssuerName,
			Namespace: c.TargetNamespace,
		},
	}

	if err := ctrl.SetControllerReference(c.Mondoo, issuer, c.Scheme); err != nil {
		certManagerLog.Error(err, "failed to set owner reference")
		return err
	}

	issuerSpec := certmanagerv1.IssuerSpec{
		IssuerConfig: certmanagerv1.IssuerConfig{
			SelfSigned: &certmanagerv1.SelfSignedIssuer{},
		},
	}

	if err := c.KubeClient.Get(ctx, client.ObjectKeyFromObject(issuer), issuer); err != nil {
		if errors.IsNotFound(err) {
			issuer.Spec = issuerSpec
			if err := c.KubeClient.Create(ctx, issuer); err != nil {
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
		if err := c.KubeClient.Update(ctx, issuer); err != nil {
			webhookLog.Error(err, "Failed to update existing cert-manager Issuer resource")
			return err
		}
	}

	return nil
}

// syncCertManagerCertificate will create/update the cert-manager Certificate resource
func (c *CertManagerHandler) syncCertManagerCertificate(ctx context.Context) error {

	certificate := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certManagerCertificateName,
			Namespace: c.TargetNamespace,
		},
	}

	if err := ctrl.SetControllerReference(c.Mondoo, certificate, c.Scheme); err != nil {
		certManagerLog.Error(err, "failed to set owner reference")
		return err
	}

	certificateSpec := certmanagerv1.CertificateSpec{
		DNSNames: []string{
			fmt.Sprintf("%s.%s.svc", getWebhookServiceName(c.Mondoo.Name), c.TargetNamespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", getWebhookServiceName(c.Mondoo.Name), c.TargetNamespace),
		},
		IssuerRef: certmanagerrefv1.ObjectReference{
			Kind: "Issuer",
			Name: certManagerIssuerName,
		},
		SecretName: webhookTLSSecretName,
	}

	if err := c.KubeClient.Get(ctx, client.ObjectKeyFromObject(certificate), certificate); err != nil {
		if errors.IsNotFound(err) {
			certificate.Spec = certificateSpec
			if err := c.KubeClient.Create(ctx, certificate); err != nil {
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
		if err := c.KubeClient.Update(ctx, certificate); err != nil {
			webhookLog.Error(err, "Failed to update existing cert-manager Certificate resource")
			return err
		}
	}

	return nil
}
