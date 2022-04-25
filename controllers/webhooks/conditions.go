package webhooks

import (
	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
)

func updateWebhooksConditions(config *mondoov1alpha2.MondooAuditConfig, degradedStatus bool) {
	msg := "Webhook is available"
	reason := "WebhookAailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	if degradedStatus {
		msg = "Webhook is Unavailable"
		reason = "WebhhookUnvailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(config.Status.Conditions, mondoov1alpha2.WebhookDegraded, status, reason, msg, updateCheck)
}
