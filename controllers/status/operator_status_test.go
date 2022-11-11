/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sversion "k8s.io/apimachinery/pkg/version"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/version"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
)

func TestReportStatusRequestFromAuditConfig_AllDisabled(t *testing.T) {
	integrationMrn := utils.RandString(10)
	nodes := []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
	}
	v := &k8sversion.Info{GitVersion: "v1.24.0"}

	m := testMondooAuditConfig()
	reportStatus := ReportStatusRequestFromAuditConfig(integrationMrn, m, nodes, v)
	assert.Equal(t, integrationMrn, reportStatus.Mrn)
	assert.Equal(t, mondooclient.Status_ACTIVE, reportStatus.Status)
	assert.Equal(t, OperatorCustomState{
		Nodes:             []string{"node1", "node2"},
		KubernetesVersion: v.GitVersion,
		MondooAuditConfig: MondooAuditConfig{Name: m.Name, Namespace: m.Namespace},
		OperatorVersion:   version.Version,
	}, reportStatus.LastState)
	messages := []mondooclient.IntegrationMessage{
		{Identifier: K8sResourcesScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: "Kubernetes resources scanning is disabled"},
		{Identifier: NodeScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: "Node scanning is disabled"},
		{Identifier: AdmissionControllerIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: "Admission controller is disabled"},
		{Identifier: ScanApiIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: "Scan API is disabled"},
	}
	assert.ElementsMatch(t, messages, reportStatus.Messages.Messages)
}

func TestReportStatusRequestFromAuditConfig_AllEnabled(t *testing.T) {
	integrationMrn := utils.RandString(10)
	nodes := []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
	}
	v := &k8sversion.Info{GitVersion: "v1.24.0"}

	m := testMondooAuditConfig()
	m.Spec.KubernetesResources.Enable = true
	m.Spec.Nodes.Enable = true
	m.Spec.Admission.Enable = true

	m.Status.Conditions = []v1alpha2.MondooAuditConfigCondition{
		{Message: "Kubernetes Resources Scanning is Available", Status: v1.ConditionFalse, Type: v1alpha2.K8sResourcesScanningDegraded},
		{Message: "Node Scanning is available", Status: v1.ConditionFalse, Type: v1alpha2.NodeScanningDegraded},
		{Message: "Admission controller is available", Status: v1.ConditionFalse, Type: v1alpha2.AdmissionDegraded},
		{Message: "ScanAPI controller is available", Status: v1.ConditionFalse, Type: v1alpha2.ScanAPIDegraded},
	}

	reportStatus := ReportStatusRequestFromAuditConfig(integrationMrn, m, nodes, v)
	assert.Equal(t, integrationMrn, reportStatus.Mrn)
	assert.Equal(t, mondooclient.Status_ACTIVE, reportStatus.Status)
	assert.Equal(t, OperatorCustomState{
		Nodes:             []string{"node1", "node2"},
		KubernetesVersion: v.GitVersion,
		MondooAuditConfig: MondooAuditConfig{Name: m.Name, Namespace: m.Namespace},
		OperatorVersion:   version.Version,
	}, reportStatus.LastState)
	messages := []mondooclient.IntegrationMessage{
		{Identifier: K8sResourcesScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[0].Message},
		{Identifier: NodeScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[1].Message},
		{Identifier: AdmissionControllerIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[2].Message},
		{Identifier: ScanApiIdentifier, Status: mondooclient.MessageStatus_MESSAGE_INFO, Message: m.Status.Conditions[3].Message},
	}
	assert.ElementsMatch(t, messages, reportStatus.Messages.Messages)
}

func TestReportStatusRequestFromAuditConfig_AllError(t *testing.T) {
	integrationMrn := utils.RandString(10)
	nodes := []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
	}
	v := &k8sversion.Info{GitVersion: "v1.24.0"}

	m := testMondooAuditConfig()
	m.Spec.KubernetesResources.Enable = true
	m.Spec.Nodes.Enable = true
	m.Spec.Admission.Enable = true

	m.Status.Conditions = []v1alpha2.MondooAuditConfigCondition{
		{Message: "Kubernetes Resources Scanning error", Status: v1.ConditionTrue, Type: v1alpha2.K8sResourcesScanningDegraded},
		{Message: "Node Scanning error", Status: v1.ConditionTrue, Type: v1alpha2.NodeScanningDegraded},
		{Message: "Admission controller error", Status: v1.ConditionTrue, Type: v1alpha2.AdmissionDegraded},
		{Message: "ScanAPI controller error", Status: v1.ConditionTrue, Type: v1alpha2.ScanAPIDegraded},
	}

	reportStatus := ReportStatusRequestFromAuditConfig(integrationMrn, m, nodes, v)
	assert.Equal(t, integrationMrn, reportStatus.Mrn)
	assert.Equal(t, mondooclient.Status_ERROR, reportStatus.Status)
	assert.Equal(t, OperatorCustomState{
		Nodes:             []string{"node1", "node2"},
		KubernetesVersion: v.GitVersion,
		MondooAuditConfig: MondooAuditConfig{Name: m.Name, Namespace: m.Namespace},
		OperatorVersion:   version.Version,
	}, reportStatus.LastState)
	messages := []mondooclient.IntegrationMessage{
		{Identifier: K8sResourcesScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_ERROR, Message: m.Status.Conditions[0].Message},
		{Identifier: NodeScanningIdentifier, Status: mondooclient.MessageStatus_MESSAGE_ERROR, Message: m.Status.Conditions[1].Message},
		{Identifier: AdmissionControllerIdentifier, Status: mondooclient.MessageStatus_MESSAGE_ERROR, Message: m.Status.Conditions[2].Message},
		{Identifier: ScanApiIdentifier, Status: mondooclient.MessageStatus_MESSAGE_ERROR, Message: m.Status.Conditions[3].Message},
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
