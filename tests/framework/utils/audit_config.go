package utils

import (
	mondoov2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const MondooClientSecret = "mondoo-client"

// DefaultAuditConfigMinimal returns a new Mondoo audit config with minimal default settings to
// make sure a test passes (e.g. setting the correct secret name). Values which have defaults are not set.
// This means that using this function in unit tests might result in strange behavior. For unit tests use
// DefaultAuditConfig instead.
func DefaultAuditConfigMinimal(ns string, workloads, nodes, admission bool) mondoov2.MondooAuditConfig {
	return mondoov2.MondooAuditConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: ns,
		},
		Spec: mondoov2.MondooAuditConfigSpec{
			MondooCredsSecretRef: corev1.LocalObjectReference{Name: MondooClientSecret},
			KubernetesResources:  mondoov2.KubernetesResources{Enable: workloads},
			Nodes:                mondoov2.Nodes{Enable: nodes},
			Admission:            mondoov2.Admission{Enable: admission},
		},
	}
}

// DefaultAuditConfig returns a new Mondoo audit config with some default settings to
// make sure a tests passes (e.g. setting the correct secret name).
func DefaultAuditConfig(ns string, workloads, nodes, admission bool) mondoov2.MondooAuditConfig {
	return mondoov2.MondooAuditConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: ns,
		},
		Spec: mondoov2.MondooAuditConfigSpec{
			MondooCredsSecretRef: corev1.LocalObjectReference{Name: MondooClientSecret},
			KubernetesResources:  mondoov2.KubernetesResources{Enable: workloads},
			Nodes:                mondoov2.Nodes{Enable: nodes},
			Admission:            mondoov2.Admission{Enable: admission},
			Scanner: mondoov2.Scanner{
				Image:              mondoov2.Image{Name: "test", Tag: "latest"},
				ServiceAccountName: "mondoo-operator-k8s-resources-scanning",
			},
		},
	}
}
