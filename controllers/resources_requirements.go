package controllers

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// defaultMondooClientResources for Mondoo Client container
var defaultMondooClientResources corev1.ResourceRequirements = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("1G"),
		corev1.ResourceCPU:    resource.MustParse("500m"),
	},

	Requests: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("500M"), // 50% of the limit
		corev1.ResourceCPU:    resource.MustParse("50m"),  // 10% of the limit
	},
}

// equalResouceRequirements compares two different resource requiremetns to be eqal
func equalResouceRequirements(x corev1.ResourceRequirements, y corev1.ResourceRequirements) bool {
	if x.Limits.Cpu().Equal(*y.Limits.Cpu()) &&
		x.Limits.Memory().Equal(*y.Limits.Memory()) &&
		x.Requests.Cpu().Equal(*y.Requests.Cpu()) &&
		x.Requests.Memory().Equal(*y.Requests.Memory()) {
		return true
	}
	return false
}

// getNodeResources will return the ResourceRequirements for the Mondoo container.
func getResourcesRequirements(m corev1.ResourceRequirements) corev1.ResourceRequirements {
	// Allow override of resource requirements from Mondoo Object
	if m.Size() != 0 {
		return m
	}

	// Default values for Mondoo resources requirements.
	return defaultMondooClientResources
}
