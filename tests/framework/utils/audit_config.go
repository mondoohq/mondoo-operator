package utils

import (
	"os"

	mondoov2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

const (
	MondooClientSecret         = "mondoo-client"
	MondooTokenSecret          = "mondoo-token"
	MondooClientImageTagEnvVar = "MONDOO_CLIENT_IMAGE_TAG"
)

var mondooClientImageTag = ""

func init() {
	imageTag, ok := os.LookupEnv(MondooClientImageTagEnvVar)
	if ok {
		mondooClientImageTag = imageTag
	}
}

// DefaultAuditConfigMinimal returns a new Mondoo audit config with minimal default settings to
// make sure a test passes (e.g. setting the correct secret name). Values which have defaults are not set.
// This means that using this function in unit tests might result in strange behavior. For unit tests use
// DefaultAuditConfig instead.
func DefaultAuditConfigMinimal(ns string, workloads, nodes, admission, enableCnspec, consoleIntegration bool) mondoov2.MondooAuditConfig {
	auditConfig := mondoov2.MondooAuditConfig{
		ObjectMeta: v1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: ns,
		},
		Spec: mondoov2.MondooAuditConfigSpec{
			ConsoleIntegration:   mondoov2.ConsoleIntegration{Enable: consoleIntegration},
			MondooCredsSecretRef: corev1.LocalObjectReference{Name: MondooClientSecret},
			MondooTokenSecretRef: corev1.LocalObjectReference{Name: MondooTokenSecret},
			KubernetesResources:  mondoov2.KubernetesResources{Enable: workloads},
			Nodes:                mondoov2.Nodes{Enable: nodes},
			Admission:            mondoov2.Admission{Enable: admission},
		},
	}

	// cnspec doesn't get edge releases at the moment, so we cannot test that
	if mondooClientImageTag != "" {
		tag := mondooClientImageTag
		auditConfig.Spec.Scanner.Image.Tag = tag
		if enableCnspec {
			// Right now the tags are coming from the Mondoo repo. We need to trigger the edge tests from the cnspec repo to fix that
			tag = "edge-latest-rootless"
		}
		zap.S().Infof("Using image %s:%s for mondoo-client", mondoo.MondooClientImage, tag)
	}

	return auditConfig
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
				Image:    mondoov2.Image{Name: "test", Tag: "latest"},
				Replicas: pointer.Int32(1),
			},
		},
	}
}
