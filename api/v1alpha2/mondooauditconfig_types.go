/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
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
	Filtering           Filtering           `json:"filtering,omitempty"`
	Containers          Containers          `json:"containers,omitempty"`

	// HttpProxy specifies a proxy to use for HTTP requests to the Mondoo platform.
	HttpProxy *string `json:"httpProxy,omitempty"`
}

type Filtering struct {
	Namespaces FilteringSpec `json:"namespaces,omitempty"`
}

type FilteringSpec struct {
	// Include is the list of resources to watch/scan. Setting Include overrides anything in the
	// Exclude list as specifying an Include list is effectively excluding everything except for what
	// is on the Include list.
	Include []string `json:"include,omitempty"`

	// Exclude is the list of resources to ignore for any watching/scanning actions. Use this if
	// the goal is to watch/scan all resources except for this Exclude list.
	Exclude []string `json:"exclude,omitempty"`
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
	// Number of replicas for the scanner.
	// For enforcing mode, the minimum should be two to prevent problems during Pod failures,
	// e.g. node failure, node scaling, etc.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`

	// PrivateRegistryScanning defines the name of a secret that contains the credentials for the private
	// registries we have to pull images from.
	PrivateRegistriesPullSecretRef corev1.LocalObjectReference `json:"privateRegistriesPullSecretRef,omitempty"`

	// Env allows setting extra environment variables for the scanner. If the operator sets already an env
	// variable with the same name, the value specified here will override it.
	Env []corev1.EnvVar `json:"env,omitempty"`
}

type KubernetesResources struct {
	Enable bool `json:"enable,omitempty"`

	// DEPRECATED: ContainerImageScanning determines whether container images are being scanned. The current implementation
	// runs a separate job once every 24h that scans the container images running in the cluster.
	ContainerImageScanning bool `json:"containerImageScanning,omitempty"`
}

type Nodes struct {
	Enable    bool                        `json:"enable,omitempty"`
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
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
	// Number of replicas for the admission webhook.
	// For enforcing mode, the minimum should be two to prevent problems during Pod failures,
	// e.g. node failure, node scaling, etc.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Replicas                *int32                  `json:"replicas,omitempty"`
	CertificateProvisioning CertificateProvisioning `json:"certificateProvisioning,omitempty"`
	// ServiceAccountName specifies the Kubernetes ServiceAccount the webhook should use
	// during its operation.
	// +kubebuilder:default=mondoo-operator-webhook
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

type Containers struct {
	Enable    bool                        `json:"enable,omitempty"`
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
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

	// ReconciledByOperatorVersion contains the version of the operator which reconciled this MondooAuditConfig
	ReconciledByOperatorVersion string `json:"reconciledByOperatorVersion,omitempty"`
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
	// Indicates weather Kubernetes container image scanning is Degraded
	K8sContainerImageScanningDegraded MondooAuditConfigConditionType = "K8sContainerImageScanningDegraded"
	// Indicates weather Admission controller is Degraded
	AdmissionDegraded MondooAuditConfigConditionType = "AdmissionDegraded"
	// Indicates weather Admission controller is Degraded because of the ScanAPI
	ScanAPIDegraded MondooAuditConfigConditionType = "ScanAPIDegraded"
	// MondooIntegrationDegraded will hold the status for any issues encountered while trying to CheckIn()
	// on behalf of the Mondoo integration MRN
	MondooIntegrationDegraded MondooAuditConfigConditionType = "IntegrationDegraded"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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
