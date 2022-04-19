package k8s

import (
	"reflect"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// AreDeploymentsEqual returns a value indicating whether 2 deployments are equal. Note that it does not perform a full
// comparison but checks just some of the properties of a deployment (only the ones we are currently interested at).
func AreDeploymentsEqual(a, b appsv1.Deployment) bool {
	c := len(a.Spec.Template.Spec.Containers) == len(b.Spec.Template.Spec.Containers)
	d := reflect.DeepEqual(a.Spec.Replicas, b.Spec.Replicas)
	e := reflect.DeepEqual(a.Spec.Selector, b.Spec.Selector)
	f := reflect.DeepEqual(a.Spec.Template.Spec.Containers[0].Image, b.Spec.Template.Spec.Containers[0].Image)
	g := reflect.DeepEqual(a.Spec.Template.Spec.Containers[0].Command, b.Spec.Template.Spec.Containers[0].Command)
	h := reflect.DeepEqual(a.Spec.Template.Spec.Containers[0].VolumeMounts, b.Spec.Template.Spec.Containers[0].VolumeMounts)
	i := reflect.DeepEqual(a.Spec.Template.Spec.Containers[0].Env, b.Spec.Template.Spec.Containers[0].Env)
	zap.S().Info(c, d, e, f, g, h, i)
	return len(a.Spec.Template.Spec.Containers) == len(b.Spec.Template.Spec.Containers) &&
		reflect.DeepEqual(a.Spec.Replicas, b.Spec.Replicas) &&
		reflect.DeepEqual(a.Spec.Selector, b.Spec.Selector) &&
		reflect.DeepEqual(a.Spec.Template.Spec.Containers[0].Image, b.Spec.Template.Spec.Containers[0].Image) &&
		reflect.DeepEqual(a.Spec.Template.Spec.Containers[0].Command, b.Spec.Template.Spec.Containers[0].Command) &&
		reflect.DeepEqual(a.Spec.Template.Spec.Containers[0].VolumeMounts, b.Spec.Template.Spec.Containers[0].VolumeMounts) &&
		reflect.DeepEqual(a.Spec.Template.Spec.Containers[0].Env, b.Spec.Template.Spec.Containers[0].Env)
}

// AreServicesEqual return a value indicating whether 2 services are equal. Note that it
// does not perform a full comparison but checks just some of the properties of a deployment
// (only the ones we are currently interested at).
func AreServicesEqual(a, b corev1.Service) bool {
	return reflect.DeepEqual(a.Spec.Ports, b.Spec.Ports) &&
		reflect.DeepEqual(a.Spec.Selector, b.Spec.Selector) &&
		a.Spec.Type == b.Spec.Type
}

// AreResouceRequirementsEqual returns a value indicating whether 2 resource requirements are equal.
func AreResouceRequirementsEqual(x corev1.ResourceRequirements, y corev1.ResourceRequirements) bool {
	if x.Limits.Cpu().Equal(*y.Limits.Cpu()) &&
		x.Limits.Memory().Equal(*y.Limits.Memory()) &&
		x.Requests.Cpu().Equal(*y.Requests.Cpu()) &&
		x.Requests.Memory().Equal(*y.Requests.Memory()) {
		return true
	}
	return false
}
