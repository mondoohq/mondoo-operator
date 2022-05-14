package integration

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	mockmondoo "go.mondoo.com/mondoo-operator/pkg/mondooclient/mock"
	"go.mondoo.com/mondoo-operator/tests/credentials"
)

const (
	testMondooCredsSecretName = "mondoo-client"
	testNamespace             = "testNamespace"

	testIntegrationMRN = "//integration.api.mondoo.app/spaces/test-infallible-taussig-123456/integrations/abcdefghhijklmnop"
)

var (
	testTokenData string

	testMondooServiceAccount = &mondooclient.ServiceAccountCredentials{
		Mrn:         "//agents.api.mondoo.app/spaces/test-infallible-taussig-123456/serviceaccounts/1234567890987654321",
		SpaceMrn:    "//captain.api.mondoo.app/spaces/test-infallible-taussig-123456",
		PrivateKey:  "REPLACE PRIVATE KEY DATA HERE FOR TESTING",
		Certificate: "CERTIFICATE DATA HERE",
		ApiEndpoint: "http://127.0.0.2:8989",
	}
	testMondooServiceAccountDataBytes []byte

	testLogger logr.Logger
)

type IntegrationCheckInSuite struct {
	suite.Suite
}

func TestMondooIntegrationCheckInSuite(t *testing.T) {
	suite.Run(t, new(IntegrationCheckInSuite))
}

func (s *IntegrationCheckInSuite) SetupSuite() {
	utilruntime.Must(clientgoscheme.AddToScheme(clientgoscheme.Scheme))
	utilruntime.Must(v1alpha2.AddToScheme(clientgoscheme.Scheme))

	// Setup logging
	var err error
	cfg := zap.NewDevelopmentConfig()

	cfg.InitialFields = map[string]interface{}{
		"controller": "integration-test",
	}

	zapLog, err := cfg.Build()
	s.Require().NoError(err, "failed to set up logging for test cases")

	testLogger = zapr.NewLogger(zapLog)

	// Build the token/service account data
	testTokenData = credentials.MondooToken(s.T(), testIntegrationMRN)

	testMondooServiceAccount.PrivateKey = credentials.MondooServiceAccount(s.T())

	testMondooServiceAccountDataBytes, err = json.Marshal(testMondooServiceAccount)
	s.Require().NoError(err, "error converting sample service account data")
}

func (s *IntegrationCheckInSuite) TestCheckIn() {
	// Arrange
	mondooAuditConfig := testMondooAuditConfig()

	existingObjects := []runtime.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), &mondooclient.IntegrationCheckInInput{
		Mrn: testIntegrationMRN, // make sure MRN in the CheckIn() in what is required for the real Mondoo API
	}).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn: testIntegrationMRN,
	}, nil)

	testMondooClientBuilder := func(mondooclient.ClientOptions) mondooclient.Client {
		return mClient
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		Log:                 testLogger,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	s.NoError(err, "should not error while processing valid MondooAuditConfig")
	mockCtrl.Finish()

}

func (s *IntegrationCheckInSuite) TestMissingIntegrationMRN() {
	// Arrange
	mondooAuditConfig := testMondooAuditConfig()

	credsSecret := testMondooCredsSecret()
	delete(credsSecret.Data, constants.MondooCredsSecretIntegrationMRNKey)

	existingObjects := []runtime.Object{
		credsSecret,
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockClient(mockCtrl)
	// EXPECT no call because of the missing integration MRN data

	testMondooClientBuilder := func(mondooclient.ClientOptions) mondooclient.Client {
		return mClient
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		Log:                 testLogger,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	// this controller doesn't make changes to k8s resources...the only side effect here are the mondooclient API calls
	s.Error(err, "expected error when missing integration MRN")
	mockCtrl.Finish()

}

func (s *IntegrationCheckInSuite) TestBadServiceAccountData() {
	// Arrange
	mondooAuditConfig := testMondooAuditConfig()

	credsSecret := testMondooCredsSecret()
	credsSecret.Data[constants.MondooCredsSecretServiceAccountKey] = []byte("NOT VALID JWT")

	existingObjects := []runtime.Object{
		credsSecret,
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockClient(mockCtrl)
	// EXPECT no call because of the bad service account data

	testMondooClientBuilder := func(mondooclient.ClientOptions) mondooclient.Client {
		return mClient
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		Log:                 testLogger,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	// this controller doesn't make changes to k8s resources...the only side effect here are the mondooclient API calls
	s.Error(err, "expected error when Mondoo service account data broken")
	mockCtrl.Finish()

}

func (s *IntegrationCheckInSuite) TestFailedCheckIn() {
	// Arrange
	mondooAuditConfig := testMondooAuditConfig()

	existingObjects := []runtime.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), gomock.Any()).Times(1).Return(
		nil, fmt.Errorf(`http status 401: {"code":16,"message":"request permission unauthenticated"}`),
	)

	testMondooClientBuilder := func(mondooclient.ClientOptions) mondooclient.Client {
		return mClient
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		Log:                 testLogger,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	// this controller doesn't make changes to k8s resources...the only side effect here are the mondooclient API calls
	s.Error(err, "expected error when CheckIn() return error")
	mockCtrl.Finish()

}

func testMondooCredsSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testMondooCredsSecretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			constants.MondooCredsSecretServiceAccountKey: testMondooServiceAccountDataBytes,
			constants.MondooCredsSecretIntegrationMRNKey: []byte(testIntegrationMRN),
		},
	}
}

func testMondooAuditConfig() *v1alpha2.MondooAuditConfig {
	return &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mondoo-config",
			Namespace: testNamespace,
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			MondooCredsSecretRef: corev1.LocalObjectReference{
				Name: testMondooCredsSecretName,
			},
		},
	}
}
