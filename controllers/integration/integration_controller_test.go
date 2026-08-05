// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/common"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	mockmondoo "go.mondoo.com/mondoo-operator/pkg/client/mondooclient/mock"
	"go.mondoo.com/mondoo-operator/pkg/constants"
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

	testMondooServiceAccountDataBytes, err = json.Marshal(testMondooServiceAccount) //nolint:gosec
	s.Require().NoError(err, "error converting sample service account data")
}

func (s *IntegrationCheckInSuite) TestCheckIn() {
	// Arrange
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true

	existingObjects := []runtime.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), &mondooclient.IntegrationCheckInInput{
		Mrn: testIntegrationMRN, // make sure MRN in the CheckIn() in what is required for the real Mondoo API
	}).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: true,
	}, nil)
	mClient.EXPECT().IntegrationConfigure(gomock.Any(), &mondooclient.IntegrationConfigureInput{
		Mrn: testIntegrationMRN,
	}).Times(1).Return(&mondooclient.IntegrationConfigureOutput{
		Details: &mondooclient.IntegrationConfigureDetails{
			Config: `{}`,
		},
	}, nil)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	s.NoError(err, "should not error while processing valid MondooAuditConfig")
	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestClearPreviousCondition() {
	// Arrange
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	mondooAuditConfig.Status.Conditions = []v1alpha2.MondooAuditConfigCondition{
		{
			Type:   v1alpha2.MondooIntegrationDegraded,
			Status: corev1.ConditionTrue,
		},
	}

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), &mondooclient.IntegrationCheckInInput{
		Mrn: testIntegrationMRN, // make sure MRN in the CheckIn() in what is required for the real Mondoo API
	}).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: true,
	}, nil)
	mClient.EXPECT().IntegrationConfigure(gomock.Any(), &mondooclient.IntegrationConfigureInput{
		Mrn: testIntegrationMRN,
	}).Times(1).Return(&mondooclient.IntegrationConfigureOutput{
		Details: &mondooclient.IntegrationConfigureDetails{
			Config: `{}`,
		},
	}, nil)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().
		WithStatusSubresource(existingObjects...).
		WithObjects(existingObjects...).
		Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	s.NoError(err, "should not error while processing valid MondooAuditConfig")
	assertConditionExists(s.T(), fakeClient, corev1.ConditionFalse, "Mondoo integration is working")
	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestMissingIntegrationMRN() {
	// Arrange
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true

	credsSecret := testMondooCredsSecret()
	delete(credsSecret.Data, constants.MondooCredsSecretIntegrationMRNKey)

	existingObjects := []client.Object{
		credsSecret,
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	// EXPECT no call because of the missing integration MRN data

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().
		WithStatusSubresource(existingObjects...).
		WithObjects(existingObjects...).
		Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	// this controller doesn't make changes to k8s resources...the only side effect here are the mondooclient API calls
	s.Error(err, "expected error when missing integration MRN")
	assertConditionExists(s.T(), fakeClient, corev1.ConditionTrue, "key with integration MRN data")
	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestBadServiceAccountData() {
	// Arrange
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	credsSecret := testMondooCredsSecret()
	credsSecret.Data[constants.MondooCredsSecretServiceAccountKey] = []byte("NOT VALID JWT")

	existingObjects := []client.Object{
		credsSecret,
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	// EXPECT no call because of the bad service account data

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(existingObjects...).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	// this controller doesn't make changes to k8s resources...the only side effect here are the mondooclient API calls
	s.Error(err, "expected error when Mondoo service account data broken")
	assertConditionExists(s.T(), fakeClient, corev1.ConditionTrue, "failed to unmarshal creds")
	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestFailedCheckIn() {
	// Arrange
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), gomock.Any()).Times(1).Return(
		nil, fmt.Errorf(`http status 401: {"code":16,"message":"request permission unauthenticated"}`),
	)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().
		WithStatusSubresource(existingObjects...).
		WithObjects(existingObjects...).
		Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	// this controller doesn't make changes to k8s resources...the only side effect here are the mondooclient API calls
	s.Error(err, "expected error when CheckIn() return error")
	assertConditionExists(s.T(), fakeClient, corev1.ConditionTrue, "failed to CheckIn")
	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestCheckInPausesScanning() {
	// Arrange
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), &mondooclient.IntegrationCheckInInput{
		Mrn: testIntegrationMRN,
	}).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: false,
	}, nil)
	mClient.EXPECT().IntegrationConfigure(gomock.Any(), &mondooclient.IntegrationConfigureInput{
		Mrn: testIntegrationMRN,
	}).Times(1).Return(&mondooclient.IntegrationConfigureOutput{
		Details: &mondooclient.IntegrationConfigureDetails{
			Config: `{"pauseScanning":true}`,
		},
	}, nil)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().
		WithStatusSubresource(existingObjects...).
		WithObjects(existingObjects...).
		Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	s.NoError(err)

	updated := testMondooAuditConfig()
	s.NoError(fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(updated), updated))
	s.True(updated.Status.ScanningPaused, "expected ScanningPaused=true after pause")
	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestCheckInUnpausesScanning() {
	// Arrange
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	mondooAuditConfig.Status.ScanningPaused = true

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), &mondooclient.IntegrationCheckInInput{
		Mrn: testIntegrationMRN,
	}).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: false,
	}, nil)
	mClient.EXPECT().IntegrationConfigure(gomock.Any(), &mondooclient.IntegrationConfigureInput{
		Mrn: testIntegrationMRN,
	}).Times(1).Return(&mondooclient.IntegrationConfigureOutput{
		Details: &mondooclient.IntegrationConfigureDetails{
			Config: `{"pauseScanning":false}`,
		},
	}, nil)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().
		WithStatusSubresource(existingObjects...).
		WithObjects(existingObjects...).
		Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	s.NoError(err)

	updated := testMondooAuditConfig()
	s.NoError(fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(updated), updated))
	s.False(updated.Status.ScanningPaused, "expected ScanningPaused=false after unpause")
	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestCheckInConfigMatchCallsConfigureWhenNoStoredHash() {
	// ConfigurationMatch=true but Configure is still called because we have no stored hash
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), &mondooclient.IntegrationCheckInInput{
		Mrn: testIntegrationMRN,
	}).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: true,
	}, nil)
	mClient.EXPECT().IntegrationConfigure(gomock.Any(), &mondooclient.IntegrationConfigureInput{
		Mrn: testIntegrationMRN,
	}).Times(1).Return(&mondooclient.IntegrationConfigureOutput{
		Details: &mondooclient.IntegrationConfigureDetails{
			Config: `{"pauseScanning":false}`,
		},
	}, nil)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	s.NoError(err)
	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestCheckInConfigMatchSkipsConfigureWithStoredHash() {
	// When we have a stored hash and ConfigurationMatch=true, Configure must NOT be called
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	mondooAuditConfig.Status.RemoteConfigHash = "abc123"

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), &mondooclient.IntegrationCheckInInput{
		Mrn:               testIntegrationMRN,
		ConfigurationHash: "abc123",
	}).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: true,
	}, nil)
	// IntegrationConfigure must NOT be called — gomock will fail if it is

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	err := r.processMondooAuditConfig(*mondooAuditConfig)

	s.NoError(err)
	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestConfigureFailureDoesNotCrash() {
	// Configure fails — operator must NOT error, scanning state must NOT change
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: false,
	}, nil)
	mClient.EXPECT().IntegrationConfigure(gomock.Any(), gomock.Any()).Times(1).Return(
		nil, fmt.Errorf("connection refused"),
	)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert — no error returned, operator stays healthy
	s.NoError(err)
	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestCheckInIntegrationDeletedInConsole() {
	// The integration was deleted in the Mondoo console: CheckIn returns a 404. The operator
	// must not recreate anything and must surface an actionable degraded condition.
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), gomock.Any()).Times(1).Return(
		nil, fmt.Errorf("failed to parse response: %w", &common.HttpError{StatusCode: http.StatusNotFound, Body: "integration not found"}),
	)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	s.Error(err)
	s.Contains(err.Error(), "no longer exists in the Mondoo console")
	assertConditionExists(s.T(), fakeClient, corev1.ConditionTrue, "no longer exists in the Mondoo console")
	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestRemoteConfigPersistedToStatus() {
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	mondooAuditConfig.Spec.RemoteManaged = true

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	configJSON := `{"scanNodes":true,"scanWorkloads":true,"schedule":"0 * * * *"}`
	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: false,
	}, nil)
	mClient.EXPECT().IntegrationConfigure(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.IntegrationConfigureOutput{
		Details: &mondooclient.IntegrationConfigureDetails{
			Config: configJSON,
		},
	}, nil)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	err := r.processMondooAuditConfig(*mondooAuditConfig)
	s.NoError(err)

	updated := testMondooAuditConfig()
	s.NoError(fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(updated), updated))
	s.Equal(configJSON, updated.Status.RemoteConfig)
	s.NotEmpty(updated.Status.RemoteConfigHash)
	s.NotNil(updated.Status.LastRemoteConfigTime)

	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestRemoteConfigDegradedOnCheckInFailure() {
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	mondooAuditConfig.Spec.RemoteManaged = true

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), gomock.Any()).Times(1).Return(
		nil, fmt.Errorf("connection refused"),
	)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	err := r.processMondooAuditConfig(*mondooAuditConfig)
	s.Error(err)

	updated := testMondooAuditConfig()
	s.NoError(fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(updated), updated))
	found := false
	for _, cond := range updated.Status.Conditions {
		if cond.Type == v1alpha2.RemoteConfigDegradedCondition {
			found = true
			s.Equal(corev1.ConditionTrue, cond.Status)
			s.Equal("FetchFailed", cond.Reason)
		}
	}
	s.True(found, "RemoteConfigDegraded condition should exist")

	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestRemoteConfigDegradedClearedOnSuccess() {
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	mondooAuditConfig.Spec.RemoteManaged = true
	mondooAuditConfig.Status.Conditions = []v1alpha2.MondooAuditConfigCondition{
		{
			Type:   v1alpha2.RemoteConfigDegradedCondition,
			Status: corev1.ConditionTrue,
			Reason: "FetchFailed",
		},
	}

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: false,
	}, nil)
	mClient.EXPECT().IntegrationConfigure(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.IntegrationConfigureOutput{
		Details: &mondooclient.IntegrationConfigureDetails{
			Config: `{"scanNodes":true}`,
		},
	}, nil)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().
		WithStatusSubresource(mondooAuditConfig).
		WithObjects(existingObjects...).
		Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	err := r.processMondooAuditConfig(*mondooAuditConfig)
	s.NoError(err)

	updated := testMondooAuditConfig()
	s.NoError(fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(updated), updated))
	for _, cond := range updated.Status.Conditions {
		if cond.Type == v1alpha2.RemoteConfigDegradedCondition {
			s.Equal(corev1.ConditionFalse, cond.Status)
			s.Equal("ConfigFetched", cond.Reason)
		}
	}

	mockCtrl.Finish()
}

func (s *IntegrationCheckInSuite) TestNonRemoteManagedSkipsRemoteConfigCondition() {
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	// RemoteManaged is false (default)

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), gomock.Any()).Times(1).Return(
		nil, fmt.Errorf("connection refused"),
	)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	err := r.processMondooAuditConfig(*mondooAuditConfig)
	s.Error(err)

	updated := testMondooAuditConfig()
	s.NoError(fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(updated), updated))
	for _, cond := range updated.Status.Conditions {
		s.NotEqual(v1alpha2.RemoteConfigDegradedCondition, cond.Type, "RemoteConfigDegraded should not be set for non-remote-managed CRs")
	}

	mockCtrl.Finish()
}

// D5: Last-known-good — Configure failure preserves existing status.remoteConfig
func (s *IntegrationCheckInSuite) TestLastKnownGoodOnConfigureFailure() {
	existingConfig := `{"scanNodes":true,"scanWorkloads":true}`
	existingHash := "deadbeef"

	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	mondooAuditConfig.Spec.RemoteManaged = true
	mondooAuditConfig.Status.RemoteConfig = existingConfig
	mondooAuditConfig.Status.RemoteConfigHash = existingHash

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: false,
	}, nil)
	// Configure fails — simulates network error
	mClient.EXPECT().IntegrationConfigure(gomock.Any(), gomock.Any()).Times(1).Return(
		nil, fmt.Errorf("connection timeout"),
	)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	err := r.processMondooAuditConfig(*mondooAuditConfig)
	// Configure failure is swallowed — no error returned
	s.NoError(err)

	updated := testMondooAuditConfig()
	s.NoError(fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(updated), updated))
	// Last-known-good preserved: config and hash unchanged
	s.Equal(existingConfig, updated.Status.RemoteConfig)
	s.Equal(existingHash, updated.Status.RemoteConfigHash)

	mockCtrl.Finish()
}

// D5: Config changes — new config replaces old config in status
func (s *IntegrationCheckInSuite) TestConfigChangeUpdatesStatus() {
	oldConfig := `{"scanNodes":false}`
	newConfig := `{"scanNodes":true,"scanWorkloads":true,"scannerReplicas":3}`

	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	mondooAuditConfig.Spec.RemoteManaged = true
	mondooAuditConfig.Status.RemoteConfig = oldConfig
	mondooAuditConfig.Status.RemoteConfigHash = "oldhash"

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), &mondooclient.IntegrationCheckInInput{
		Mrn:               testIntegrationMRN,
		ConfigurationHash: "oldhash",
	}).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: false,
	}, nil)
	mClient.EXPECT().IntegrationConfigure(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.IntegrationConfigureOutput{
		Details: &mondooclient.IntegrationConfigureDetails{
			Config: newConfig,
		},
	}, nil)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	err := r.processMondooAuditConfig(*mondooAuditConfig)
	s.NoError(err)

	updated := testMondooAuditConfig()
	s.NoError(fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(updated), updated))
	s.Equal(newConfig, updated.Status.RemoteConfig)
	s.NotEqual("oldhash", updated.Status.RemoteConfigHash)
	s.NotNil(updated.Status.LastRemoteConfigTime)

	mockCtrl.Finish()
}

// Hash-match means no Configure call and no status change
func (s *IntegrationCheckInSuite) TestConfigMatchPreservesExistingConfig() {
	existingConfig := `{"scanNodes":true}`
	existingHash := "abc123"

	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	mondooAuditConfig.Spec.RemoteManaged = true
	mondooAuditConfig.Status.RemoteConfig = existingConfig
	mondooAuditConfig.Status.RemoteConfigHash = existingHash

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), &mondooclient.IntegrationCheckInInput{
		Mrn:               testIntegrationMRN,
		ConfigurationHash: existingHash,
	}).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: true,
	}, nil)
	// IntegrationConfigure must NOT be called — gomock will fail if it is

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	err := r.processMondooAuditConfig(*mondooAuditConfig)
	s.NoError(err)

	updated := testMondooAuditConfig()
	s.NoError(fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(updated), updated))
	// Config unchanged
	s.Equal(existingConfig, updated.Status.RemoteConfig)
	s.Equal(existingHash, updated.Status.RemoteConfigHash)

	mockCtrl.Finish()
}

// Pause state changes independently of remote config
func (s *IntegrationCheckInSuite) TestPauseChangedViaRemoteConfig() {
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	mondooAuditConfig.Spec.RemoteManaged = true

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: false,
	}, nil)
	mClient.EXPECT().IntegrationConfigure(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.IntegrationConfigureOutput{
		Details: &mondooclient.IntegrationConfigureDetails{
			Config: `{"pauseScanning": true, "scanNodes": true}`,
		},
	}, nil)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	err := r.processMondooAuditConfig(*mondooAuditConfig)
	s.NoError(err)

	updated := testMondooAuditConfig()
	s.NoError(fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(updated), updated))
	s.True(updated.Status.ScanningPaused)

	mockCtrl.Finish()
}

// Configure returns empty details — no crash, no config change
func (s *IntegrationCheckInSuite) TestConfigureReturnsEmptyDetails() {
	mondooAuditConfig := testMondooAuditConfig()
	mondooAuditConfig.Spec.ConsoleIntegration.Enable = true
	mondooAuditConfig.Spec.RemoteManaged = true
	mondooAuditConfig.Status.RemoteConfig = `{"scanNodes":true}`
	mondooAuditConfig.Status.RemoteConfigHash = "existing"

	existingObjects := []client.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn:                testIntegrationMRN,
		ConfigurationMatch: false,
	}, nil)
	mClient.EXPECT().IntegrationConfigure(gomock.Any(), gomock.Any()).Times(1).Return(&mondooclient.IntegrationConfigureOutput{
		Details: &mondooclient.IntegrationConfigureDetails{
			Config: "",
		},
	}, nil)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithObjects(existingObjects...).WithStatusSubresource(mondooAuditConfig).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	err := r.processMondooAuditConfig(*mondooAuditConfig)
	s.NoError(err)

	updated := testMondooAuditConfig()
	s.NoError(fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(updated), updated))
	// Last-known-good preserved
	s.Equal(`{"scanNodes":true}`, updated.Status.RemoteConfig)
	s.Equal("existing", updated.Status.RemoteConfigHash)

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

func assertConditionExists(t *testing.T, kubeClient client.Client, status corev1.ConditionStatus, message string) {
	mondoo := testMondooAuditConfig()
	require.NoError(t, kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(mondoo), mondoo), "error fetching current MondooAuditConfig from fake client")

	found := false
	for _, cond := range mondoo.Status.Conditions {
		if cond.Type == v1alpha2.MondooIntegrationDegraded {
			found = true
			assert.Equal(t, status, cond.Status)
			assert.Contains(t, cond.Message, message)
		}
	}

	assert.True(t, found, "expected condition to exist")
}
