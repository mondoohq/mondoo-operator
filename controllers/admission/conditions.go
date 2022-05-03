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
	if degradedStatus {
		msg = "Admission controller is Unavailable"
		reason = "AdmissionUnvailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(config.Status.Conditions, mondoov1alpha2.AdmissionDegraded, status, reason, msg, updateCheck)
}
