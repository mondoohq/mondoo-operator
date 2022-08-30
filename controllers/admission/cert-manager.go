/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package admission

import (
	"context"
	_ "embed"
	"fmt"
	"reflect"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
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
	Mondoo          *v1alpha2.MondooAuditConfig
	KubeClient      client.Client
	TargetNamespace string
	Scheme          *runtime.Scheme
}

func (c *CertManagerHandler) Setup(ctx context.Context) error {
	if err := c.syncCertManagerIssuer(ctx); err != nil {
		return err
	}

	if err := c.syncCertManagerCertificate(ctx); err != nil {
		return err
	}

	return nil
}

func (c *CertManagerHandler) GetAnnotations() (string, string) {
	// Modify the webhook annotation so that cert-manager mutates it with the certificate authority data.
	// The format for cert-manager annotation value is namespace/nameOfCertManagerCertificate
	annotationValue := c.TargetNamespace + "/" + certManagerCertificateName

	return certManagerAnnotationKey, annotationValue
}

func (c *CertManagerHandler) Cleanup(ctx context.Context) error {
	// Cleanup cert-manager Certificate and Issuer
	certificate := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certManagerCertificateName,
			Namespace: c.TargetNamespace,
		},
	}

	if err := k8s.DeleteIfExists(ctx, c.KubeClient, certificate); err != nil {
		certManagerLog.Error(err, "Failed to clean up cert-manager Certificate resource")
		return err
	}

	issuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certManagerIssuerName,
			Namespace: c.TargetNamespace,
		},
	}

	if err := k8s.DeleteIfExists(ctx, c.KubeClient, issuer); err != nil {
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
			fmt.Sprintf("%s.%s.svc", webhookServiceName(c.Mondoo.Name), c.TargetNamespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", webhookServiceName(c.Mondoo.Name), c.TargetNamespace),
		},
		IssuerRef: certmanagerrefv1.ObjectReference{
			Kind: "Issuer",
			Name: certManagerIssuerName,
		},
		SecretName: GetTLSCertificatesSecretName(c.Mondoo.Name),
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
