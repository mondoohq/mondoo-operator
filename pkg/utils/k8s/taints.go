/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
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
