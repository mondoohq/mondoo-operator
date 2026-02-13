// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package status

import (
	"context"
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
	NamespaceFilteringIdentifier     = "namespace-filtering"
	MondooOperatorIdentifier         = "mondoo-operator"
	noStatusMessage                  = "No status reported yet"
)

type OperatorCustomState struct {
	KubernetesVersion      string
	Nodes                  []string
	MondooAuditConfig      MondooAuditConfig
	OperatorVersion        string
	OperatorImageDigest    string
	CnspecVersion          string
	CnspecImageDigest      string
	K8sResourcesScanning   bool
	ContainerImageScanning bool
	NodeScanning           bool
	FilteringConfig        v1alpha2.Filtering
}

type MondooAuditConfig struct {
	Name      string
	Namespace string
}

func ReportStatusRequestFromAuditConfig(
	ctx context.Context, integrationMrn string, m v1alpha2.MondooAuditConfig, nodes []v1.Node, k8sVersion *k8sversion.Info, containerImageResolver mondoo.ContainerImageResolver, log logr.Logger,
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
				extraStruct, err := createOOMExtraInformation(k8sResourcesScanning.Message, k8sResourcesScanning.AffectedPods, k8sResourcesScanning.MemoryLimit)
				if err != nil {
					log.Error(err, "Failed to create extra information for Kubernetes Resource Scanning on OOM error")
				}
				if extraStruct != nil {
					messages[0].Extra = extraStruct
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
					messages[1].Extra = extraStruct
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
					messages[2].Extra = extraStruct
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

	messages[3].Identifier = MondooOperatorIdentifier
	mondooOperator := mondoo.FindMondooAuditConditions(m.Status.Conditions, v1alpha2.MondooOperatorDegraded)
	if mondooOperator != nil {
		if mondooOperator.Status == v1.ConditionTrue {
			messages[3].Status = mondooclient.MessageStatus_MESSAGE_ERROR
			extraStruct, err := createOOMExtraInformation(mondooOperator.Message, mondooOperator.AffectedPods, mondooOperator.MemoryLimit)
			if err != nil {
				log.Error(err, "Failed to create extra information for Mondoo Operator on OOM error")
			}
			if extraStruct != nil {
				messages[3].Extra = extraStruct
			}
		} else {
			messages[3].Status = mondooclient.MessageStatus_MESSAGE_INFO
		}
		messages[3].Message = mondooOperator.Message
	} else {
		messages[3].Status = mondooclient.MessageStatus_MESSAGE_UNKNOWN
		messages[3].Message = noStatusMessage
	}

	// If there were any error messages, the overall status is error
	status := mondooclient.Status_ACTIVE
	for _, m := range messages {
		if m.Status == mondooclient.MessageStatus_MESSAGE_ERROR {
			status = mondooclient.Status_ERROR
			break
		}
	}

	// Resolve cnspec and operator images to get version and digest
	var cnspecVersion, cnspecImageDigest, operatorImageDigest string
	if containerImageResolver != nil {
		resolvedImage, err := containerImageResolver.CnspecImage(m.Spec.Scanner.Image.Name, m.Spec.Scanner.Image.Tag, m.Spec.Scanner.Image.Digest, false)
		if err != nil {
			log.Error(err, "Failed to resolve cnspec image for status reporting")
		} else {
			// Extract digest from resolved image (format: image@sha256:...)
			if idx := strings.Index(resolvedImage, "@"); idx != -1 {
				cnspecImageDigest = resolvedImage[idx+1:]
			}
		}
		// Get the tag separately (without resolution) for the version field
		tagImage, _ := containerImageResolver.CnspecImage(m.Spec.Scanner.Image.Name, m.Spec.Scanner.Image.Tag, m.Spec.Scanner.Image.Digest, true)
		if idx := strings.LastIndex(tagImage, ":"); idx != -1 {
			cnspecVersion = tagImage[idx+1:]
		}

		// Resolve operator image digest
		resolvedOperator, err := containerImageResolver.MondooOperatorImage(ctx, "", "", "", false)
		if err != nil {
			log.Error(err, "Failed to resolve operator image for status reporting")
		} else {
			if idx := strings.Index(resolvedOperator, "@"); idx != -1 {
				operatorImageDigest = resolvedOperator[idx+1:]
			}
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
			OperatorImageDigest:    operatorImageDigest,
			CnspecVersion:          cnspecVersion,
			CnspecImageDigest:      cnspecImageDigest,
			K8sResourcesScanning:   m.Spec.KubernetesResources.Enable,
			ContainerImageScanning: m.Spec.Containers.Enable || m.Spec.KubernetesResources.ContainerImageScanning,
			NodeScanning:           m.Spec.Nodes.Enable,
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
