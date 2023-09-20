/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package integration

import (
	"context"
	"encoding/json"
	"fmt"
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

	testMondooServiceAccountDataBytes, err = json.Marshal(testMondooServiceAccount)
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
		Mrn: testIntegrationMRN,
	}, nil)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()

	r := &IntegrationReconciler{
		Client:              fakeClient,
		MondooClientBuilder: testMondooClientBuilder,
	}

	// Act
	err := r.processMondooAuditConfig(*mondooAuditConfig)

	// Assert
	s.NoError(err, "should not error while processing valid MondooAuditConfig")
	s.Zero(len(mondooAuditConfig.Status.Conditions), "expected no condtion set on happy path")
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

	existingObjects := []runtime.Object{
		testMondooCredsSecret(),
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	mClient.EXPECT().IntegrationCheckIn(gomock.Any(), &mondooclient.IntegrationCheckInInput{
		Mrn: testIntegrationMRN, // make sure MRN in the CheckIn() in what is required for the real Mondoo API
	}).Times(1).Return(&mondooclient.IntegrationCheckInOutput{
		Mrn: testIntegrationMRN,
	}, nil)

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()

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

	existingObjects := []runtime.Object{
		credsSecret,
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	// EXPECT no call because of the missing integration MRN data

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()

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

	existingObjects := []runtime.Object{
		credsSecret,
		mondooAuditConfig,
	}

	mockCtrl := gomock.NewController(s.T())

	mClient := mockmondoo.NewMockMondooClient(mockCtrl)
	// EXPECT no call because of the bad service account data

	testMondooClientBuilder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
		return mClient, nil
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()

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

	existingObjects := []runtime.Object{
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

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()

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
