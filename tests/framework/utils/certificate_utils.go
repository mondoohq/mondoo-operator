// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package utils

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// Certificate generation approach from https://gist.github.com/velotiotech/2e0cfd15043513d253cad7c9126d2026#file-initcontainer_main-go

// GenerateTLSCerts will return create a CA and return the CA certificate, the Server certificate, and the Server private key
// for the provided list of dnsNames
func GenerateTLSCerts(dnsNames []string) (*bytes.Buffer, *bytes.Buffer, *bytes.Buffer, error) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2222),
		Subject: pkix.Name{
			Organization: []string{"Mondoo Operator E2E CA"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, 5),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate private key for CA: %s", err)
	}

	caCert, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create self-signed certificate for CA")
	}

	pemEncodedCA := new(bytes.Buffer)
	if err := pem.Encode(pemEncodedCA, &pem.Block{Type: "CERTIFICATE", Bytes: caCert}); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to encode CA certificate: %s", err)
	}

	serverCert := &x509.Certificate{
		DNSNames:     dnsNames,
		SerialNumber: big.NewInt(3333),
		Subject: pkix.Name{
			CommonName:   "Admission Webhook for Mondoo",
			Organization: []string{"mondoo.com"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 0, 4),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	serverPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate private key for Service: %s", err)
	}

	serverCertBytes, err := x509.CreateCertificate(rand.Reader, serverCert, ca, &serverPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to sign server certificate for Service: %s", err)
	}

	pemEncodedServerCert := new(bytes.Buffer)
	if err := pem.Encode(pemEncodedServerCert, &pem.Block{Type: "CERTIFICATE", Bytes: serverCertBytes}); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to encode certificate for Service: %s", err)
	}

	pemEncodedServerPrivKey := new(bytes.Buffer)
	if err := pem.Encode(pemEncodedServerPrivKey, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverPrivKey)}); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to encode private key for Service: %s", err)
	}

	return pemEncodedCA, pemEncodedServerCert, pemEncodedServerPrivKey, nil
}
