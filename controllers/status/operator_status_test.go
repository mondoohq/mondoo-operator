// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package status

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sversion "k8s.io/apimachinery/pkg/version"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/version"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
)

func TestReportStatusRequestFromAuditConfig_AllDisabled(t *testing.T) {
	logger := logr.Logger{}
	integrationMrn := utils.RandString(10)
	nodes := []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
	}
	v := &k8sversion.Info{GitVersion: "v1.24.0"}

	m := testMondooAuditConfig()
	reportStatus := ReportStatusRequestFromAuditConfig(integrationMrn, m, nodes, v, logger)
	assert.Equal(t, integrationMrn, reportStatus.Mrn)
	assert.Equal(t, mondooclient.Status_ACTIVE, reportStatus.Status)
	assert.Equal(t, OperatorCustomState{
		Nodes:             []string{"node1", "node2"},
		KubernetesVersion: v.GitVersion,
		MondooAuditConfig: MondooAuditConfig{Name: m.Name, Namespace: m.Namespace},
		OperatorVersion:   version.Version,
		FilteringConfig:   v1alpha2.Filtering{},
	}, reportStatus.LastState)
	messages := []mondooclient.IntegrationMessage{
		{Identifier: K8sResourcesScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: "Kubernetes resources scanning is disabled"},
		{Identifier: ContainerImageScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: "Container image scanning is disabled"},
		{Identifier: NodeScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: "Node scanning is disabled"},
		{Identifier: ScanApiIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: "Scan API is disabled"},
		{Identifier: MondooOperatorIdentifier, Status: mondooclient.MessageStatus_MESSAGE_UNKNOWN, Message: "No status reported yet"},
	}
	assert.ElementsMatch(t, messages, reportStatus.Messages.Messages)
}

func TestReportStatusRequestFromAuditConfig_AllEnabled(t *testing.T) {
	logger := logr.Logger{}
	integrationMrn := utils.RandString(10)
	nodes := []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
	}
	v := &k8sversion.Info{GitVersion: "v1.24.0"}

	m := testMondooAuditConfig()
	m.Spec.KubernetesResources.Enable = true
	m.Spec.Containers.Enable = true
	m.Spec.Nodes.Enable = true
	m.Spec.Filtering.Namespaces = v1alpha2.FilteringSpec{
		Include: []string{"includeA", "includeB"},
		Exclude: []string{"excludeX", "excludeY"},
	}

	m.Status.Conditions = []v1alpha2.MondooAuditConfigCondition{
		{Message: "Kubernetes Resources Scanning is available", Status: v1.ConditionFalse, Type: v1alpha2.K8sResourcesScanningDegraded},
		{Message: "Kubernetes Container Image Scanning is available", Status: v1.ConditionFalse, Type: v1alpha2.K8sContainerImageScanningDegraded},
		{Message: "Node Scanning is available", Status: v1.ConditionFalse, Type: v1alpha2.NodeScanningDegraded},
		{Message: "ScanAPI controller is available", Status: v1.ConditionFalse, Type: v1alpha2.ScanAPIDegraded},
		{Message: "Mondoo Operator controller is available", Status: v1.ConditionFalse, Type: v1alpha2.MondooOperatorDegraded},
	}

	reportStatus := ReportStatusRequestFromAuditConfig(integrationMrn, m, nodes, v, logger)
	assert.Equal(t, integrationMrn, reportStatus.Mrn)
	assert.Equal(t, mondooclient.Status_ACTIVE, reportStatus.Status)
	assert.Equal(t, OperatorCustomState{
		Nodes:                  []string{"node1", "node2"},
		KubernetesVersion:      v.GitVersion,
		MondooAuditConfig:      MondooAuditConfig{Name: m.Name, Namespace: m.Namespace},
		OperatorVersion:        version.Version,
		K8sResourcesScanning:   m.Spec.KubernetesResources.Enable,
		ContainerImageScanning: m.Spec.Containers.Enable,
		NodeScanning:           m.Spec.Nodes.Enable,
		FilteringConfig: v1alpha2.Filtering{
			Namespaces: v1alpha2.FilteringSpec{
				Include: []string{"includeA", "includeB"},
				Exclude: []string{"excludeX", "excludeY"},
			},
		},
	}, reportStatus.LastState)
	messages := []mondooclient.IntegrationMessage{
		{Identifier: K8sResourcesScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[0].Message},
		{Identifier: ContainerImageScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[1].Message},
		{Identifier: NodeScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[2].Message},
		{Identifier: ScanApiIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[3].Message},
		{Identifier: MondooOperatorIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[4].Message},
	}
	assert.ElementsMatch(t, messages, reportStatus.Messages.Messages)
}

func TestReportStatusRequestFromAuditConfig_AllEnabled_DeprecatedFields(t *testing.T) {
	logger := logr.Logger{}
	integrationMrn := utils.RandString(10)
	nodes := []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
	}
	v := &k8sversion.Info{GitVersion: "v1.24.0"}

	m := testMondooAuditConfig()
	m.Spec.KubernetesResources.Enable = true
	m.Spec.KubernetesResources.ContainerImageScanning = true
	m.Spec.Nodes.Enable = true
	m.Spec.Filtering.Namespaces = v1alpha2.FilteringSpec{
		Include: []string{"includeA", "includeB"},
		Exclude: []string{"excludeX", "excludeY"},
	}

	m.Status.Conditions = []v1alpha2.MondooAuditConfigCondition{
		{Message: "Kubernetes Resources Scanning is available", Status: v1.ConditionFalse, Type: v1alpha2.K8sResourcesScanningDegraded},
		{Message: "Kubernetes Container Image Scanning is available", Status: v1.ConditionFalse, Type: v1alpha2.K8sContainerImageScanningDegraded},
		{Message: "Node Scanning is available", Status: v1.ConditionFalse, Type: v1alpha2.NodeScanningDegraded},
		{Message: "ScanAPI controller is available", Status: v1.ConditionFalse, Type: v1alpha2.ScanAPIDegraded},
		{Message: "Mondoo Operator controller is available", Status: v1.ConditionFalse, Type: v1alpha2.MondooOperatorDegraded},
	}

	reportStatus := ReportStatusRequestFromAuditConfig(integrationMrn, m, nodes, v, logger)
	assert.Equal(t, integrationMrn, reportStatus.Mrn)
	assert.Equal(t, mondooclient.Status_ACTIVE, reportStatus.Status)
	assert.Equal(t, OperatorCustomState{
		Nodes:                  []string{"node1", "node2"},
		KubernetesVersion:      v.GitVersion,
		MondooAuditConfig:      MondooAuditConfig{Name: m.Name, Namespace: m.Namespace},
		OperatorVersion:        version.Version,
		K8sResourcesScanning:   m.Spec.KubernetesResources.Enable,
		ContainerImageScanning: m.Spec.KubernetesResources.ContainerImageScanning,
		NodeScanning:           m.Spec.Nodes.Enable,
		FilteringConfig: v1alpha2.Filtering{
			Namespaces: v1alpha2.FilteringSpec{
				Include: []string{"includeA", "includeB"},
				Exclude: []string{"excludeX", "excludeY"},
			},
		},
	}, reportStatus.LastState)
	messages := []mondooclient.IntegrationMessage{
		{Identifier: K8sResourcesScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[0].Message},
		{Identifier: ContainerImageScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[1].Message},
		{Identifier: NodeScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[2].Message},
		{Identifier: ScanApiIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[3].Message},
		{Identifier: MondooOperatorIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[4].Message},
	}
	assert.ElementsMatch(t, messages, reportStatus.Messages.Messages)
}

func TestReportStatusRequestFromAuditConfig_AllError(t *testing.T) {
	logger := logr.Logger{}
	integrationMrn := utils.RandString(10)
	nodes := []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
	}
	v := &k8sversion.Info{GitVersion: "v1.24.0"}

	m := testMondooAuditConfig()
	m.Spec.KubernetesResources.Enable = true
	m.Spec.KubernetesResources.ContainerImageScanning = true
	m.Spec.Nodes.Enable = true

	m.Status.Conditions = []v1alpha2.MondooAuditConfigCondition{
		{Message: "Kubernetes Resources Scanning error", Status: v1.ConditionTrue, Type: v1alpha2.K8sResourcesScanningDegraded},
		{Message: "Kubernetes Container Image Scanning error", Status: v1.ConditionFalse, Type: v1alpha2.K8sContainerImageScanningDegraded},
		{Message: "Node Scanning error", Status: v1.ConditionTrue, Type: v1alpha2.NodeScanningDegraded},
		{Message: "ScanAPI controller error", Status: v1.ConditionTrue, Type: v1alpha2.ScanAPIDegraded},
		{Message: "Mondoo Operator controller is unavailable", Status: v1.ConditionTrue, Type: v1alpha2.MondooOperatorDegraded},
	}

	reportStatus := ReportStatusRequestFromAuditConfig(integrationMrn, m, nodes, v, logger)
	assert.Equal(t, integrationMrn, reportStatus.Mrn)
	assert.Equal(t, mondooclient.Status_ERROR, reportStatus.Status)
	assert.Equal(t, OperatorCustomState{
		Nodes:                  []string{"node1", "node2"},
		KubernetesVersion:      v.GitVersion,
		MondooAuditConfig:      MondooAuditConfig{Name: m.Name, Namespace: m.Namespace},
		OperatorVersion:        version.Version,
		K8sResourcesScanning:   m.Spec.KubernetesResources.Enable,
		ContainerImageScanning: m.Spec.KubernetesResources.ContainerImageScanning,
		NodeScanning:           m.Spec.Nodes.Enable,
		FilteringConfig:        v1alpha2.Filtering{},
	}, reportStatus.LastState)
	messages := []mondooclient.IntegrationMessage{
		{Identifier: K8sResourcesScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_ERROR, Message: m.Status.Conditions[0].Message},
		{Identifier: ContainerImageScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[1].Message},
		{Identifier: NodeScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_ERROR, Message: m.Status.Conditions[2].Message},
		{Identifier: ScanApiIdentifier, Status: mondooclient.MessageStatus_MESSAGE_ERROR, Message: m.Status.Conditions[3].Message},
		{Identifier: MondooOperatorIdentifier, Status: mondooclient.MessageStatus_MESSAGE_ERROR, Message: m.Status.Conditions[4].Message},
	}
	assert.ElementsMatch(t, messages, reportStatus.Messages.Messages)
}

func testMondooAuditConfig() v1alpha2.MondooAuditConfig {
	return v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: "mondoo-operator",
		},
	}
}

func TestReportStatusRequestFromAuditConfig_AllEnabled_ScanAPI_OOM(t *testing.T) {
	logger := logr.Logger{}
	integrationMrn := utils.RandString(10)
	nodes := []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
	}
	v := &k8sversion.Info{GitVersion: "v1.24.0"}

	m := testMondooAuditConfig()
	m.Spec.KubernetesResources.Enable = true
	m.Spec.Containers.Enable = true
	m.Spec.Nodes.Enable = true

	m.Status.Conditions = []v1alpha2.MondooAuditConfigCondition{
		{Message: "Kubernetes Resources Scanning is available", Status: v1.ConditionFalse, Type: v1alpha2.K8sResourcesScanningDegraded},
		{Message: "Kubernetes Container Image Scanning is available", Status: v1.ConditionFalse, Type: v1alpha2.K8sContainerImageScanningDegraded},
		{Message: "Node Scanning is available", Status: v1.ConditionFalse, Type: v1alpha2.NodeScanningDegraded},
		{Message: "ScanAPI controller is degraded due to OOM", Status: v1.ConditionTrue, Type: v1alpha2.ScanAPIDegraded, AffectedPods: []string{"scanapi-1", "scanapi-2"}, MemoryLimit: "300Mi"},
	}

	reportStatus := ReportStatusRequestFromAuditConfig(integrationMrn, m, nodes, v, logger)
	assert.Equal(t, integrationMrn, reportStatus.Mrn)
	assert.Equal(t, mondooclient.Status_ERROR, reportStatus.Status)
	extraData := reportStatus.Messages.Messages[3].Extra.(*structpb.Struct)
	extraMap := extraData.AsMap()
	assert.Contains(t, extraMap, "errorCode")
	assert.Contains(t, extraMap, "affectedPods")
	assert.Contains(t, extraMap, "memoryLimit")
	assert.Contains(t, extraMap["errorCode"], "OOMKilled")
	assert.Contains(t, extraMap["affectedPods"], "scanapi-1")
	assert.Contains(t, extraMap["memoryLimit"], "300Mi")
}

func TestCreateOOMExtraInformation(t *testing.T) {
	// Test cases
	tests := []struct {
		name         string
		message      string
		affectedPods []string
		memoryLimit  string
		expected     *structpb.Struct
		expectedErr  error
	}{
		{
			name:         "Message ends with OOM",
			message:      "Container was terminated due to OOM",
			affectedPods: []string{"pod1", "pod2"},
			memoryLimit:  "1Gi",
			expected: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"errorCode": {
						Kind: &structpb.Value_StringValue{
							StringValue: "OOMKilled",
						},
					},
					"affectedPods": {
						Kind: &structpb.Value_StringValue{
							StringValue: "pod1, pod2",
						},
					},
					"memoryLimit": {
						Kind: &structpb.Value_StringValue{
							StringValue: "1Gi",
						},
					},
				},
			},
			expectedErr: nil,
		},
		{
			name:         "Message does not end with OOM",
			message:      "Container was terminated due to an error",
			affectedPods: []string{"pod1", "pod2"},
			memoryLimit:  "1Gi",
			expected:     nil,
			expectedErr:  nil,
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := createOOMExtraInformation(tt.message, tt.affectedPods, tt.memoryLimit)
			assert.Equal(t, tt.expectedErr, err)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestCreateOOMExtraInformationPBMap(t *testing.T) {
	// Test cases
	tests := []struct {
		name         string
		message      string
		affectedPods []string
		memoryLimit  string
		expected     map[string]interface{}
		expectedErr  error
	}{
		{
			name:         "Message ends with OOM",
			message:      "Container was terminated due to OOM",
			affectedPods: []string{"pod1", "pod2"},
			memoryLimit:  "1Gi",
			expected: map[string]interface{}{
				"errorCode":    "OOMKilled",
				"affectedPods": "pod1, pod2",
				"memoryLimit":  "1Gi",
			},
			expectedErr: nil,
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := createOOMExtraInformation(tt.message, tt.affectedPods, tt.memoryLimit)

			assert.Equal(t, tt.expectedErr, err)
			assert.Equal(t, tt.expected, actual.AsMap())
		})
	}
}
