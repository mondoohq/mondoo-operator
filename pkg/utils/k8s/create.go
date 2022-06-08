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

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateIfNotExist creates an object if it doesn't already exist. The returned boolean indicates whether the
// object has been created. If the object already existed or an error occurred, then "false" is returned. If the
// object exists, no AlreadyExists error is returned.
func CreateIfNotExist(ctx context.Context, kubeClient client.Client, retrieveObj, createObj client.Object) (bool, error) {
	if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(createObj), retrieveObj); err != nil {
		if errors.IsNotFound(err) {
			if err := kubeClient.Create(ctx, createObj); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, err
	}
	return false, nil
}
