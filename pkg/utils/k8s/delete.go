package k8s

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeleteIfExists deletes a Kubernetes object if it exists. Any errors that might pop up because the object
// does not exist are ignored.
func DeleteIfExists(ctx context.Context, kubeClient client.Client, obj client.Object) error {
	// If the Delete return a NotFound or an error containing "no matches for kind", it means the object
	// does not exists. In any other case we return an error.
	if err := kubeClient.Delete(ctx, obj); err != nil &&
		!errors.IsNotFound(err) &&
		!strings.Contains(err.Error(), "no matches for kind") {
		return err
	}
	return nil
}
