package controllers

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	cfake "k8s.io/client-go/kubernetes/fake"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	mockmondoo "go.mondoo.com/mondoo-operator/pkg/mondooclient/mock"
)

const (
	testMondooAuditConfigName = "mondoo-config"
	testNamespace             = "mondoo-operator"
	testMondooCredsSecretName = "mondoo-client"
	testMondooTokenSecretName = "mondoo-token'"

	testServiceAccountData = `SERVICE ACCOUNT DATA HERE`
)

var (
	testTokenData string
)

func TestTokenRegistration(t *testing.T) {

	zapLog, err := zap.NewDevelopment()
	require.NoError(t, err, "error setting up logging")
	logr := zapr.NewLogger(zapLog)

	testTokenData = testToken(t)

	tests := []struct {
		name              string
		existingObjects   []runtime.Object
		mondooAuditConfig *v1alpha2.MondooAuditConfig
		mockMondooClient  func(*gomock.Controller) *mockmondoo.MockClient
		verify            func(*testing.T, kubernetes.Interface)
		expectError       bool
	}{
		{
			name:              "generate service account from token secret",
			mondooAuditConfig: testMondooAuditConfig(),
			existingObjects: []runtime.Object{
				testTokenSecret(),
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockClient {
				mClient := mockmondoo.NewMockClient(mockCtrl)

				mClient.EXPECT().ExchangeRegistrationToken(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.ExchangeRegistrationTokenOutput{
					ServiceAccount: testServiceAccountData,
				}, nil)

				return mClient
			},
			verify: func(t *testing.T, kubeClient kubernetes.Interface) {

				tokenSecret, err := kubeClient.CoreV1().Secrets(testNamespace).Get(context.TODO(), testMondooCredsSecretName, metav1.GetOptions{})
				assert.NoError(t, err, "error getting secret that should exist")

				// Check StringData because we're using the fake client
				assert.Equal(t, testServiceAccountData, tokenSecret.StringData["config"])
			},
		},
		{
			name:              "no token, no service account",
			mondooAuditConfig: testMondooAuditConfig(),
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockClient {
				mClient := mockmondoo.NewMockClient(mockCtrl)

				return mClient
			},
			verify: func(t *testing.T, kubeClient kubernetes.Interface) {
				_, err := kubeClient.CoreV1().Secrets(testNamespace).Get(context.TODO(), testMondooCredsSecretName, metav1.GetOptions{})
				assert.True(t, errors.IsNotFound(err), "expected Mondoo creds secret to not exist")
			},
		},
		{
			name:              "already a Mondoo creds secret",
			mondooAuditConfig: testMondooAuditConfig(),
			existingObjects: func() []runtime.Object {
				objs := []runtime.Object{}

				objs = append(objs, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testMondooCredsSecretName,
						Namespace: testNamespace,
					},
					StringData: map[string]string{
						"config": "EXISTING MONDOO CONFIG",
					},
				})

				return objs
			}(),
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockClient {
				mClient := mockmondoo.NewMockClient(mockCtrl)

				return mClient
			},
			verify: func(t *testing.T, kubeClient kubernetes.Interface) {
				credsSecret, err := kubeClient.CoreV1().Secrets(testNamespace).Get(context.TODO(), testMondooCredsSecretName, metav1.GetOptions{})
				assert.NoError(t, err, "unexpected error getting pre-existing Secret")

				assert.Equal(t, "EXISTING MONDOO CONFIG", credsSecret.StringData["config"])
			},
		},
		{
			name:              "mondoo API error",
			mondooAuditConfig: testMondooAuditConfig(),
			existingObjects: []runtime.Object{
				testTokenSecret(),
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockClient {
				mClient := mockmondoo.NewMockClient(mockCtrl)

				mClient.EXPECT().ExchangeRegistrationToken(gomock.Any(), gomock.Any()).Times(1).Return(nil, fmt.Errorf("an error occurred"))

				return mClient
			},
			expectError: true,
		},
		{
			name:              "malformed JWT",
			mondooAuditConfig: testMondooAuditConfig(),
			existingObjects: func() []runtime.Object {
				objs := []runtime.Object{}
				sec := testTokenSecret()
				sec.Data["token"] = []byte("NOT JWT DATA")
				objs = append(objs, sec)
				return objs
			}(),
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockClient {
				mClient := mockmondoo.NewMockClient(mockCtrl)

				return mClient
			},
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mClient := test.mockMondooClient(mockCtrl)

			testMondooClientBuilder := func(mondooclient.ClientOptions) mondooclient.Client {
				return mClient
			}

			fakeClient := cfake.NewSimpleClientset(test.existingObjects...)

			reconciler := &MondooAuditConfigReconciler{
				MondooClientBuilder: testMondooClientBuilder,
			}

			// Act
			err := reconciler.newServiceAccountIfNeeded(context.TODO(), fakeClient, test.mondooAuditConfig, logr)

			// Assert

			if test.expectError {
				assert.Error(t, err, "expected error for test case")
			} else {
				assert.NoError(t, err)

				test.verify(t, fakeClient)
			}
		})
	}
}

func testMondooAuditConfig() *v1alpha2.MondooAuditConfig {
	return &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testMondooAuditConfigName,
			Namespace: testNamespace,
			Finalizers: []string{
				finalizerString,
			},
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			MondooCredsSecretRef: corev1.LocalObjectReference{
				Name: testMondooCredsSecretName,
			},
			MondooTokenSecretRef: corev1.LocalObjectReference{
				Name: testMondooTokenSecretName,
			},
		},
	}
}

func testTokenSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testMondooTokenSecretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"token": []byte(testTokenData),
		},
	}
}

func testToken(t *testing.T) string {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "failed to generate private key for generating JWT")

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err, "failed to extract public key")

	hasher := crypto.SHA256.New()
	hasher.Write(publicKeyBytes)
	publicKeyHash := hasher.Sum(nil)
	keyID := base64.RawURLEncoding.EncodeToString(publicKeyHash)

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":          "//some/user/id",
		"aud":          []string{"mondoo"},
		"iss":          "mondoo/issuer",
		"api_endpoint": "https://some.domain.com/path/to/endpoint",
		"exp":          time.Now().Unix() + 600, // 600 seconds
		"iat":          time.Now().Unix(),
		"space":        "//some/mondoo/spaceID",
	})

	token.Header["kid"] = keyID

	tokenString, err := token.SignedString(privateKey)
	require.NoError(t, err, "failed to generate signed token string")

	return tokenString
}
