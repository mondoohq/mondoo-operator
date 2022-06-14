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
