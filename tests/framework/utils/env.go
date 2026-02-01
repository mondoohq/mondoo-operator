// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package utils

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
)

func SortEnvVars(envVars []corev1.EnvVar) {
	sort.Slice(envVars, func(i, j int) bool {
		return envVars[i].Name < envVars[j].Name
	})
}
