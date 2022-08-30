/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package admission

import (
	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
)

func updateAdmissionConditions(config *mondoov1alpha2.MondooAuditConfig, degradedStatus bool) {
	msg := "Admission controller is available"
	reason := "AdmissionAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	if !config.Spec.Admission.Enable {
		msg = "Admission controller is disabled"
		reason = "AdmissionDisabled"
		status = corev1.ConditionFalse
	} else if degradedStatus {
		msg = "Admission controller is unavailable"
		reason = "AdmissionUnvailable"
		status = corev1.ConditionTrue
		condition := mondoo.FindMondooAuditConditions(config.Status.Conditions, mondoov1alpha2.ScanAPIDegraded)
		if condition != nil && condition.Status == corev1.ConditionTrue {
			reason = "Scan API is unavailable"
		}
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(config.Status.Conditions, mondoov1alpha2.AdmissionDegraded, status, reason, msg, updateCheck)
}
