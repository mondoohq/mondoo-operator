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

func TaintsToTolerations(taints []corev1.Taint) []corev1.Toleration {
	var tolerations []corev1.Toleration
	for _, t := range taints {
		tolerations = append(tolerations, TaintToToleration(t))
	}
	return tolerations
}

func TaintToToleration(t corev1.Taint) corev1.Toleration {
	return corev1.Toleration{
		Key:    t.Key,
		Effect: t.Effect,
		Value:  t.Value,
	}
}
