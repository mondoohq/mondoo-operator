// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package gomemlimit

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
)

func CalculateGoMemLimit(containerResources v1.ResourceRequirements) string {
	// https://cs.opensource.google/go/go/+/master:src/runtime/mgcpacer.go;l=96?q=GOMEMLIMIT&ss=go%2Fgo
	// Initialized from GOMEMLIMIT. GOMEMLIMIT=off is equivalent to MaxInt64
	// which means no soft memory limit in practice.
	gcLimit := "off"
	memoryLimit := containerResources.Limits.Memory()

	if memoryLimit != nil {
		// https://go.dev/doc/gc-guide#Suggested_uses
		// deployment ... into containers with a fixed amount of available memory.
		// In this case, a good rule of thumb is to leave an additional 5-10% of headroom to account for memory sources the Go runtime is unaware of.
		gcLimit = fmt.Sprintf("%.0f", (float64(memoryLimit.Value()) * 0.9))
		if gcLimit == "0" {
			gcLimit = "off"
		}
	}

	return gcLimit
}
