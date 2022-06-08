package k8s_scan

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
)

func updateWorkloadsConditions(config *v1alpha2.MondooAuditConfig, degradedStatus bool) {
	msg := "API Scanning is Available"
	reason := "APIScanningAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	if degradedStatus {
		msg = "API Scanning is Unavailable"
		reason = "APIScanningUnavailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.K8sResourcesScanningDegraded, status, reason, msg, updateCheck)
}
