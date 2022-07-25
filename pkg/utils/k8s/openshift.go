package k8s

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// IsOpenShift will check whether we are running on
// an OpenShift-style cluster
func IsOpenShift(cfg *rest.Config) (bool, error) {
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return false, err
	}

	crdClient := dynClient.Resource(schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	})

	// The clusterversions CRD shouldn't exist outside of OpenShift
	_, err = crdClient.Get(context.Background(), "clusterversions.config.openshift.io", metav1.GetOptions{})
	if err == nil {
		return true, nil
	} else if errors.IsNotFound(err) {
		return false, nil
	} else {
		return false, err
	}
}
