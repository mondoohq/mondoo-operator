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
		corev1.ResourceMemory: resource.MustParse("450M"),
		corev1.ResourceCPU:    resource.MustParse("1"),
	},

	Requests: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("250M"),
		corev1.ResourceCPU:    resource.MustParse("300m"),
	},
}

// DefaultContainerScanningResources for cnspec container
// Container scanning downloads images to /tmp, requiring significant ephemeral storage
// GKE Autopilot defaults to 1Gi but allows up to 10Gi per container
var DefaultContainerScanningResources corev1.ResourceRequirements = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceMemory:           resource.MustParse("500M"),
		corev1.ResourceCPU:              resource.MustParse("1"),
		corev1.ResourceEphemeralStorage: resource.MustParse("5Gi"),
	},

	Requests: corev1.ResourceList{
		corev1.ResourceMemory:           resource.MustParse("250M"),
		corev1.ResourceCPU:              resource.MustParse("400m"),
		corev1.ResourceEphemeralStorage: resource.MustParse("2Gi"),
	},
}

// DefaultNodeScanningResources for cnspec container when scanning nodes
var DefaultNodeScanningResources corev1.ResourceRequirements = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("250M"),
		corev1.ResourceCPU:    resource.MustParse("300m"),
	},

	Requests: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("150M"),
		corev1.ResourceCPU:    resource.MustParse("100m"),
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
