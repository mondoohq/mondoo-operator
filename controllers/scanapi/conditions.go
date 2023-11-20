// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package scanapi

import (
	"regexp"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

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
		msg = "ScanAPI controller is unavailable"

		// perhaps more general ReplicaFailure?
		serviceAccountNotFound := regexp.MustCompile(`^.+serviceaccount ".+" not found$`)
		for _, condition := range conditions {
			if serviceAccountNotFound.MatchString(condition.Message) {
				msg = condition.Message
				break
			}
		}

		for _, pod := range pods.Items {
			for i, containerStatus := range pod.Status.ContainerStatuses {
				if containerStatus.Name != "cnspec" {
					continue
				}
				if (containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137) ||
					(containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode == 137) {
					msg = "ScanAPI controller is unavailable due to OOM"
					affectedPods = append(affectedPods, pod.Name)
					memoryLimit = pod.Spec.Containers[i].Resources.Limits.Memory().String()
				}
			}
		}

		reason = "ScanAPIUnvailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(config.Status.Conditions, mondoov1alpha2.ScanAPIDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}
