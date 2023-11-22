// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package container_image

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
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
		reason = "KubernetesContainerImageScanningUnavailable"
		status = corev1.ConditionTrue
	}

	currentPod := k8s.GetNewestPodFromList(pods)
	for i, containerStatus := range currentPod.Status.ContainerStatuses {
		if containerStatus.Name != "mondoo-containers-scan" {
			continue
		}
		if (containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137) ||
			(containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode == 137) {
			msg = "Kubernetes Container Image Scanning is unavailable due to OOM"
			affectedPods = append(affectedPods, currentPod.Name)
			memoryLimit = currentPod.Spec.Containers[i].Resources.Limits.Memory().String()
			reason = "KubernetesContainerImageScanningUnavailable"
			status = corev1.ConditionTrue
		}
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.K8sContainerImageScanningDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}
