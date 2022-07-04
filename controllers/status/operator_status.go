/*
Copyright 2022 Mondoo, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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

type OperatorCustomState struct {
	KubernetesVersion string
	Nodes             []string
}

func ReportStatusRequestFromAuditConfig(
	integrationMrn string, m v1alpha2.MondooAuditConfig, nodes []v1.Node, k8sVersion *k8sversion.Info) mondooclient.ReportStatusRequest {
	nodeNames := make([]string, len(nodes))
	for i := range nodes {
		nodeNames[i] = nodes[i].Name
	}

	messages := make([]mondooclient.IntegrationMessage, 3)

	// Kubernetes resources scanning status
	messages[0].Identifier = "k8s-resources-scanning"
	if m.Spec.KubernetesResources.Enable {
		k8sResourcesScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.K8sResourcesScanningDegraded)
		if k8sResourcesScanning.Status == v1.ConditionTrue {
			messages[0].Status = mondooclient.MessageStatus_MESSAGE_ERROR
		} else {
			messages[0].Status = mondooclient.MessageStatus_MESSAGE_INFO
		}
		messages[0].Message = k8sResourcesScanning.Message
	} else {
		messages[0].Status = mondooclient.MessageStatus_MESSAGE_INFO
		messages[0].Message = "Kubernetes resources scanning is disabled"
	}

	// Node scanning status
	messages[1].Identifier = "node-scanning"
	if m.Spec.Nodes.Enable {
		nodeScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.NodeScanningDegraded)
		if nodeScanning.Status == v1.ConditionTrue {
			messages[1].Status = mondooclient.MessageStatus_MESSAGE_ERROR
		} else {
			messages[1].Status = mondooclient.MessageStatus_MESSAGE_INFO
		}
		messages[1].Message = nodeScanning.Message
	} else {
		messages[1].Status = mondooclient.MessageStatus_MESSAGE_INFO
		messages[1].Message = "Node scanning is disabled"
	}

	// Admission controller status
	messages[2].Identifier = "admission-controller"
	if m.Spec.Admission.Enable {
		admissionControllerScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.AdmissionDegraded)
		if admissionControllerScanning.Status == v1.ConditionTrue {
			messages[2].Status = mondooclient.MessageStatus_MESSAGE_ERROR
		} else {
			messages[2].Status = mondooclient.MessageStatus_MESSAGE_INFO
		}
		messages[2].Message = admissionControllerScanning.Message
	} else {
		messages[2].Status = mondooclient.MessageStatus_MESSAGE_INFO
		messages[2].Message = "Admission controller is disabled"
	}

	// If there were any error messages, the overall status is error
	status := mondooclient.Status_ACTIVE
	for _, m := range messages {
		if m.Status == mondooclient.MessageStatus_MESSAGE_ERROR {
			status = mondooclient.Status_ERROR
		}
	}

	return mondooclient.ReportStatusRequest{
		Mrn:       integrationMrn,
		Status:    status,
		LastState: OperatorCustomState{Nodes: nodeNames, KubernetesVersion: k8sVersion.GitVersion},
		Messages:  messages,
		Version:   version.Version,
	}
}
