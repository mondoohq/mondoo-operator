// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package runtime_cache

import (
	"sort"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
)

const oomMessage = "Runtime Cache Scanning is unavailable due to OOM"

func updateRuntimeCacheConditions(config *v1alpha2.MondooAuditConfig, degradedStatus bool, pods *corev1.PodList, reasonOverride, messageOverride string) {
	msg := "Runtime Cache Scanning is available"
	reason := "RuntimeCacheScanningAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	affectedPods := []string{}
	memoryLimit := ""

	if !config.Spec.Containers.RuntimeCache.Enable {
		msg = "Runtime Cache Scanning is disabled"
		reason = "RuntimeCacheScanningDisabled"
		status = corev1.ConditionFalse
	} else if reasonOverride != "" {
		msg = messageOverride
		reason = reasonOverride
		status = corev1.ConditionTrue
	} else if degradedStatus {
		msg = "Runtime Cache Scanning is unavailable"
		reason = "RuntimeCacheScanningUnavailable"
		status = corev1.ConditionTrue
	}

	if config.Spec.Containers.RuntimeCache.Enable {
		for _, pod := range pods.Items {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if containerStatus.Name != "mondoo-runtime-cache-scan" || !isOOMKilled(containerStatus) {
					continue
				}
				msg = oomMessage
				affectedPods = append(affectedPods, pod.Name)
				if memoryLimit == "" {
					memoryLimit = containerMemoryLimit(pod, containerStatus.Name)
				}
				reason = "RuntimeCacheScanningUnavailable"
				status = corev1.ConditionTrue
				updateCheck = mondoo.UpdateConditionAlways
			}
		}
		sort.Strings(affectedPods)
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.RuntimeCacheScanningDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}

func isOOMKilled(containerStatus corev1.ContainerStatus) bool {
	return (containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137) ||
		(containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode == 137)
}

func containerMemoryLimit(pod corev1.Pod, containerName string) string {
	for _, container := range pod.Spec.Containers {
		if container.Name != containerName {
			continue
		}
		return container.Resources.Limits.Memory().String()
	}
	return ""
}
