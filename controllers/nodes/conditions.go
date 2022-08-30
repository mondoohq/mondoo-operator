/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package nodes

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"

	corev1 "k8s.io/api/core/v1"
)

func updateNodeConditions(config *v1alpha2.MondooAuditConfig, degradedStatus bool) {
	msg := "Node Scanning is available"
	reason := "NodeScanningAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	if !config.Spec.Nodes.Enable {
		msg = "Node Scanning is disabled"
		reason = "NodeScanningDisabled"
		status = corev1.ConditionFalse
	} else if degradedStatus {
		msg = "Node Scanning is unavailable"
		reason = "NodeScanningUnavailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.NodeScanningDegraded, status, reason, msg, updateCheck)
}
