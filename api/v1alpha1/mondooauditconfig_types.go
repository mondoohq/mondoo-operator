/*
Copyright 2022 Mondoo, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"fmt"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MondooAuditConfigSpec defines the desired state of MondooAuditConfig
type MondooAuditConfigData struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Config is an example field of MondooAuditConfig. Edit mondooauditconfig_types.go to remove/update
	Nodes           Nodes     `json:"nodes,omitempty"`
	Workloads       Workloads `json:"workloads,omitempty"`
	Webhooks        Webhooks  `json:"webhooks,omitempty"`
	MondooSecretRef string    `json:"mondooSecretRef"`
}
type Nodes struct {
	Enable    bool                        `json:"enable,omitempty"`
	Inventory string                      `json:"inventory,omitempty"`
	Image     Image                       `json:"image,omitempty"`
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

type Workloads struct {
	Enable         bool                        `json:"enable,omitempty"`
	Inventory      string                      `json:"inventory,omitempty"`
	Resources      corev1.ResourceRequirements `json:"resources,omitempty"`
	ServiceAccount string                      `json:"serviceAccount,omitempty"`
	Image          Image                       `json:"image,omitempty"`
}

type Image struct {
	Name string `json:"name,omitempty"`
	Tag  string `json:"tag,omitempty"`
}

// InjectionStyle is the specified method the cluster uses for automated creation of TLS certificates
type InjectionStyle string

const (
	CertManager InjectionStyle = "cert-manager"
	OpenShift   InjectionStyle = "openshift"
)

type WebhookCertificateConfig struct {
	// +kubebuilder:validation:Enum="";cert-manager;openshift
	InjectionStyle string `json:"injectionStyle,omitempty"`
}

// WebhookMode specifies the allowed modes of operation for the webhook admission controller
type WebhookMode string

const (
	Permissive WebhookMode = "permissive"
	Enforcing  WebhookMode = "enforcing"
)

type Webhooks struct {
	Enable bool `json:"enable,omitempty"`

	// CertificateConfig allows defining which certificate system to use.
	// Leaving it as the empty string will mean the user will be responsible
	// for creating the Secret with the TLS data, and inserting the CA data
	// into the ValidatingWebhookConfigurations as well.
	CertificateConfig WebhookCertificateConfig `json:"certificateConfig,omitempty"`
	Image             Image                    `json:"image,omitempty"`
	// Mode represents whether the webhook will behave in a "permissive" mode (the default) which
	// will only scan and report on k8s resources or "enforcing" mode where depending
	// on the scan results may reject the k8s resource creation/modification.
	// +kubebuilder:validation:Enum="";permissive;enforcing
	Mode string `json:"mode,omitempty"`
}

// MondooAuditConfigStatus defines the observed state of MondooAuditConfig
type MondooAuditConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Pods store the name of the pods which are running mondoo instances
	Pods []string `json:"pods,omitempty"`

	// Conditions includes detailed status for the MondooAuditConfig
	Conditions []MondooAuditConfigCondition `json:"conditions,omitempty"`
}

type MondooAuditConfigCondition struct {
	// Type is the specific type of the condition
	// +kubebuilder:validation:Required
	// +required
	Type MondooAuditConfigConditionType `json:"type"`
	// Status is the status of the condition
	// +kubebuilder:validation:Required
	// +required
	Status corev1.ConditionStatus `json:"status"`
	// LastAuditTime is the last time we probed the condition
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// LastTransitionTime is the last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason is a unique, one-word, CamelCase reason for the condition's last transition
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable message indicating details about the last transition
	Message string `json:"message,omitempty"`
}

// MondooOperatorConfigConditionType is a valid value for MondooOperatorConfig.Status.Condition[].Type
type MondooAuditConfigConditionType string

const (
	// Indicates weather NodeScanning is Degraded
	NodeScanningDegraded MondooAuditConfigConditionType = "NodeScanningDegraded"
	// Indicates weather APIScanning is Degraded
	APIScanningDegraded MondooAuditConfigConditionType = "APIScanningDegraded"
	// Indicates weather Webhook is Degraded
	WebhookDegraded MondooAuditConfigConditionType = "WebhookDegraded"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:deprecatedversion
//+kubebuilder:deprecatedversion:warning="k8s.mondoo.com/v1alpha1 is deprecated. The CRD has to be manually converted to v1alpha2"

// MondooAuditConfig is the Schema for the mondooauditconfigs API
type MondooAuditConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MondooAuditConfigData   `json:"spec,omitempty"`
	Status MondooAuditConfigStatus `json:"status,omitempty"`
}

// ConvertTo converts this MondooAuditConfig to the Hub version (v1alpha2).
func (src *MondooAuditConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.MondooAuditConfig)

	dst.ObjectMeta = *src.ObjectMeta.DeepCopy()

	dst.Spec.MondooCredsSecretRef.Name = src.Spec.MondooSecretRef

	dst.Spec.Admission.CertificateProvisioning.Mode =
		v1alpha2.CertificateProvisioningMode(src.Spec.Webhooks.CertificateConfig.InjectionStyle)
	if dst.Spec.Admission.CertificateProvisioning.Mode == "" {
		dst.Spec.Admission.CertificateProvisioning.Mode = v1alpha2.ManualProvisioning
	}

	dst.Spec.Scanner.ServiceAccountName = src.Spec.Workloads.ServiceAccount

	// Try to set the image to the image for nodes. If such is not specified attempt to take the
	// image from workloads.
	if src.Spec.Nodes.Image.Name != "" || src.Spec.Nodes.Image.Tag != "" {
		dst.Spec.Scanner.Image.Name = src.Spec.Nodes.Image.Name
		dst.Spec.Scanner.Image.Tag = src.Spec.Nodes.Image.Tag
	} else if src.Spec.Workloads.Image.Name != "" || src.Spec.Workloads.Image.Tag != "" {
		dst.Spec.Scanner.Image.Name = src.Spec.Workloads.Image.Name
		dst.Spec.Scanner.Image.Tag = src.Spec.Workloads.Image.Tag
	}

	// Try to set the requirements to the requirements for nodes. If such are not specified attempt
	// to take the requirements from workloads.
	if src.Spec.Nodes.Resources.Size() != 0 {
		dst.Spec.Scanner.Resources = src.Spec.Nodes.Resources
	} else if src.Spec.Workloads.Resources.Size() != 0 {
		dst.Spec.Scanner.Resources = src.Spec.Workloads.Resources
	}

	dst.Spec.KubernetesResources.Enable = src.Spec.Workloads.Enable

	dst.Spec.Nodes.Enable = src.Spec.Nodes.Enable

	dst.Spec.Admission.Enable = src.Spec.Webhooks.Enable
	dst.Spec.Admission.Mode = v1alpha2.AdmissionMode(src.Spec.Webhooks.Mode)
	dst.Spec.Admission.Image.Name = src.Spec.Webhooks.Image.Name
	dst.Spec.Admission.Image.Tag = src.Spec.Webhooks.Image.Tag

	dst.Status.Pods = src.Status.Pods
	for _, c := range src.Status.Conditions {
		var cType v1alpha2.MondooAuditConfigConditionType
		switch c.Type {
		case NodeScanningDegraded:
			cType = v1alpha2.NodeScanningDegraded
		case APIScanningDegraded:
			cType = v1alpha2.K8sResourcesScanningDegraded
		case WebhookDegraded:
			cType = v1alpha2.AdmissionDegraded
		default:
			return fmt.Errorf("Unknown condition type %s", c.Type)
		}

		alpha2C := v1alpha2.MondooAuditConfigCondition{
			Type:               cType,
			Status:             c.Status,
			LastUpdateTime:     c.LastUpdateTime,
			LastTransitionTime: c.LastTransitionTime,
			Reason:             c.Reason,
			Message:            c.Message,
		}
		dst.Status.Conditions = append(dst.Status.Conditions, alpha2C)
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1alpha2) to this version.
func (dst *MondooAuditConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.MondooAuditConfig)

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.MondooSecretRef = src.Spec.MondooCredsSecretRef.Name

	dst.Spec.Webhooks.Enable = src.Spec.Admission.Enable
	dst.Spec.Webhooks.CertificateConfig.InjectionStyle = string(src.Spec.Admission.CertificateProvisioning.Mode)
	if dst.Spec.Webhooks.CertificateConfig.InjectionStyle == string(v1alpha2.ManualProvisioning) {
		// The equivalent of manual provisioning in the old version is an empty string
		dst.Spec.Webhooks.CertificateConfig.InjectionStyle = ""
	}
	dst.Spec.Webhooks.Mode = string(src.Spec.Admission.Mode)
	dst.Spec.Webhooks.Image.Name = src.Spec.Admission.Image.Name
	dst.Spec.Webhooks.Image.Tag = src.Spec.Admission.Image.Tag

	dst.Spec.Workloads.Enable = src.Spec.KubernetesResources.Enable
	dst.Spec.Workloads.ServiceAccount = src.Spec.Scanner.ServiceAccountName
	dst.Spec.Workloads.Resources = src.Spec.Scanner.Resources
	dst.Spec.Workloads.Image.Name = src.Spec.Scanner.Image.Name
	dst.Spec.Workloads.Image.Tag = src.Spec.Scanner.Image.Tag

	dst.Spec.Nodes.Enable = src.Spec.Nodes.Enable
	dst.Spec.Nodes.Resources = src.Spec.Scanner.Resources
	dst.Spec.Nodes.Image.Name = src.Spec.Scanner.Image.Name
	dst.Spec.Nodes.Image.Tag = src.Spec.Scanner.Image.Tag

	dst.Status.Pods = src.Status.Pods
	for _, c := range src.Status.Conditions {
		var cType MondooAuditConfigConditionType
		switch c.Type {
		case v1alpha2.NodeScanningDegraded:
			cType = NodeScanningDegraded
		case v1alpha2.K8sResourcesScanningDegraded:
			cType = APIScanningDegraded
		case v1alpha2.AdmissionDegraded:
			cType = WebhookDegraded
		default:
			return fmt.Errorf("Unknown condition type %s", c.Type)
		}

		alpha2C := MondooAuditConfigCondition{
			Type:               cType,
			Status:             c.Status,
			LastUpdateTime:     c.LastUpdateTime,
			LastTransitionTime: c.LastTransitionTime,
			Reason:             c.Reason,
			Message:            c.Message,
		}
		dst.Status.Conditions = append(dst.Status.Conditions, alpha2C)
	}

	return nil
}

//+kubebuilder:object:root=true

// MondooAuditConfigList contains a list of MondooAuditConfig
type MondooAuditConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MondooAuditConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MondooAuditConfig{}, &MondooAuditConfigList{})
}
