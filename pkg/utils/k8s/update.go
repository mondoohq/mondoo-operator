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
