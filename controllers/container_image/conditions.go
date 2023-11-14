// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package container_image

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
)

func updateImageScanningConditions(config *v1alpha2.MondooAuditConfig, degradedStatus bool, pods *corev1.PodList) {
	msg := "Kubernetes Container Image Scanning is available"
	reason := "KubernetesContainerImageScanningAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	affectedPods := []string{}
	memoryLimit := ""
	if !config.Spec.KubernetesResources.ContainerImageScanning && !config.Spec.Containers.Enable {
		msg = "Kubernetes Container Image Scanning is disabled"
		reason = "KubernetesContainerImageScanningDisabled"
		status = corev1.ConditionFalse
	} else if degradedStatus {
		msg = "Kubernetes Container Image Scanning is unavailable"
		for _, pod := range pods.Items {
			for _, status := range pod.Status.ContainerStatuses {
				if status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode == 137 {
					// TODO: double check container name?
					msg = "Kubernetes Container Image Scanning is unavailable due to OOM"
					affectedPods = append(affectedPods, pod.Name)
					memoryLimit = pod.Spec.Containers[0].Resources.Limits.Memory().String()
					break
				}
			}
		}
		reason = "KubernetesContainerImageScanningUnavailable"
		status = corev1.ConditionTrue
	}

	for _, pod := range pods.Items {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137 {
				// TODO: double check container name?
				msg = "Kubernetes Container Image Scanning is unavailable due to OOM"
				affectedPods = append(affectedPods, pod.Name)
				memoryLimit = pod.Spec.Containers[0].Resources.Limits.Memory().String()
				reason = "KubernetesContainerImageScanningUnavailable"
				status = corev1.ConditionTrue
			}
		}
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.K8sContainerImageScanningDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}
