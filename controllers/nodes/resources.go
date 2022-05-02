package nodes

import "go.mondoo.com/mondoo-operator/api/v1alpha2"

func CronJobLabels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo-scan-api",
		"mondoo_cr": m.Name,
	}
}
