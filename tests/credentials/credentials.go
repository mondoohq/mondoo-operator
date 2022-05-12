package credentials

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"
)

// MondooToken will generate a private key and return a JWT as a string
// the integrationMRN parameter can be passed in so that the resulting JWT
// models those provided by the Mondoo k8s integration
func MondooToken(t *testing.T, integrationMRN string) string {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "failed to generate private key for generating JWT")

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err, "failed to extract public key")

	hasher := crypto.SHA256.New()
	hasher.Write(publicKeyBytes)
	publicKeyHash := hasher.Sum(nil)
	keyID := base64.RawURLEncoding.EncodeToString(publicKeyHash)

	claims := jwt.MapClaims{
		"sub":          "//some/user/id",
		"aud":          []string{"mondoo"},
		"iss":          "mondoo/issuer",
		"api_endpoint": "https://some.domain.com/path/to/endpoint",
		"exp":          time.Now().Unix() + 600, // 600 seconds
		"iat":          time.Now().Unix(),
		"space":        "//some/mondoo/spaceID",
	}

	if integrationMRN != "" {
		claims["owner"] = integrationMRN
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	token.Header["kid"] = keyID

	tokenString, err := token.SignedString(privateKey)
	require.NoError(t, err, "failed to generate signed token string")

	return tokenString
}

// MondooServiceAccount will generate an elliptic curve key and return the string representation
// of the PEM-encoded private key
func MondooServiceAccount(t *testing.T) string {
	serviceAccountPrivateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err, "error while generating private key for test service account")

	x509Encoded, err := x509.MarshalPKCS8PrivateKey(serviceAccountPrivateKey)
	require.NoError(t, err, "failed to marshal generated private key")

	pemEncoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: x509Encoded})

	return string(pemEncoded)
}
