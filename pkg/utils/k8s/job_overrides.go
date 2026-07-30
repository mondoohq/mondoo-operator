// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	batchv1 "k8s.io/api/batch/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

// ApplyJobOverrides applies user-configured overrides to a generated scan CronJob.
func ApplyJobOverrides(cj *batchv1.CronJob, o v1alpha2.JobOverrides) {
	if o.TTLSecondsAfterFinished != nil {
		cj.Spec.JobTemplate.Spec.TTLSecondsAfterFinished = o.TTLSecondsAfterFinished
	}

	if len(o.Annotations) > 0 {
		cj.Spec.JobTemplate.Annotations = mergeUserAnnotations(cj.Spec.JobTemplate.Annotations, o.Annotations)
		cj.Spec.JobTemplate.Spec.Template.Annotations = mergeUserAnnotations(cj.Spec.JobTemplate.Spec.Template.Annotations, o.Annotations)
	}

	podSpec := &cj.Spec.JobTemplate.Spec.Template.Spec
	// Node scan pods are pinned via nodeName. The kubelet rejects pinned pods whose
	// nodeSelector doesn't match the node, so the selector is only applied to pods
	// that go through the scheduler.
	if len(o.NodeSelector) > 0 && podSpec.NodeName == "" {
		podSpec.NodeSelector = o.NodeSelector
	}

	podSpec.Tolerations = append(podSpec.Tolerations, o.Tolerations...)
}

// mergeUserAnnotations merges user-defined annotations with operator-managed ones.
// Operator-managed annotations take precedence and cannot be overwritten.
func mergeUserAnnotations(operator, user map[string]string) map[string]string {
	merged := make(map[string]string, len(operator)+len(user))
	for k, v := range user {
		merged[k] = v
	}
	for k, v := range operator {
		merged[k] = v
	}
	return merged
}
