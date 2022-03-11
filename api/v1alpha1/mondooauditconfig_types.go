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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	Enable    bool   `json:"enable,omitempty"`
	Inventory string `json:"inventory,omitempty"`
	Image     Image  `json:"image,omitempty"`
}

type Workloads struct {
	Enable    bool   `json:"enable,omitempty"`
	Inventory string `json:"inventory,omitempty"`
	// Replicas               int32  `json:"replicas,omitempty"`
	ServiceAccount string `json:"serviceAccount,omitempty"`
	Image          Image  `json:"image,omitempty"`
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

type Webhooks struct {
	Enable bool `json:"enable,omitemmpty"`

	// CertificateConfig allows defining which certificate system to use.
	// Leaving it as the empty string will mean the user will be responsible
	// for creating the Secret with the TLS data, and inserting the CA data
	// into the ValidatingWebhookConfigurations as well.
	CertificateConfig WebhookCertificateConfig `json:"certificateConfig,omitempty"`
}

// MondooAuditConfigStatus defines the observed state of MondooAuditConfig
type MondooAuditConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Nodes store the name of the pods which are running mondoo instances
	Pods []string `json:"pods,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MondooAuditConfig is the Schema for the mondooauditconfigs API
type MondooAuditConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MondooAuditConfigData   `json:"spec,omitempty"`
	Status MondooAuditConfigStatus `json:"status,omitempty"`
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
