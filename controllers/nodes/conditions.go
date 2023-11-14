// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"

	corev1 "k8s.io/api/core/v1"
)

func updateNodeConditions(config *v1alpha2.MondooAuditConfig, degradedStatus bool, pods *corev1.PodList) {
	msg := "Node Scanning is available"
	reason := "NodeScanningAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	affectedPods := []string{}
	memoryLimit := ""
	if !config.Spec.Nodes.Enable {
		msg = "Node Scanning is disabled"
		reason = "NodeScanningDisabled"
		status = corev1.ConditionFalse
	} else if degradedStatus {
		msg = "Node Scanning is unavailable"
		for _, pod := range pods.Items {
			for _, status := range pod.Status.ContainerStatuses {
				if status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode == 137 {
					// TODO: double check container name?
					msg = "Node Scanning is unavailable due to OOM"
					affectedPods = append(affectedPods, pod.Name)
					memoryLimit = pod.Spec.Containers[0].Resources.Limits.Memory().String()
					break
				}
			}
		}
		reason = "NodeScanningUnavailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.NodeScanningDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}
