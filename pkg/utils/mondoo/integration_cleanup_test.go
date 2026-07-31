// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/common"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	mockmondoo "go.mondoo.com/mondoo-operator/pkg/client/mondooclient/mock"
	"go.mondoo.com/mondoo-operator/tests/credentials"
)

const (
	cleanupTestNamespace  = "mondoo-operator"
	cleanupTestCredsName  = "mondoo-client"
	cleanupTestTokenName  = "mondoo-token"
	cleanupTestConfigName = "mondoo-config"
)

func cleanupTestServiceAccount(t *testing.T) (mondooclient.ServiceAccountCredentials, []byte) {
	sa := mondooclient.ServiceAccountCredentials{
		Mrn:         "//agents.api.mondoo.app/spaces/test-space/serviceaccounts/1234",
		SpaceMrn:    testSpaceMrn,
		PrivateKey:  credentials.MondooServiceAccount(t),
		Certificate: "CERT",
		ApiEndpoint: "http://127.0.0.2:8989",
	}
	data, err := json.Marshal(sa) //nolint:gosec
	require.NoError(t, err)
	return sa, data
}

func cleanupTestAuditConfig() *v1alpha2.MondooAuditConfig {
	return &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cleanupTestConfigName,
			Namespace: cleanupTestNamespace,
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			MondooCredsSecretRef: corev1.LocalObjectReference{Name: cleanupTestCredsName},
			MondooTokenSecretRef: corev1.LocalObjectReference{Name: cleanupTestTokenName},
			ConsoleIntegration:   v1alpha2.ConsoleIntegration{Enable: true},
		},
	}
}

func cleanupTestCredsSecret(saData []byte, operatorManaged bool) *corev1.Secret {
	data := map[string][]byte{
		"config":         saData,
		"integrationmrn": []byte(testIntegrationMrn),
	}
	if operatorManaged {
		data["operator-managed"] = []byte("true")
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cleanupTestCredsName,
			Namespace: cleanupTestNamespace,
		},
		Data: data,
	}
}

func cleanupTestProvisionerSecret(saData []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cleanupTestCredsName + "-provisioner",
			Namespace: cleanupTestNamespace,
		},
		Data: map[string][]byte{"config": saData},
	}
}

func TestCleanupConsoleIntegration(t *testing.T) {
	_, saData := cleanupTestServiceAccount(t)

	tests := []struct {
		name             string
		auditConfig      *v1alpha2.MondooAuditConfig
		existingObjects  func() []client.Object
		mockMondooClient func(*gomock.Controller) *mockmondoo.MockMondooClient
	}{
		{
			name:        "reports deleted and deletes operator-managed integration",
			auditConfig: cleanupTestAuditConfig(),
			existingObjects: func() []client.Object {
				return []client.Object{
					cleanupTestCredsSecret(saData, true),
					cleanupTestProvisionerSecret(saData),
				}
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockMondooClient {
				mClient := mockmondoo.NewMockMondooClient(mockCtrl)
				mClient.EXPECT().IntegrationReportStatus(gomock.Any(), &mondooclient.ReportStatusRequest{
					Mrn:    testIntegrationMrn,
					Status: mondooclient.Status_DELETED,
				}).Times(1).Return(nil)
				mClient.EXPECT().IntegrationDelete(gomock.Any(), &mondooclient.IntegrationDeleteInput{
					Mrn: testIntegrationMrn,
				}).Times(1).Return(nil)
				return mClient
			},
		},
		{
			name: "retains integration when deletionPolicy is Retain",
			auditConfig: func() *v1alpha2.MondooAuditConfig {
				m := cleanupTestAuditConfig()
				m.Spec.ConsoleIntegration.DeletionPolicy = v1alpha2.IntegrationDeletionPolicyRetain
				return m
			}(),
			existingObjects: func() []client.Object {
				return []client.Object{
					cleanupTestCredsSecret(saData, true),
					cleanupTestProvisionerSecret(saData),
				}
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockMondooClient {
				mClient := mockmondoo.NewMockMondooClient(mockCtrl)
				// integration is still reported as DELETED so it does not linger as ACTIVE
				mClient.EXPECT().IntegrationReportStatus(gomock.Any(), gomock.Any()).Times(1).Return(nil)
				return mClient
			},
		},
		{
			name:        "never deletes integrations the operator did not create",
			auditConfig: cleanupTestAuditConfig(),
			existingObjects: func() []client.Object {
				return []client.Object{
					cleanupTestCredsSecret(saData, false),
					cleanupTestProvisionerSecret(saData),
				}
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockMondooClient {
				mClient := mockmondoo.NewMockMondooClient(mockCtrl)
				mClient.EXPECT().IntegrationReportStatus(gomock.Any(), gomock.Any()).Times(1).Return(nil)
				return mClient
			},
		},
		{
			name:        "does nothing without a creds secret",
			auditConfig: cleanupTestAuditConfig(),
			existingObjects: func() []client.Object {
				return nil
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockMondooClient {
				return mockmondoo.NewMockMondooClient(mockCtrl)
			},
		},
		{
			name:        "falls back to the token secret for the provisioner credential",
			auditConfig: cleanupTestAuditConfig(),
			existingObjects: func() []client.Object {
				return []client.Object{
					cleanupTestCredsSecret(saData, true),
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      cleanupTestTokenName,
							Namespace: cleanupTestNamespace,
						},
						Data: map[string][]byte{"token": saData},
					},
				}
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockMondooClient {
				mClient := mockmondoo.NewMockMondooClient(mockCtrl)
				mClient.EXPECT().IntegrationReportStatus(gomock.Any(), gomock.Any()).Times(1).Return(nil)
				mClient.EXPECT().IntegrationDelete(gomock.Any(), &mondooclient.IntegrationDeleteInput{
					Mrn: testIntegrationMrn,
				}).Times(1).Return(nil)
				return mClient
			},
		},
		{
			name:        "retries the deleted report with the provisioner credential",
			auditConfig: cleanupTestAuditConfig(),
			existingObjects: func() []client.Object {
				return []client.Object{
					cleanupTestCredsSecret(saData, true),
					cleanupTestProvisionerSecret(saData),
				}
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockMondooClient {
				mClient := mockmondoo.NewMockMondooClient(mockCtrl)
				forbidden := &common.HttpError{StatusCode: http.StatusForbidden, Body: "permission denied"}
				gomock.InOrder(
					// runtime credentials are rejected → the report is retried with the
					// provisioner credential
					mClient.EXPECT().IntegrationReportStatus(gomock.Any(), gomock.Any()).Times(1).Return(forbidden),
					mClient.EXPECT().IntegrationReportStatus(gomock.Any(), gomock.Any()).Times(1).Return(nil),
				)
				mClient.EXPECT().IntegrationDelete(gomock.Any(), &mondooclient.IntegrationDeleteInput{
					Mrn: testIntegrationMrn,
				}).Times(1).Return(nil)
				return mClient
			},
		},
		{
			name:        "tolerates an already-deleted integration",
			auditConfig: cleanupTestAuditConfig(),
			existingObjects: func() []client.Object {
				return []client.Object{
					cleanupTestCredsSecret(saData, true),
					cleanupTestProvisionerSecret(saData),
				}
			},
			mockMondooClient: func(mockCtrl *gomock.Controller) *mockmondoo.MockMondooClient {
				mClient := mockmondoo.NewMockMondooClient(mockCtrl)
				notFound := &common.HttpError{StatusCode: http.StatusNotFound, Body: "integration not found"}
				mClient.EXPECT().IntegrationReportStatus(gomock.Any(), gomock.Any()).Times(1).Return(notFound)
				mClient.EXPECT().IntegrationDelete(gomock.Any(), gomock.Any()).Times(1).Return(notFound)
				return mClient
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mClient := test.mockMondooClient(mockCtrl)
			builder := func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error) {
				return mClient, nil
			}

			fakeClient := fake.NewClientBuilder().WithObjects(test.existingObjects()...).Build()

			CleanupConsoleIntegration(context.Background(), fakeClient, builder, test.auditConfig, nil, nil, nil, logr.Discard())
		})
	}
}
