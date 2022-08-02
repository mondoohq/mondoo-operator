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

import corev1 "k8s.io/api/core/v1"

// MergeEnv merges 2 slices of env vars. If the same key is present in
// both slices, the value from the second slice will be used.
func MergeEnv(a, b []corev1.EnvVar) []corev1.EnvVar {
	envSet := make(map[string]string)
	for _, e := range a {
		envSet[e.Name] = e.Value
	}
	for _, e := range b {
		envSet[e.Name] = e.Value
	}

	mergedEnv := make([]corev1.EnvVar, 0, len(envSet))
	for k, v := range envSet {
		mergedEnv = append(mergedEnv, corev1.EnvVar{Name: k, Value: v})
	}
	return mergedEnv
}
