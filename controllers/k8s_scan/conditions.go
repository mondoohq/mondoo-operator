/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package k8s_scan

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
)

func updateWorkloadsConditions(config *v1alpha2.MondooAuditConfig, degradedStatus bool) {
	msg := "Kubernetes Resources Scanning is Available"
	reason := "KubernetesResourcesScanningAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	if !config.Spec.KubernetesResources.Enable {
		msg = "Kubernetes Resources Scanning is disabled"
		reason = "KubernetesResourcesScanningDisabled"
		status = corev1.ConditionFalse
	} else if degradedStatus {
		msg = "Kubernetes Resources Scanning is Unavailable"
		reason = "KubernetesResourcesScanningUnavailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.K8sResourcesScanningDegraded, status, reason, msg, updateCheck)
}
