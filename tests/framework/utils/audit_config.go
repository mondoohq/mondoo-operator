package utils

import (
	mondoov1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const MondooClientSecret = "mondoo-client"

// DefaultAuditConfig returns a new Mondoo audit config with some default settings to
// make sure a tests passes (e.g. setting the correct secret name).
func DefaultAuditConfig(ns string, workloads, nodes, webhooks bool) mondoov1.MondooAuditConfig {
	return mondoov1.MondooAuditConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: ns,
		},
		Spec: mondoov1.MondooAuditConfigData{
			Workloads:       mondoov1.Workloads{Enable: workloads},
			Nodes:           mondoov1.Nodes{Enable: nodes},
			Webhooks:        mondoov1.Webhooks{Enable: webhooks},
			MondooSecretRef: MondooClientSecret,
		},
	}
}
