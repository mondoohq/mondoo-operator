// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
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
		for _, pod := range pods.Items {
			for _, status := range pod.Status.ContainerStatuses {
				if status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode == 137 {
					// TODO: double check container name?
					msg = "Kubernetes Resources Scanning is unavailable due to OOM"
					affectedPods = append(affectedPods, pod.Name)
					memoryLimit = pod.Spec.Containers[0].Resources.Limits.Memory().String()
					break
				}
			}
		}
		reason = "KubernetesResourcesScanningUnavailable"
		status = corev1.ConditionTrue
	}

	for _, pod := range pods.Items {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137 {
				// TODO: double check container name?
				msg = "Kubernetes Resources Scanning is unavailable due to OOM"
				affectedPods = append(affectedPods, pod.Name)
				memoryLimit = pod.Spec.Containers[0].Resources.Limits.Memory().String()
				reason = "KubernetesResourcesScanningUnavailable"
				status = corev1.ConditionTrue
			}
		}
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.K8sResourcesScanningDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}
