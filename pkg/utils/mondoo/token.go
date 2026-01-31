// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	jwt "github.com/golang-jwt/jwt/v4"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"sigs.k8s.io/yaml"
)

// must be set to "mondoo/ams" when making Mondoo API calls
const tokenIssuer = "mondoo/ams"

func GenerateTokenFromServiceAccount(serviceAccount mondooclient.ServiceAccountCredentials, logger logr.Logger) (string, error) {
	block, _ := pem.Decode([]byte(serviceAccount.PrivateKey))
	if block == nil {
		err := fmt.Errorf("found no PEM block in private key")
		logger.Error(err, "failed to decode service account's private key")
		return "", err
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		logger.Error(err, "failed to parse private key data")
		return "", err
	}
	switch pk := key.(type) {
	case *ecdsa.PrivateKey:
		return CreateSignedToken(pk, serviceAccount, logger)
	default:
		return "", fmt.Errorf("AuthKey must be of type ecdsa.PrivateKey")
	}
}

func CreateSignedToken(pk *ecdsa.PrivateKey, sa mondooclient.ServiceAccountCredentials, logger logr.Logger) (string, error) {
	issuedAt := time.Now().Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodES384, jwt.MapClaims{
		"sub": sa.Mrn,
		"iss": tokenIssuer,
		"iat": issuedAt,
		"exp": issuedAt + 60, // expire in 1 minute
		"nbf": issuedAt,
	})

	token.Header["kid"] = sa.Mrn

	tokenString, err := token.SignedString(pk)
	if err != nil {
		logger.Error(err, "failed to generate token")
		return "", err
	}

	return tokenString, nil
}

// LoadServiceAccountFromFile loads service account credentials from a mondoo.yml config file
func LoadServiceAccountFromFile(data []byte) (*mondooclient.ServiceAccountCredentials, error) {
	sa := &mondooclient.ServiceAccountCredentials{}
	if err := yaml.Unmarshal(data, sa); err != nil {
		return nil, fmt.Errorf("failed to unmarshal service account: %w", err)
	}

	if sa.Mrn == "" {
		return nil, fmt.Errorf("service account mrn is missing")
	}
	if sa.PrivateKey == "" {
		return nil, fmt.Errorf("service account private_key is missing")
	}
	if sa.ApiEndpoint == "" {
		return nil, fmt.Errorf("service account api_endpoint is missing")
	}

	return sa, nil
}
