// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
)

const oomMessage = "Resource Watcher is unavailable due to OOM"

func updateResourceWatcherConditions(config *v1alpha2.MondooAuditConfig, degradedStatus bool, pods *corev1.PodList) {
	msg := "Resource Watcher is available"
	reason := "ResourceWatcherAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	affectedPods := []string{}
	memoryLimit := ""

	if !config.Spec.KubernetesResources.Enable || !config.Spec.KubernetesResources.ResourceWatcher.Enable {
		msg = "Resource Watcher is disabled"
		reason = "ResourceWatcherDisabled"
		status = corev1.ConditionFalse
	} else if degradedStatus {
		cond := mondoo.FindMondooAuditConditions(config.Status.Conditions, v1alpha2.ResourceWatcherDegraded)
		if cond != nil && cond.Status == corev1.ConditionTrue && cond.Message == oomMessage {
			// no need to update condition if it's already set to OOM. We should only update if it's back to active
			return
		}

		msg = "Resource Watcher is unavailable"
		reason = "ResourceWatcherUnavailable"
		status = corev1.ConditionTrue
	}

	currentPod := k8s.GetNewestPodFromList(pods.Items)
	for i, containerStatus := range currentPod.Status.ContainerStatuses {
		if containerStatus.Name != "mondoo-resource-watcher" {
			continue
		}
		if (containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137) ||
			(containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode == 137) {
			msg = oomMessage
			affectedPods = append(affectedPods, currentPod.Name)
			memoryLimit = currentPod.Spec.Containers[i].Resources.Limits.Memory().String()
			reason = "ResourceWatcherUnavailable"
			status = corev1.ConditionTrue
		}
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.ResourceWatcherDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}
