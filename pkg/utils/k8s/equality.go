// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"reflect"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// AreDeploymentsEqual returns a value indicating whether 2 deployments are equal. Note that it does not perform a full
// comparison but checks just some of the properties of a deployment (only the ones we are currently interested at).
func AreDeploymentsEqual(a, b appsv1.Deployment) bool {
	return len(a.Spec.Template.Spec.Containers) == len(b.Spec.Template.Spec.Containers) &&
		reflect.DeepEqual(a.Spec.Replicas, b.Spec.Replicas) &&
		reflect.DeepEqual(a.Spec.Selector, b.Spec.Selector) &&
		a.Spec.Template.Spec.ServiceAccountName == b.Spec.Template.Spec.ServiceAccountName &&
		reflect.DeepEqual(a.Spec.Template.Spec.Containers[0].Image, b.Spec.Template.Spec.Containers[0].Image) &&
		reflect.DeepEqual(a.Spec.Template.Spec.Containers[0].Command, b.Spec.Template.Spec.Containers[0].Command) &&
		reflect.DeepEqual(a.Spec.Template.Spec.Containers[0].Args, b.Spec.Template.Spec.Containers[0].Args) &&
		reflect.DeepEqual(a.Spec.Template.Spec.Containers[0].VolumeMounts, b.Spec.Template.Spec.Containers[0].VolumeMounts) &&
		AreEnvVarsEqual(a.Spec.Template.Spec.Containers[0].Env, b.Spec.Template.Spec.Containers[0].Env) &&
		AreResouceRequirementsEqual(a.Spec.Template.Spec.Containers[0].Resources, b.Spec.Template.Spec.Containers[0].Resources) &&
		reflect.DeepEqual(a.Spec.Template.Spec.Volumes, b.Spec.Template.Spec.Volumes) &&
		reflect.DeepEqual(a.Spec.Template.Spec.Affinity, b.Spec.Template.Spec.Affinity) &&
		AreSecurityContextsEqual(a.Spec.Template.Spec.Containers[0].SecurityContext, b.Spec.Template.Spec.Containers[0].SecurityContext) &&
		reflect.DeepEqual(a.GetOwnerReferences(), b.GetOwnerReferences())
}

// AreSecurityContextsEqual checks whether the provided Pod SecurityContexts are equal
// for the fields we are interested in.
func AreSecurityContextsEqual(a, b *corev1.SecurityContext) bool {
	// If both left undefined, then they're equal to us
	if a == nil && b == nil {
		return true
	}
	// If not both are undefined, but one is, then unequal
	if a == nil || b == nil {
		return false
	}

	// Finally do the field comparisons for the filds we care about
	return reflect.DeepEqual(a.AllowPrivilegeEscalation, b.AllowPrivilegeEscalation) &&
		reflect.DeepEqual(a.ReadOnlyRootFilesystem, b.ReadOnlyRootFilesystem) &&
		reflect.DeepEqual(a.RunAsNonRoot, b.RunAsNonRoot) &&
		reflect.DeepEqual(a.Capabilities, b.Capabilities) &&
		reflect.DeepEqual(a.RunAsUser, b.RunAsUser)
}

// AreServicesEqual return a value indicating whether 2 services are equal. Note that it
// does not perform a full comparison but checks just some of the properties of a deployment
// (only the ones we are currently interested at).
func AreServicesEqual(a, b corev1.Service) bool {
	return reflect.DeepEqual(a.Spec.Ports, b.Spec.Ports) &&
		reflect.DeepEqual(a.Spec.Selector, b.Spec.Selector) &&
		reflect.DeepEqual(a.GetOwnerReferences(), b.GetOwnerReferences()) &&
		a.Spec.Type == b.Spec.Type
}

// AreCronJobsEqual returns a value indicating whether 2 cron jobs are equal. Note that it does not perform a full
// comparison but checks just some of the properties of a deployment (only the ones we are currently interested at).
func AreCronJobsEqual(a, b batchv1.CronJob) bool {
	aPodSpec := a.Spec.JobTemplate.Spec.Template.Spec
	bPodSpec := b.Spec.JobTemplate.Spec.Template.Spec
	return len(aPodSpec.Containers) == len(bPodSpec.Containers) &&
		aPodSpec.ServiceAccountName == bPodSpec.ServiceAccountName &&
		reflect.DeepEqual(aPodSpec.Tolerations, bPodSpec.Tolerations) &&
		reflect.DeepEqual(aPodSpec.NodeName, bPodSpec.NodeName) &&
		reflect.DeepEqual(aPodSpec.Containers[0].Image, bPodSpec.Containers[0].Image) &&
		reflect.DeepEqual(aPodSpec.Containers[0].Command, bPodSpec.Containers[0].Command) &&
		reflect.DeepEqual(aPodSpec.Containers[0].Args, bPodSpec.Containers[0].Args) &&
		reflect.DeepEqual(aPodSpec.Containers[0].VolumeMounts, bPodSpec.Containers[0].VolumeMounts) &&
		AreEnvVarsEqual(aPodSpec.Containers[0].Env, bPodSpec.Containers[0].Env) &&
		AreResouceRequirementsEqual(aPodSpec.Containers[0].Resources, bPodSpec.Containers[0].Resources) &&
		AreSecurityContextsEqual(aPodSpec.Containers[0].SecurityContext, bPodSpec.Containers[0].SecurityContext) &&
		reflect.DeepEqual(aPodSpec.Volumes, bPodSpec.Volumes) &&
		reflect.DeepEqual(a.Spec.SuccessfulJobsHistoryLimit, b.Spec.SuccessfulJobsHistoryLimit) &&
		reflect.DeepEqual(a.Spec.ConcurrencyPolicy, b.Spec.ConcurrencyPolicy) &&
		a.Spec.Schedule == b.Spec.Schedule &&
		reflect.DeepEqual(a.Spec.FailedJobsHistoryLimit, b.Spec.FailedJobsHistoryLimit) &&
		reflect.DeepEqual(a.GetOwnerReferences(), b.GetOwnerReferences())
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

// AreEnvVarsEqual returns a value indicating whether 2 slices of environment variables are equal. Ordering
// is ignored.
func AreEnvVarsEqual(a, b []corev1.EnvVar) bool {
	return cmp.Equal(a, b, cmpopts.SortSlices(func(a, b corev1.EnvVar) bool { return a.Name < b.Name }))
}
