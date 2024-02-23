// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
)

func updateWorkloadsConditions(config *v1alpha2.MondooAuditConfig, degradedStatus bool, pods *corev1.PodList) {
	msg := "Kubernetes Resources Scanning is available"
	reason := "KubernetesResourcesScanningAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	affectedPods := []string{}
	memoryLimit := ""
	if !config.Spec.KubernetesResources.Enable {
		msg = "Kubernetes Resources Scanning is disabled"
		reason = "KubernetesResourcesScanningDisabled"
		status = corev1.ConditionFalse
	} else if degradedStatus {
		msg = "Kubernetes Resources Scanning is unavailable"
		reason = "KubernetesResourcesScanningUnavailable"
		status = corev1.ConditionTrue
	}

	currentPod := k8s.GetNewestPodFromList(pods.Items)
	for i, containerStatus := range currentPod.Status.ContainerStatuses {
		if containerStatus.Name != "mondoo-k8s-scan" {
			continue
		}
		if (containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137) ||
			(containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode == 137) {
			msg = "Kubernetes Resources Scanning is unavailable due to OOM"
			affectedPods = append(affectedPods, currentPod.Name)
			memoryLimit = currentPod.Spec.Containers[i].Resources.Limits.Memory().String()
			reason = "KubernetesResourcesScanningUnavailable"
			status = corev1.ConditionTrue
		}
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.K8sResourcesScanningDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}
