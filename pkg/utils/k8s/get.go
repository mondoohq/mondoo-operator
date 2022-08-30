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

// CheckIfExists will attempt to Get() the object, and report whether or not the object was found to exist.
func CheckIfExists(ctx context.Context, kubeClient client.Client, retrieveObj, checkObj client.Object) (bool, error) {
	if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(checkObj), retrieveObj); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
