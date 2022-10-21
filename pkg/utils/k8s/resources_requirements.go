/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package k8s

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// DefaultMondooClientResources for Mondoo Client container
var DefaultMondooClientResources corev1.ResourceRequirements = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("400M"),
		corev1.ResourceCPU:    resource.MustParse("1"),
	},

	Requests: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("180M"),
		corev1.ResourceCPU:    resource.MustParse("400m"),
	},
}

// ResourcesRequirementsWithDefaults will return the resource requirements from the parameter if such
// are specified. If not requirements are specified, default values will be returned.
func ResourcesRequirementsWithDefaults(m corev1.ResourceRequirements) corev1.ResourceRequirements {
	if m.Size() != 0 {
		return m
	}

	// Default values for Mondoo resources requirements.
	return DefaultMondooClientResources
}

// DefaultNodeScanningResources for Mondoo Client container when scanning nodes
var DefaultNodeScanningResources corev1.ResourceRequirements = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("500M"),
		corev1.ResourceCPU:    resource.MustParse("200m"),
	},

	Requests: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("60M"),
		corev1.ResourceCPU:    resource.MustParse("50m"),
	},
}

// NodeScanningResourcesRequirementsWithDefaults will return the resource requirements from the parameter if such
// are specified. If not requirements are specified, default values will be returned.
func NodeScanningResourcesRequirementsWithDefaults(m corev1.ResourceRequirements) corev1.ResourceRequirements {
	if m.Size() != 0 {
		return m
	}

	// Default values for Mondoo resources requirements.
	return DefaultNodeScanningResources
}
