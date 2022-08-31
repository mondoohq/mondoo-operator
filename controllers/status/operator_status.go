/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package status

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mondoo-operator/pkg/version"
	v1 "k8s.io/api/core/v1"
	k8sversion "k8s.io/apimachinery/pkg/version"
)

const (
	K8sResourcesScanningIdentifier = "k8s-resources-scanning"
	NodeScanningIdentifier         = "node-scanning"
	AdmissionControllerIdentifier  = "admission-controller"
	ScanApiIdentifier              = "scan-api"
	noStatusMessage                = "No status yet reported'"
)

type OperatorCustomState struct {
	KubernetesVersion string
	Nodes             []string
	MondooAuditConfig MondooAuditConfig
}

type MondooAuditConfig struct {
	Name      string
	Namespace string
}

func ReportStatusRequestFromAuditConfig(
	integrationMrn string, m v1alpha2.MondooAuditConfig, nodes []v1.Node, k8sVersion *k8sversion.Info,
) mondooclient.ReportStatusRequest {
	nodeNames := make([]string, len(nodes))
	for i := range nodes {
		nodeNames[i] = nodes[i].Name
	}

	messages := make([]mondooclient.IntegrationMessage, 4)

	// Kubernetes resources scanning status
	messages[0].Identifier = K8sResourcesScanningIdentifier
	if m.Spec.KubernetesResources.Enable {
		k8sResourcesScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.K8sResourcesScanningDegraded)
		if k8sResourcesScanning != nil {
			if k8sResourcesScanning.Status == v1.ConditionTrue {
				messages[0].Status = mondooclient.MessageStatus_MESSAGE_ERROR
			} else {
				messages[0].Status = mondooclient.MessageStatus_MESSAGE_INFO
			}
			messages[0].Message = k8sResourcesScanning.Message
		} else {
			messages[0].Status = mondooclient.MessageStatus_MESSAGE_UNKNOWN
			messages[0].Message = noStatusMessage
		}
	} else {
		messages[0].Status = mondooclient.MessageStatus_MESSAGE_INFO
		messages[0].Message = "Kubernetes resources scanning is disabled"
	}

	// Node scanning status
	messages[1].Identifier = NodeScanningIdentifier
	if m.Spec.Nodes.Enable {
		nodeScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.NodeScanningDegraded)
		if nodeScanning != nil {
			if nodeScanning.Status == v1.ConditionTrue {
				messages[1].Status = mondooclient.MessageStatus_MESSAGE_ERROR
			} else {
				messages[1].Status = mondooclient.MessageStatus_MESSAGE_INFO
			}
			messages[1].Message = nodeScanning.Message
		} else {
			messages[1].Status = mondooclient.MessageStatus_MESSAGE_UNKNOWN
			messages[1].Message = noStatusMessage
		}
	} else {
		messages[1].Status = mondooclient.MessageStatus_MESSAGE_INFO
		messages[1].Message = "Node scanning is disabled"
	}

	// Admission controller status
	messages[2].Identifier = AdmissionControllerIdentifier
	if m.Spec.Admission.Enable {
		admissionControllerScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.AdmissionDegraded)
		if admissionControllerScanning != nil {
			if admissionControllerScanning.Status == v1.ConditionTrue {
				messages[2].Status = mondooclient.MessageStatus_MESSAGE_ERROR
			} else {
				messages[2].Status = mondooclient.MessageStatus_MESSAGE_INFO
			}
			messages[2].Message = admissionControllerScanning.Message
		} else {
			messages[2].Status = mondooclient.MessageStatus_MESSAGE_UNKNOWN
			messages[2].Message = noStatusMessage
		}
	} else {
		messages[2].Status = mondooclient.MessageStatus_MESSAGE_INFO
		messages[2].Message = "Admission controller is disabled"
	}

	messages[3].Identifier = ScanApiIdentifier
	if m.Spec.Admission.Enable || m.Spec.KubernetesResources.Enable {
		scanApi := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.ScanAPIDegraded)
		if scanApi != nil {
			if scanApi.Status == v1.ConditionTrue {
				messages[3].Status = mondooclient.MessageStatus_MESSAGE_ERROR
			} else {
				messages[3].Status = mondooclient.MessageStatus_MESSAGE_INFO
			}
			messages[3].Message = scanApi.Message
		} else {
			messages[3].Status = mondooclient.MessageStatus_MESSAGE_UNKNOWN
			messages[3].Message = noStatusMessage
		}
	} else {
		messages[3].Status = mondooclient.MessageStatus_MESSAGE_INFO
		messages[3].Message = "Scan API is disabled"
	}

	// If there were any error messages, the overall status is error
	status := mondooclient.Status_ACTIVE
	for _, m := range messages {
		if m.Status == mondooclient.MessageStatus_MESSAGE_ERROR {
			status = mondooclient.Status_ERROR
			break
		}
	}

	return mondooclient.ReportStatusRequest{
		Mrn:    integrationMrn,
		Status: status,
		LastState: OperatorCustomState{
			Nodes:             nodeNames,
			KubernetesVersion: k8sVersion.GitVersion,
			MondooAuditConfig: MondooAuditConfig{Name: m.Name, Namespace: m.Namespace},
		},
		Messages: mondooclient.Messages{Messages: messages},
		Version:  version.Version,
	}
}
