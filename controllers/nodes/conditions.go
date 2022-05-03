package nodes

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
)

func updateNodeConditions(config *v1alpha2.MondooAuditConfig, degradedStatus bool) {
	msg := "Node Scanning is Available"
	reason := "NodeScanningAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	if degradedStatus {
		msg = "Node Scanning is Unavailable"
		reason = "NodeScanningUnavailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.NodeScanningDegraded, status, reason, msg, updateCheck)
}
