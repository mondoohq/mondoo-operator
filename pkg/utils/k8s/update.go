package k8s

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// UpdateService updates a service such that it matches a desired state. The function does not
// replace all fields but only a set of fields that we are interested at.
func UpdateService(current *corev1.Service, desired corev1.Service) {
	current.Spec.Ports = desired.Spec.Ports
	current.Spec.Selector = desired.Spec.Selector
	current.Spec.Type = desired.Spec.Type
	current.SetOwnerReferences(desired.GetOwnerReferences())
}

// UpdateDeployment updates a deployment such that it matches a desired state. The function does
// not replace all fields but only a set of fields that we are interested at.
func UpdateDeployment(current *appsv1.Deployment, desired appsv1.Deployment) {
	current.Spec = desired.Spec
	current.SetOwnerReferences(desired.GetOwnerReferences())
}
