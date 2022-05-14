package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	scheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	mockmondoo "go.mondoo.com/mondoo-operator/pkg/mondooclient/mock"
	"go.mondoo.com/mondoo-operator/tests/credentials"
)

const (
	testMondooAuditConfigName = "mondoo-config"
	testNamespace             = "mondoo-operator"
	testMondooCredsSecretName = "mondoo-client"
	testMondooTokenSecretName = "mondoo-token'"

	testServiceAccountData = `SERVICE ACCOUNT DATA HERE`

	testIntegrationMRN = "//integration.api.mondoo.app/spaces/test-infallible-taussig-123456/integrations/abcdefghhijklmnop"
)

var (
	testTokenData            string
	testIntegrationTokenData string

	testMondooServiceAccount = &mondooclient.ServiceAccountCredentials{
		Mrn:         "//agents.api.mondoo.app/spaces/test-infallible-taussig-123456/serviceaccounts/1234567890987654321",
		SpaceMrn:    "//captain.api.mondoo.app/spaces/test-infallible-taussig-123456",
		PrivateKey:  "PRIVATE KEY DATA HERE",
		Certificate: "CERTIFICATE DATA HERE",
		ApiEndpoint: "http://127.0.0.2:8989",
	}
	testMondooServiceAccountDataBytes []byte
)

func init() {
	utilruntime.Must(v1alpha2.AddToScheme(scheme.Scheme))
	utilruntime.Must(scheme.AddToScheme(scheme.Scheme))
	utilruntime.Must(certmanagerv1.AddToScheme(scheme.Scheme))
}

func TestTokenRegistration(t *testing.T) {

	utilruntime.Must(v1alpha2.AddToScheme(scheme.Scheme))

	testTokenData = credentials.MondooToken(t, "")
	testIntegrationTokenData = credentials.MondooToken(t, testIntegrationMRN)
	testMondooServiceAccount.PrivateKey = credentials.MondooServiceAccount(t)

	var err error
	testMondooServiceAccountDataBytes, err = json.Marshal(testMondooServiceAccount)
	require.NoError(t, err, "error converting sample service account data")

	tests := []struct {
		name             string
		existingObjects  []runtime.Object
		mockMondooClient func(*gomock.Controller) *mockmondoo.MockClient
		verify           func(*testing.T, client.Client)
		expectError      bool
	}{
		{
			name: "generate service account from token secret",
			existingObjects: []runtime.Object{
				testTokenSecret(),
				testMondooAuditConfig(),
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockClient {
				mClient := mockmondoo.NewMockClient(mockCtrl)

				mClient.EXPECT().ExchangeRegistrationToken(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.ExchangeRegistrationTokenOutput{
					ServiceAccount: testServiceAccountData,
				}, nil)

				return mClient
			},
			verify: func(t *testing.T, kubeClient client.Client) {

				credsSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testMondooCredsSecretName,
						Namespace: testNamespace,
					},
				}
				err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(credsSecret), credsSecret)

				assert.NoError(t, err, "error getting secret that should exist")

				// Check StringData because we're using the fake client
				assert.Equal(t, testServiceAccountData, credsSecret.StringData["config"])
			},
		},
		{
			name: "no token, no service account",
			existingObjects: []runtime.Object{
				testMondooAuditConfig(),
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockClient {
				mClient := mockmondoo.NewMockClient(mockCtrl)

				return mClient
			},
			verify: func(t *testing.T, kubeClient client.Client) {
				credsSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testMondooCredsSecretName,
						Namespace: testNamespace,
					},
				}

				err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(credsSecret), credsSecret)
				assert.True(t, errors.IsNotFound(err), "expected Mondoo creds secret to not exist")
			},
		},
		{
			name: "already a Mondoo creds secret",
			existingObjects: func() []runtime.Object {
				objs := []runtime.Object{testMondooAuditConfig()}

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
			verify: func(t *testing.T, kubeClient client.Client) {
				credsSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testMondooCredsSecretName,
						Namespace: testNamespace,
					},
				}
				err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(credsSecret), credsSecret)

				assert.NoError(t, err, "unexpected error getting pre-existing Secret")

				assert.Equal(t, "EXISTING MONDOO CONFIG", credsSecret.StringData["config"])
			},
		},
		{
			name: "mondoo API error",
			existingObjects: []runtime.Object{
				testTokenSecret(),
				testMondooAuditConfig(),
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockClient {
				mClient := mockmondoo.NewMockClient(mockCtrl)

				mClient.EXPECT().ExchangeRegistrationToken(gomock.Any(), gomock.Any()).Times(1).Return(nil, fmt.Errorf("an error occurred"))

				return mClient
			},
			expectError: true,
		},
		{
			name: "malformed JWT",
			existingObjects: func() []runtime.Object {
				objs := []runtime.Object{testMondooAuditConfig()}
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
		{
			name: "generate service account via Integrations",
			existingObjects: []runtime.Object{
				testIntegrationTokenSecret(),
				testMondooAuditConfigWithIntegration(),
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockClient {
				mClient := mockmondoo.NewMockClient(mockCtrl)

				mClient.EXPECT().IntegrationRegister(gomock.Any(), &mondooclient.IntegrationRegisterInput{
					Mrn:   testIntegrationMRN,       // verify we are getting the expected integration MRN
					Token: testIntegrationTokenData, // and that the token data matches what was in the token Secret
				}).Times(1).Return(&mondooclient.IntegrationRegisterOutput{
					Mrn:   testIntegrationMRN,
					Creds: testMondooServiceAccount,
				}, nil)

				// expect initial CheckIn()
				mClient.EXPECT().IntegrationCheckIn(gomock.Any(), &mondooclient.IntegrationCheckInInput{
					Mrn: testIntegrationMRN,
				}).Times(1).Return(&mondooclient.IntegrationCheckInOutput{}, nil)

				return mClient
			},
			verify: func(t *testing.T, kubeClient client.Client) {

				credsSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testMondooCredsSecretName,
						Namespace: testNamespace,
					},
				}
				err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(credsSecret), credsSecret)

				assert.NoError(t, err, "error getting secret that should exist")

				// Check StringData because we're using the fake client
				assert.Equal(t, string(testMondooServiceAccountDataBytes), credsSecret.StringData["config"])
			},
		},
		{
			name: "missing owner claim error",
			existingObjects: []runtime.Object{
				testTokenSecret(),
				testMondooAuditConfigWithIntegration(),
			},
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

			fakeClient := fake.NewClientBuilder().WithRuntimeObjects(test.existingObjects...).Build()

			reconciler := &MondooAuditConfigReconciler{
				MondooClientBuilder: testMondooClientBuilder,
				Client:              fakeClient,
			}

			// Act
			_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testMondooAuditConfigName,
					Namespace: testNamespace,
				},
			})

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

func testMondooAuditConfigWithIntegration() *v1alpha2.MondooAuditConfig {
	mac := testMondooAuditConfig()
	mac.Spec.ConsoleIntegration.Enable = true

	return mac
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

func testIntegrationTokenSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testMondooTokenSecretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"token": []byte(testIntegrationTokenData),
		},
	}
}
