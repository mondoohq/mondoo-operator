/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
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
