package scanapi

import (
	"fmt"
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
	if degradedStatus {
		fmt.Println("degraded state")
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
}
