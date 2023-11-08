// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package status

import (
	"strings"

	"github.com/go-logr/logr"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mondoo-operator/pkg/version"
	"google.golang.org/protobuf/types/known/structpb"
	v1 "k8s.io/api/core/v1"
	k8sversion "k8s.io/apimachinery/pkg/version"
)

const (
	K8sResourcesScanningIdentifier   = "k8s-resources-scanning"
	ContainerImageScanningIdentifier = "container-image-scanning"
	NodeScanningIdentifier           = "node-scanning"
	AdmissionControllerIdentifier    = "admission-controller"
	ScanApiIdentifier                = "scan-api"
	NamespaceFilteringIdentifier     = "namespace-filtering"
	noStatusMessage                  = "No status reported yet"
)

type OperatorCustomState struct {
	KubernetesVersion      string
	Nodes                  []string
	MondooAuditConfig      MondooAuditConfig
	OperatorVersion        string
	K8sResourcesScanning   bool
	ContainerImageScanning bool
	NodeScanning           bool
	AdmissionController    bool
	FilteringConfig        v1alpha2.Filtering
}

type MondooAuditConfig struct {
	Name      string
	Namespace string
}

type K8sIntegrationStatusMessageExtra struct {
	ErrorCode    string   `json:"errorCode,omitempty"`
	AffectedPods []string `json:"affectedPods,omitempty"`
	MemoryLimit  string   `json:"memoryLimit,omitempty"`
}

func ReportStatusRequestFromAuditConfig(
	integrationMrn string, m v1alpha2.MondooAuditConfig, nodes []v1.Node, k8sVersion *k8sversion.Info, log logr.Logger,
) mondooclient.ReportStatusRequest {
	nodeNames := make([]string, len(nodes))
	for i := range nodes {
		nodeNames[i] = nodes[i].Name
	}

	messages := make([]mondooclient.IntegrationMessage, 5)

	// Kubernetes resources scanning status
	messages[0].Identifier = K8sResourcesScanningIdentifier
	if m.Spec.KubernetesResources.Enable {
		k8sResourcesScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.K8sResourcesScanningDegraded)
		if k8sResourcesScanning != nil {
			if k8sResourcesScanning.Status == v1.ConditionTrue {
				messages[0].Status = mondooclient.MessageStatus_MESSAGE_ERROR
				extraStruct, err := createOOMExtraInformation(k8sResourcesScanning.Message, k8sResourcesScanning.AffectedPods, k8sResourcesScanning.MemoryLimit)
				if err != nil {
					log.Error(err, "Failed to create extra information for Kubernetes Resource Scanning on OOM error")
				}
				if extraStruct != nil {
					messages[4].Extra = extraStruct
				}
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

	// Container image scanning status
	messages[1].Identifier = ContainerImageScanningIdentifier
	if m.Spec.KubernetesResources.ContainerImageScanning || m.Spec.Containers.Enable {
		containerImageScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.K8sContainerImageScanningDegraded)
		if containerImageScanning != nil {
			if containerImageScanning.Status == v1.ConditionTrue {
				messages[1].Status = mondooclient.MessageStatus_MESSAGE_ERROR
				extraStruct, err := createOOMExtraInformation(containerImageScanning.Message, containerImageScanning.AffectedPods, containerImageScanning.MemoryLimit)
				if err != nil {
					log.Error(err, "Failed to create extra information for Kubernetes Container Image on OOM error")
				}
				if extraStruct != nil {
					messages[4].Extra = extraStruct
				}
			} else {
				messages[1].Status = mondooclient.MessageStatus_MESSAGE_INFO
			}
			messages[1].Message = containerImageScanning.Message
		} else {
			messages[1].Status = mondooclient.MessageStatus_MESSAGE_UNKNOWN
			messages[1].Message = noStatusMessage
		}
	} else {
		messages[1].Status = mondooclient.MessageStatus_MESSAGE_INFO
		messages[1].Message = "Container image scanning is disabled"
	}

	// Node scanning status
	messages[2].Identifier = NodeScanningIdentifier
	if m.Spec.Nodes.Enable {
		nodeScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.NodeScanningDegraded)
		if nodeScanning != nil {
			if nodeScanning.Status == v1.ConditionTrue {
				messages[2].Status = mondooclient.MessageStatus_MESSAGE_ERROR
				extraStruct, err := createOOMExtraInformation(nodeScanning.Message, nodeScanning.AffectedPods, nodeScanning.MemoryLimit)
				if err != nil {
					log.Error(err, "Failed to create extra information for Node Scanning on OOM error")
				}
				if extraStruct != nil {
					messages[4].Extra = extraStruct
				}
			} else {
				messages[2].Status = mondooclient.MessageStatus_MESSAGE_INFO
			}
			messages[2].Message = nodeScanning.Message
		} else {
			messages[2].Status = mondooclient.MessageStatus_MESSAGE_UNKNOWN
			messages[2].Message = noStatusMessage
		}
	} else {
		messages[2].Status = mondooclient.MessageStatus_MESSAGE_INFO
		messages[2].Message = "Node scanning is disabled"
	}

	// Admission controller status
	messages[3].Identifier = AdmissionControllerIdentifier
	if m.Spec.Admission.Enable {
		admissionControllerScanning := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.AdmissionDegraded)
		if admissionControllerScanning != nil {
			if admissionControllerScanning.Status == v1.ConditionTrue {
				messages[3].Status = mondooclient.MessageStatus_MESSAGE_ERROR
				extraStruct, err := createOOMExtraInformation(admissionControllerScanning.Message, admissionControllerScanning.AffectedPods, admissionControllerScanning.MemoryLimit)
				if err != nil {
					log.Error(err, "Failed to create extra information for Admission Controller on OOM error")
				}
				if extraStruct != nil {
					messages[4].Extra = extraStruct
				}
			} else {
				messages[3].Status = mondooclient.MessageStatus_MESSAGE_INFO
			}
			messages[3].Message = admissionControllerScanning.Message
		} else {
			messages[3].Status = mondooclient.MessageStatus_MESSAGE_UNKNOWN
			messages[3].Message = noStatusMessage
		}
	} else {
		messages[3].Status = mondooclient.MessageStatus_MESSAGE_INFO
		messages[3].Message = "Admission controller is disabled"
	}

	messages[4].Identifier = ScanApiIdentifier
	if m.Spec.Admission.Enable || m.Spec.KubernetesResources.Enable {
		scanApi := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.ScanAPIDegraded)
		if scanApi != nil {
			if scanApi.Status == v1.ConditionTrue {
				messages[4].Status = mondooclient.MessageStatus_MESSAGE_ERROR
				extraStruct, err := createOOMExtraInformation(scanApi.Message, scanApi.AffectedPods, scanApi.MemoryLimit)
				if err != nil {
					log.Error(err, "Failed to create extra information for Scan API on OOM error")
				}
				if extraStruct != nil {
					messages[4].Extra = extraStruct
				}
			} else {
				messages[4].Status = mondooclient.MessageStatus_MESSAGE_INFO
			}
			messages[4].Message = scanApi.Message
		} else {
			messages[4].Status = mondooclient.MessageStatus_MESSAGE_UNKNOWN
			messages[4].Message = noStatusMessage
		}
	} else {
		messages[4].Status = mondooclient.MessageStatus_MESSAGE_INFO
		messages[4].Message = "Scan API is disabled"
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
			Nodes:                  nodeNames,
			KubernetesVersion:      k8sVersion.GitVersion,
			MondooAuditConfig:      MondooAuditConfig{Name: m.Name, Namespace: m.Namespace},
			OperatorVersion:        version.Version,
			K8sResourcesScanning:   m.Spec.KubernetesResources.Enable,
			ContainerImageScanning: m.Spec.Containers.Enable || m.Spec.KubernetesResources.ContainerImageScanning,
			NodeScanning:           m.Spec.Nodes.Enable,
			AdmissionController:    m.Spec.Admission.Enable,
			FilteringConfig:        m.Spec.Filtering,
		},
		Messages: mondooclient.Messages{Messages: messages},
	}
}

func createOOMExtraInformation(message string, affectedPods []string, memoryLimit string) (*structpb.Struct, error) {
	var pbStruct *structpb.Struct
	var err error
	if strings.HasSuffix(message, " OOM") {
		pbStruct, err = structpb.NewStruct(map[string]interface{}{
			"errorCode":    "OOMKilled",
			"affectedPods": strings.Join(affectedPods, ", "),
			"memoryLimit":  memoryLimit,
		})
		if err != nil {
			return nil, err
		}
	}
	return pbStruct, nil
}
