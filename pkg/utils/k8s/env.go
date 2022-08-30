/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package k8s

import corev1 "k8s.io/api/core/v1"

// MergeEnv merges 2 slices of env vars. If the same key is present in
// both slices, the value from the second slice will be used.
func MergeEnv(a, b []corev1.EnvVar) []corev1.EnvVar {
	envSet := make(map[string]corev1.EnvVar)
	for _, e := range a {
		envSet[e.Name] = e
	}
	for _, e := range b {
		envSet[e.Name] = e
	}

	mergedEnv := make([]corev1.EnvVar, 0, len(envSet))
	for _, v := range envSet {
		mergedEnv = append(mergedEnv, v)
	}
	return mergedEnv
}
