// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
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
		reason = "NodeScanningUnavailable"
		status = corev1.ConditionTrue
	}

	podsPerNode := map[string][]corev1.Pod{}
	for _, pod := range pods.Items {
		podsPerNode[pod.Spec.NodeName] = append(podsPerNode[pod.Spec.NodeName], pod)
	}

	// Check if the latest pod for each node is not OOM
	for _, pods := range podsPerNode {
		currentPod := k8s.GetNewestPodFromList(pods)
		isOOM := false
		for i, containerStatus := range currentPod.Status.ContainerStatuses {
			if containerStatus.Name != "cnspec" {
				continue
			}
			if (containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137) ||
				(containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode == 137) {
				isOOM = true
				msg = "Node Scanning is unavailable due to OOM"
				affectedPods = append(affectedPods, currentPod.Name)
				memoryLimit = currentPod.Spec.Containers[i].Resources.Limits.Memory().String()
				reason = "NodeScanningUnavailable"
				status = corev1.ConditionTrue
				break
			}
		}

		// If there is at least 1 pod that is OOM, then we report OOM
		if isOOM {
			break
		}
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.NodeScanningDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}
