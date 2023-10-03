// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// DefaultCnspecResources for cnspec container
var DefaultCnspecResources corev1.ResourceRequirements = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("400M"),
		corev1.ResourceCPU:    resource.MustParse("600m"),
	},

	Requests: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("150M"),
		corev1.ResourceCPU:    resource.MustParse("300m"),
	},
}

// DefaultContainerScanningResources for cnspec container
var DefaultContainerScanningResources corev1.ResourceRequirements = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("300M"),
		corev1.ResourceCPU:    resource.MustParse("1"),
	},

	Requests: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("150M"),
		corev1.ResourceCPU:    resource.MustParse("400m"),
	},
}

// DefaultNodeScanningResources for cnspec container when scanning nodes
var DefaultNodeScanningResources corev1.ResourceRequirements = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("160M"),
		corev1.ResourceCPU:    resource.MustParse("200m"),
	},

	Requests: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("100M"),
		corev1.ResourceCPU:    resource.MustParse("50m"),
	},
}

// ResourcesRequirementsWithDefaults will return the resource requirements from the parameter if such
// are specified. If not requirements are specified, default values will be returned.
func ResourcesRequirementsWithDefaults(m corev1.ResourceRequirements, defaults corev1.ResourceRequirements) corev1.ResourceRequirements {
	if m.Size() != 0 {
		return m
	}
	return defaults
}
