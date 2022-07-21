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
		corev1.ResourceMemory: resource.MustParse("100M"),
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
