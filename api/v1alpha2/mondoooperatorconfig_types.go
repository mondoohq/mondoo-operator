// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// MondooOperatorConfigName is the one allowed name for the
	// cluster-scoped MondooOperatorConfig resource
	MondooOperatorConfigName = "mondoo-operator-config"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MondooOperatorConfigSpec defines the desired state of MondooOperatorConfig
type MondooOperatorConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Metrics controls the enabling/disabling of metrics report of mondoo-operator
	Metrics Metrics `json:"metrics,omitempty"`
	// Allows skipping Image resolution from upstream repository
	SkipContainerResolution bool `json:"skipContainerResolution,omitempty"`
	// HttpProxy specifies a proxy to use for HTTP requests to the Mondoo Platform.
	HttpProxy *string `json:"httpProxy,omitempty"`
	// ContainerProxy specifies a proxy to use for container images.
	ContainerProxy *string `json:"containerProxy,omitempty"`
}

type Metrics struct {
	Enable bool `json:"enable,omitempty"`
	// ResourceLabels allows providing a list of extra labels to apply to the metrics-related
	// resources (eg. ServiceMonitor)
	ResourceLabels map[string]string `json:"resourceLabels,omitempty"`
}

// MondooOperatorConfigStatus defines the observed state of MondooOperatorConfig
type MondooOperatorConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions includes more detailed status for the mondoo config
	// +optional
	Conditions []MondooOperatorConfigCondition `json:"conditions,omitempty"`
}

// Condition contains details for the current condition of a MondooOperatorConfig
type MondooOperatorConfigCondition struct {
	// Type is the type of the condition.
	Type MondooOperatorConfigConditionType `json:"type"`
	// Status is the status of the condition.
	Status corev1.ConditionStatus `json:"status"`
	// LastUpdateTime is the last time the condition was updated.
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason is a unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// MondooOperatorConfigConditionType is a valid value for MondooOperatorConfig.Status.Condition[].Type
type MondooOperatorConfigConditionType string

const (
	// PrometheusMissingCondition is used to indicate whether Prometheus was found to be installed or not.
	PrometheusMissingCondition MondooOperatorConfigConditionType = "PrometheusMissing"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// MondooOperatorConfig is the Schema for the mondoooperatorconfigs API
type MondooOperatorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MondooOperatorConfigSpec   `json:"spec,omitempty"`
	Status MondooOperatorConfigStatus `json:"status,omitempty"`
}

// Hub marks this type as a conversion hub.
func (*MondooOperatorConfig) Hub() {}

//+kubebuilder:object:root=true

// MondooOperatorConfigList contains a list of MondooOperatorConfig
type MondooOperatorConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MondooOperatorConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MondooOperatorConfig{}, &MondooOperatorConfigList{})
}
