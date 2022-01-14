/*
Copyright 2022.

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

// MondooClientSpec defines the desired state of MondooClient
type MondooClientData struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Config is an example field of MondooClient. Edit mondooclient_types.go to remove/update
	Config string `json:"config,omitempty"`
}

// MondooClientStatus defines the observed state of MondooClient
type MondooClientStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Nodes store the name of the pods which are running mondoo instances
	Nodes []string `json:"nodes,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MondooClient is the Schema for the mondooclients API
type MondooClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Data   MondooClientData   `json:"data,omitempty"`
	Status MondooClientStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MondooClientList contains a list of MondooClient
type MondooClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MondooClient `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MondooClient{}, &MondooClientList{})
}
