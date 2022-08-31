/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package status

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient/mock"
	"go.mondoo.com/mondoo-operator/tests/credentials"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testNamespace      = "mondoo-operator"
	testIntegrationMrn = "test-mrn"
)

type StatusReporterSuite struct {
	suite.Suite
	ctx context.Context

	auditConfig       v1alpha2.MondooAuditConfig
	fakeClientBuilder *fake.ClientBuilder
	mockCtrl          *gomock.Controller
	mockMondooClient  *mock.MockClient
}

func (s *StatusReporterSuite) SetupSuite() {
	s.ctx = context.Background()
}

func (s *StatusReporterSuite) BeforeTest(suiteName, testName string) {
	s.auditConfig = utils.DefaultAuditConfig(testNamespace, false, false, false)
	s.auditConfig.Spec.ConsoleIntegration.Enable = true

	key := credentials.MondooServiceAccount(s.T())
	sa, err := json.Marshal(mondooclient.ServiceAccountCredentials{Mrn: "mrn", PrivateKey: key})
	s.Require().NoError(err)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.auditConfig.Spec.MondooCredsSecretRef.Name,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			constants.MondooCredsSecretServiceAccountKey: sa,
			constants.MondooCredsSecretIntegrationMRNKey: []byte(testIntegrationMrn),
		},
	}

	s.fakeClientBuilder = fake.NewClientBuilder().WithObjects(secret)
	s.mockCtrl = gomock.NewController(s.T())
	s.mockMondooClient = mock.NewMockClient(s.mockCtrl)
}

func (s *StatusReporterSuite) AfterTest(suiteName, testName string) {
	s.mockCtrl.Finish()
}

func (s *StatusReporterSuite) TestReport() {
	nodes := s.seedNodes()
	statusReport := s.createStatusReporter()

	nodeNames := make([]string, len(nodes))
	for i := range nodes {
		nodeNames[i] = nodes[i].GetName()
	}

	s.mockMondooClient.EXPECT().IntegrationReportStatus(gomock.Any(), &mondooclient.ReportStatusRequest{
		Mrn:     testIntegrationMrn,
		Status:  mondooclient.Status_ACTIVE,
		Version: "latest",
		Messages: mondooclient.Messages{
			Messages: []mondooclient.IntegrationMessage{
				{
					Message:    "Kubernetes resources scanning is disabled",
					Identifier: K8sResourcesScanningIdentifier,
					Status:     mondooclient.MessageStatus_MESSAGE_INFO,
				},
				{
					Message:    "Node scanning is disabled",
					Identifier: NodeScanningIdentifier,
					Status:     mondooclient.MessageStatus_MESSAGE_INFO,
				},
				{
					Message:    "Admission controller is disabled",
					Identifier: AdmissionControllerIdentifier,
					Status:     mondooclient.MessageStatus_MESSAGE_INFO,
				},
				{
					Message:    "Scan API is disabled",
					Identifier: ScanApiIdentifier,
					Status:     mondooclient.MessageStatus_MESSAGE_INFO,
				},
			},
		},
		LastState: OperatorCustomState{
			KubernetesVersion: statusReport.k8sVersion.GitVersion,
			Nodes:             nodeNames,
			MondooAuditConfig: MondooAuditConfig{Name: s.auditConfig.Name, Namespace: s.auditConfig.Namespace},
		},
	}).Times(1).Return(nil)

	s.NoError(statusReport.Report(s.ctx, s.auditConfig))

	// We call Report another time to make sure IntegrationReportStatus is only called whenever the status actually changes.
	s.NoError(statusReport.Report(s.ctx, s.auditConfig))
}

func (s *StatusReporterSuite) TestReport_StatusChange() {
	nodes := s.seedNodes()
	statusReport := s.createStatusReporter()

	nodeNames := make([]string, len(nodes))
	for i := range nodes {
		nodeNames[i] = nodes[i].GetName()
	}

	expected := &mondooclient.ReportStatusRequest{
		Mrn:     testIntegrationMrn,
		Status:  mondooclient.Status_ACTIVE,
		Version: "latest",
		Messages: mondooclient.Messages{
			Messages: []mondooclient.IntegrationMessage{
				{
					Message:    "Kubernetes resources scanning is disabled",
					Identifier: K8sResourcesScanningIdentifier,
					Status:     mondooclient.MessageStatus_MESSAGE_INFO,
				},
				{
					Message:    "Node scanning is disabled",
					Identifier: NodeScanningIdentifier,
					Status:     mondooclient.MessageStatus_MESSAGE_INFO,
				},
				{
					Message:    "Admission controller is disabled",
					Identifier: AdmissionControllerIdentifier,
					Status:     mondooclient.MessageStatus_MESSAGE_INFO,
				},
				{
					Message:    "Scan API is disabled",
					Identifier: ScanApiIdentifier,
					Status:     mondooclient.MessageStatus_MESSAGE_INFO,
				},
			},
		},
		LastState: OperatorCustomState{
			KubernetesVersion: statusReport.k8sVersion.GitVersion,
			Nodes:             nodeNames,
			MondooAuditConfig: MondooAuditConfig{Name: s.auditConfig.Name, Namespace: s.auditConfig.Namespace},
		},
	}
	s.mockMondooClient.EXPECT().IntegrationReportStatus(gomock.Any(), expected).Times(1).Return(nil)

	s.NoError(statusReport.Report(s.ctx, s.auditConfig))

	s.auditConfig.Spec.Nodes.Enable = true
	s.auditConfig.Status.Conditions = []v1alpha2.MondooAuditConfigCondition{
		{Message: "Node Scanning error", Status: v1.ConditionTrue, Type: v1alpha2.NodeScanningDegraded},
	}

	expected.Status = mondooclient.Status_ERROR
	expected.Messages.Messages[1].Message = s.auditConfig.Status.Conditions[0].Message
	expected.Messages.Messages[1].Status = mondooclient.MessageStatus_MESSAGE_ERROR
	s.mockMondooClient.EXPECT().IntegrationReportStatus(gomock.Any(), expected).Times(1).Return(nil)

	// We call Report another time to make sure IntegrationReportStatus is only called whenever the status actually changes.
	s.NoError(statusReport.Report(s.ctx, s.auditConfig))
}

func TestStatusReporterSuite(t *testing.T) {
	suite.Run(t, new(StatusReporterSuite))
}

func (s *StatusReporterSuite) seedNodes() []client.Object {
	nodes := []client.Object{
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node01"}},
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node02"}},
	}
	s.fakeClientBuilder = s.fakeClientBuilder.WithObjects(nodes...)
	return nodes
}

func (s *StatusReporterSuite) createStatusReporter() StatusReporter {
	return StatusReporter{
		kubeClient:          s.fakeClientBuilder.Build(),
		k8sVersion:          &version.Info{GitVersion: "v1.24.0"},
		mondooClientBuilder: func(opts mondooclient.ClientOptions) mondooclient.Client { return s.mockMondooClient },
	}
}
