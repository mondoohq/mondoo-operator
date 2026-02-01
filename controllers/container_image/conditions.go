// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package container_image

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
)

const oomMessage = "Kubernetes Container Image Scanning is unavailable due to OOM"

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
		cond := mondoo.FindMondooAuditConditions(config.Status.Conditions, v1alpha2.K8sContainerImageScanningDegraded)
		if cond != nil && cond.Status == corev1.ConditionTrue && cond.Message == oomMessage {
			// no need to update condition if it's already set to OOM. We should only update if it's back to active
			return
		}

		msg = "Kubernetes Container Image Scanning is unavailable"
		reason = "KubernetesContainerImageScanningUnavailable"
		status = corev1.ConditionTrue
	}

	currentPod := k8s.GetNewestPodFromList(pods.Items)
	for i, containerStatus := range currentPod.Status.ContainerStatuses {
		if containerStatus.Name != "mondoo-containers-scan" {
			continue
		}
		if (containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137) ||
			(containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode == 137) {
			msg = oomMessage
			affectedPods = append(affectedPods, currentPod.Name)
			memoryLimit = currentPod.Spec.Containers[i].Resources.Limits.Memory().String()
			reason = "KubernetesContainerImageScanningUnavailable"
			status = corev1.ConditionTrue
		}
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.K8sContainerImageScanningDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}
