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

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MondooAuditConfigSpec defines the desired state of MondooAuditConfig
type MondooAuditConfigSpec struct {
	// Important: Run "make" to regenerate code after modifying this file

	// Config is an example field of MondooAuditConfig. Edit mondooauditconfig_types.go to remove/update
	// +kubebuilder:validation:Required
	// +required
	MondooCredsSecretRef corev1.LocalObjectReference `json:"mondooCredsSecretRef"`

	// MondooTokenSecretRef can optionally hold a time-limited token that the mondoo-operator will use
	// to create a Mondoo service account saved to the Secret specified in .spec.mondooCredsSecretRef
	// if that Secret does not exist.
	MondooTokenSecretRef corev1.LocalObjectReference ` json:"mondooTokenSecretRef,omitempty"`

	Scanner             Scanner             `json:"scanner,omitempty"`
	KubernetesResources KubernetesResources `json:"kubernetesResources,omitempty"`
	Nodes               Nodes               `json:"nodes,omitempty"`
	Admission           Admission           `json:"admission,omitempty"`
	ConsoleIntegration  ConsoleIntegration  `json:"consoleIntegration,omitempty"`
}

type ConsoleIntegration struct {
	Enable bool `json:"enable,omitempty"`
}

// CertificateProvisioning defines the certificate provisioning configuration within the cluster.
type CertificateProvisioning struct {
	// +kubebuilder:validation:Enum=cert-manager;openshift;manual
	// +kubebuilder:default=manual
	Mode CertificateProvisioningMode `json:"mode,omitempty"`
}

// Scanner defines the settings for the Mondoo scanner that will be running in the cluster. The same scanner
// is used for scanning the Kubernetes API, the nodes and for serving the admission controller.
type Scanner struct {
	// +kubebuilder:default=mondoo-operator-k8s-resources-scanning
	ServiceAccountName string                      `json:"serviceAccountName,omitempty"`
	Image              Image                       `json:"image,omitempty"`
	Resources          corev1.ResourceRequirements `json:"resources,omitempty"`
}

type KubernetesResources struct {
	Enable bool `json:"enable,omitempty"`
}

type Nodes struct {
	Enable bool `json:"enable,omitempty"`
}

type Admission struct {
	Enable bool  `json:"enable,omitempty"`
	Image  Image `json:"image,omitempty"`
	// Mode represents whether the webhook will behave in a "permissive" mode (the default) which
	// will only scan and report on k8s resources or "enforcing" mode where depending
	// on the scan results may reject the k8s resource creation/modification.
	// +kubebuilder:validation:Enum=permissive;enforcing
	// +kubebuilder:default=permissive
	Mode AdmissionMode `json:"mode,omitempty"`

	CertificateProvisioning CertificateProvisioning `json:"certificateProvisioning,omitempty"`
}

type Image struct {
	Name string `json:"name,omitempty"`
	Tag  string `json:"tag,omitempty"`
}

// CertificateProvisioningMode is the specified method the cluster uses for provisioning TLS certificates
type CertificateProvisioningMode string

const (
	CertManagerProvisioning CertificateProvisioningMode = "cert-manager"
	OpenShiftProvisioning   CertificateProvisioningMode = "openshift"
	ManualProvisioning      CertificateProvisioningMode = "manual"
)

// AdmissionMode specifies the allowed modes of operation for the webhook admission controller
type AdmissionMode string

const (
	Permissive AdmissionMode = "permissive"
	Enforcing  AdmissionMode = "enforcing"
)

// MondooAuditConfigStatus defines the observed state of MondooAuditConfig
type MondooAuditConfigStatus struct {
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
	// LastUpdateTime is the last time we probed the condition
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
	// Indicates weather Kubernetes resources scanning is Degraded
	K8sResourcesScanningDegraded MondooAuditConfigConditionType = "K8sResourcesScanningDegraded"
	// Indicates weather Admission controller is Degraded
	AdmissionDegraded MondooAuditConfigConditionType = "AdmissionDegraded"
	// MondooIntegrationDegraded will hold the status for any issues encountered while trying to CheckIn()
	// on behalf of the Mondoo integration MRN
	MondooIntegrationDegraded MondooAuditConfigConditionType = "IntegrationDegraded"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

// MondooAuditConfig is the Schema for the mondooauditconfigs API
type MondooAuditConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MondooAuditConfigSpec   `json:"spec,omitempty"`
	Status MondooAuditConfigStatus `json:"status,omitempty"`
}

// Hub marks this type as a conversion hub.
func (*MondooAuditConfig) Hub() {}

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
