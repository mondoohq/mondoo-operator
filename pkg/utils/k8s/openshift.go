/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package k8s

var openshiftAPIs = [][]string{{"config.openshift.io", "v1"}, {"nodes.config.openshift.io", "v1"}}

// IsOpenshift returns a value indicating whether the current cluster is an OpenShift cluster.
func IsOpenshift() (bool, error) {
	for _, a := range openshiftAPIs {
		exists, err := VerifyAPI(a[0], a[1])
		if err != nil {
			return false, err
		}

		// If the API exists, then this is an OpenShift cluster
		if exists {
			return true, nil
		}
	}
	return false, nil
}
