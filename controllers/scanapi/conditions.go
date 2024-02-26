// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package scanapi

import (
	"regexp"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const oomMessage = "ScanAPI controller is unavailable due to OOM"

func updateScanAPIConditions(config *mondoov1alpha2.MondooAuditConfig, degradedStatus bool, conditions []appsv1.DeploymentCondition, pods *corev1.PodList) {
	msg := "ScanAPI controller is available"
	reason := "ScanAPIAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	affectedPods := []string{}
	memoryLimit := ""
	if !config.Spec.KubernetesResources.Enable && !config.Spec.Admission.Enable {
		msg = "ScanAPI is disabled"
		reason = "ScanAPIDisabled"
		status = corev1.ConditionFalse
	} else if degradedStatus {
		cond := mondoo.FindMondooAuditConditions(config.Status.Conditions, mondoov1alpha2.ScanAPIDegraded)
		if cond != nil && cond.Status == corev1.ConditionTrue && cond.Message == oomMessage {
			// no need to update condition if it's already set to OOM. We should only update if it's back to active
			return
		}

		reason = "ScanAPIUnavailable"
		status = corev1.ConditionTrue
		msg = "ScanAPI controller is unavailable"

		// perhaps more general ReplicaFailure?
		serviceAccountNotFound := regexp.MustCompile(`^.+serviceaccount ".+" not found$`)
		for _, condition := range conditions {
			if serviceAccountNotFound.MatchString(condition.Message) {
				msg = condition.Message
				break
			}
		}
	}

	currentPod := k8s.GetNewestPodFromList(pods.Items)
	logger.Info("ScanAPI controller is unavailable", " pod ", currentPod.Status.ContainerStatuses)
	for i, containerStatus := range currentPod.Status.ContainerStatuses {
		if containerStatus.Name != "cnspec" {
			continue
		}
		if (containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137) ||
			(containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode == 137) {
			msg = oomMessage
			reason = "ScanAPIUnavailable"
			status = corev1.ConditionTrue
			affectedPods = append(affectedPods, currentPod.Name)
			memoryLimit = currentPod.Spec.Containers[i].Resources.Limits.Memory().String()
		}
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(config.Status.Conditions, mondoov1alpha2.ScanAPIDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}
