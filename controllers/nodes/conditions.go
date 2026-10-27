// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"fmt"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"

	corev1 "k8s.io/api/core/v1"
)

const oomMessage = "Node Scanning is unavailable due to OOM"

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
		cond := mondoo.FindMondooAuditConditions(config.Status.Conditions, v1alpha2.NodeScanningDegraded)
		if cond != nil && cond.Status == corev1.ConditionTrue && cond.Message == oomMessage {
			// no need to update condition if it's already set to OOM. We should only update if it's back to active
			return
		}

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
		for _, containerStatus := range currentPod.Status.ContainerStatuses {
			if containerStatus.Name != "cnspec" {
				continue
			}
			if (containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137) ||
				(containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode == 137) {
				isOOM = true
				msg = oomMessage
				affectedPods = append(affectedPods, currentPod.Name)
				memoryLimit = k8s.ContainerMemoryLimit(&currentPod, containerStatus.Name)
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

	// A failing render-node-inventory init container keeps cnspec from ever starting. A
	// DaemonSet still counts such pods as scheduled, so report it explicitly. OOM wins.
	if len(affectedPods) == 0 && config.Spec.Nodes.Enable {
		for _, pods := range podsPerNode {
			currentPod := k8s.GetNewestPodFromList(pods)
			if failure := renderInitFailure(&currentPod); failure != "" {
				msg = fmt.Sprintf("Node Scanning is unavailable: init container %s failed (%s)", renderNodeInventoryContainerName, failure)
				affectedPods = append(affectedPods, currentPod.Name)
				reason = "NodeScanningUnavailable"
				status = corev1.ConditionTrue
			}
		}
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.NodeScanningDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}

// renderInitFailure returns why the render-node-inventory init container of pod is failing,
// or "" while it is running, starting normally or has succeeded.
func renderInitFailure(pod *corev1.Pod) string {
	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.Name != renderNodeInventoryContainerName {
			continue
		}
		if w := cs.State.Waiting; w != nil && w.Reason != "" && w.Reason != "PodInitializing" && w.Reason != "ContainerCreating" {
			return w.Reason
		}
		if t := cs.State.Terminated; t != nil && t.ExitCode != 0 {
			if t.Reason != "" {
				return t.Reason
			}
			return fmt.Sprintf("exit code %d", t.ExitCode)
		}
	}
	return ""
}
