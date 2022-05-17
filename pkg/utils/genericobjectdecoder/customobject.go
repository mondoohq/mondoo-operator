package customobject

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CustomObjectMeta allows us to decode the raw k8s Object to unmarshal
// the Type and Object fields we use to generate labels from.
type CustomObjectMeta struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}
