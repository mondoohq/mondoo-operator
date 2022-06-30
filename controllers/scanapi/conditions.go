package scanapi

import (
	"regexp"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func updateScanAPIConditions(config *mondoov1alpha2.MondooAuditConfig, degradedStatus bool, conditions []appsv1.DeploymentCondition) {
	msg := "ScanAPI controller is available"
	reason := "ScanAPIAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
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

		reason = "ScanAPIUnvailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(config.Status.Conditions, mondoov1alpha2.ScanAPIDegraded, status, reason, msg, updateCheck)
	if degradedStatus {
		// also admission is degraded
		for _, condition := range config.Status.Conditions {
			if condition.Type == mondoov1alpha2.AdmissionDegraded {
				condition.Status = status
				condition.Reason = reason
				condition.Message = msg
				config.Status.Conditions = mondoo.SetMondooAuditCondition(config.Status.Conditions, mondoov1alpha2.AdmissionDegraded, corev1.ConditionTrue, "Admission is degraded because the Scan API is degraded", msg, updateCheck)
				break
			}
		}
	}
}
