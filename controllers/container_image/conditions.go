// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package container_image

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
)

func updateImageScanningConditions(config *v1alpha2.MondooAuditConfig, degradedStatus bool) {
	msg := "Kubernetes Container Image Scanning is Available"
	reason := "KubernetesContainerImageScanningAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	if !config.Spec.KubernetesResources.ContainerImageScanning && !config.Spec.Containers.Enable {
		msg = "Kubernetes Container Image Scanning is disabled"
		reason = "KubernetesContainerImageScanningDisabled"
		status = corev1.ConditionFalse
	} else if degradedStatus {
		msg = "Kubernetes Container Image Scanning is Unavailable"
		reason = "KubernetesContainerImageScanningUnavailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.K8sContainerImageScanningDegraded, status, reason, msg, updateCheck, []string{}, "")
}
