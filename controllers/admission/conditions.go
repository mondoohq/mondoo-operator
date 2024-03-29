// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package admission

import (
	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
)

const oomMessage = "Admission controller is unavailable due to OOM"

func updateAdmissionConditions(config *mondoov1alpha2.MondooAuditConfig, degradedStatus bool, pods *corev1.PodList) {
	msg := "Admission controller is available"
	reason := "AdmissionAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	affectedPods := []string{}
	memoryLimit := ""
	if !config.Spec.Admission.Enable {
		msg = "Admission controller is disabled"
		reason = "AdmissionDisabled"
		status = corev1.ConditionFalse
	} else if degradedStatus {
		cond := mondoo.FindMondooAuditConditions(config.Status.Conditions, mondoov1alpha2.AdmissionDegraded)
		if cond != nil && cond.Status == corev1.ConditionTrue && cond.Message == oomMessage {
			// no need to update condition if it's already set to OOM. We should only update if it's back to active
			return
		}

		msg = "Admission controller is unavailable"
		reason = "AdmissionUnvailable"
		status = corev1.ConditionTrue
		condition := mondoo.FindMondooAuditConditions(config.Status.Conditions, mondoov1alpha2.ScanAPIDegraded)
		if condition != nil && condition.Status == corev1.ConditionTrue {
			reason = "Scan API is unavailable"
		}
	}

	currentPod := k8s.GetNewestPodFromList(pods.Items)
	for i, containerStatus := range currentPod.Status.ContainerStatuses {
		if containerStatus.Name != "webhook" {
			continue
		}
		if (containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137) ||
			(containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode == 137) {
			msg = oomMessage
			reason = "AdmissionUnvailable"
			status = corev1.ConditionTrue
			affectedPods = append(affectedPods, currentPod.Name)
			memoryLimit = currentPod.Spec.Containers[i].Resources.Limits.Memory().String()
			break
		}
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(config.Status.Conditions, mondoov1alpha2.AdmissionDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}
