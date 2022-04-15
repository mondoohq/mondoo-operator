package utils

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"github.com/stretchr/testify/suite"
)

// Webhook generation approach from https://gist.github.com/velotiotech/2e0cfd15043513d253cad7c9126d2026#file-initcontainer_main-go

// GenerateWebhookCerts will return the CA certificate, the Server certificate, and the Server private key
func GenerateWebhookCerts(s suite.Suite, dnsNames []string) (*bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2222),
		Subject: pkix.Name{
			Organization: []string{"Mondoo Operator E2E CA"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(0, 0, 5),
		IsCA:      true,
		//ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	s.NoError(err, "Failed to generate private key for CA")

	caCert, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	s.NoError(err, "Failed to create self-sign certificate for CA")

	pemEncodedCA := new(bytes.Buffer)
	s.NoError(pem.Encode(pemEncodedCA, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCert,
	}))

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
	s.NoError(err, "Failed to generate private key for webhook Service")

	serverCertBytes, err := x509.CreateCertificate(rand.Reader, serverCert, ca, &serverPrivKey.PublicKey, caPrivKey)
	s.NoError(err, "Failed to sign webhook server certificate")

	pemEncodedServerCert := new(bytes.Buffer)
	s.NoError(pem.Encode(pemEncodedServerCert, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverCertBytes,
	}))

	pemEncodedServerPrivKey := new(bytes.Buffer)
	s.NoError(pem.Encode(pemEncodedServerPrivKey, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverPrivKey),
	}))

	return pemEncodedCA, pemEncodedServerCert, pemEncodedServerPrivKey

}
