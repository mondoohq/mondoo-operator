// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

// MergeJobOverrides deep-merges a global JobOverrides with a per-scan-type
// override. Per-type values take precedence on key conflicts for maps; for
// scalar pointers, per-type wins if non-nil; tolerations are union-merged
// with dedup.
func MergeJobOverrides(global, perType v1alpha2.JobOverrides) v1alpha2.JobOverrides {
	merged := v1alpha2.JobOverrides{
		TTLSecondsAfterFinished: global.TTLSecondsAfterFinished,
		Labels:                  mergeMaps(global.Labels, perType.Labels),
		Annotations:             mergeMaps(global.Annotations, perType.Annotations),
		NodeSelector:            mergeMaps(global.NodeSelector, perType.NodeSelector),
		Tolerations:             MergeTolerations(global.Tolerations, perType.Tolerations),
	}
	if perType.TTLSecondsAfterFinished != nil {
		merged.TTLSecondsAfterFinished = perType.TTLSecondsAfterFinished
	}
	return merged
}

// mergeMaps deep-merges two string maps. Values in the override map take
// precedence on key conflicts. Returns nil when both inputs are nil.
func mergeMaps(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := make(map[string]string, len(base)+len(override))
	maps.Copy(merged, base)
	maps.Copy(merged, override)
	return merged
}

// MergeTolerations returns the union of two toleration slices, deduplicating
// by (Key, Operator, Value, Effect, TolerationSeconds). The first slice's
// entries take precedence when duplicates exist.
func MergeTolerations(existing, additional []corev1.Toleration) []corev1.Toleration {
	if len(existing) == 0 && len(additional) == 0 {
		return nil
	}

	type tolerationKey struct {
		Key                  string
		Operator             corev1.TolerationOperator
		Value                string
		Effect               corev1.TaintEffect
		TolerationSecondsSet bool
		TolerationSeconds    int64
	}

	keyFor := func(t corev1.Toleration) tolerationKey {
		k := tolerationKey{
			Key:      t.Key,
			Operator: t.Operator,
			Value:    t.Value,
			Effect:   t.Effect,
		}
		if t.TolerationSeconds != nil {
			k.TolerationSecondsSet = true
			k.TolerationSeconds = *t.TolerationSeconds
		}
		return k
	}

	seen := make(map[tolerationKey]struct{}, len(existing))
	result := make([]corev1.Toleration, 0, len(existing)+len(additional))

	for _, t := range existing {
		seen[keyFor(t)] = struct{}{}
		result = append(result, *t.DeepCopy())
	}
	for _, t := range additional {
		if _, ok := seen[keyFor(t)]; ok {
			continue
		}
		seen[keyFor(t)] = struct{}{}
		result = append(result, *t.DeepCopy())
	}
	return result
}

// ApplyJobOverrides applies user-configured overrides to a generated scan CronJob.
func ApplyJobOverrides(cj *batchv1.CronJob, o v1alpha2.JobOverrides) {
	if o.TTLSecondsAfterFinished != nil {
		cj.Spec.JobTemplate.Spec.TTLSecondsAfterFinished = o.TTLSecondsAfterFinished
	}

	if len(o.Labels) > 0 {
		cj.Spec.JobTemplate.Labels = mergeUserMetadata(cj.Spec.JobTemplate.Labels, o.Labels)
		cj.Spec.JobTemplate.Spec.Template.Labels = mergeUserMetadata(cj.Spec.JobTemplate.Spec.Template.Labels, o.Labels)
	}

	if len(o.Annotations) > 0 {
		cj.Spec.JobTemplate.Annotations = mergeUserMetadata(cj.Spec.JobTemplate.Annotations, o.Annotations)
		cj.Spec.JobTemplate.Spec.Template.Annotations = mergeUserMetadata(cj.Spec.JobTemplate.Spec.Template.Annotations, o.Annotations)
	}

	podSpec := &cj.Spec.JobTemplate.Spec.Template.Spec
	// Node scan pods are pinned via nodeName. The kubelet rejects pinned pods whose
	// nodeSelector doesn't match the node, so the selector is only applied to pods
	// that go through the scheduler.
	if len(o.NodeSelector) > 0 && podSpec.NodeName == "" {
		podSpec.NodeSelector = o.NodeSelector
	}

	podSpec.Tolerations = MergeTolerations(podSpec.Tolerations, o.Tolerations)
}

// ApplyDeploymentOverrides applies user-configured overrides to a generated Deployment.
func ApplyDeploymentOverrides(d *appsv1.Deployment, o v1alpha2.JobOverrides) {
	if len(o.Labels) > 0 {
		d.Spec.Template.Labels = mergeUserMetadata(d.Spec.Template.Labels, o.Labels)
	}

	if len(o.Annotations) > 0 {
		d.Spec.Template.Annotations = mergeUserMetadata(d.Spec.Template.Annotations, o.Annotations)
	}

	podSpec := &d.Spec.Template.Spec
	if len(o.NodeSelector) > 0 {
		podSpec.NodeSelector = o.NodeSelector
	}

	podSpec.Tolerations = MergeTolerations(podSpec.Tolerations, o.Tolerations)
}

// ApplyDaemonSetOverrides applies user-configured overrides to a generated DaemonSet.
func ApplyDaemonSetOverrides(ds *appsv1.DaemonSet, o v1alpha2.JobOverrides) {
	if len(o.Labels) > 0 {
		ds.Spec.Template.Labels = mergeUserMetadata(ds.Spec.Template.Labels, o.Labels)
	}

	if len(o.Annotations) > 0 {
		ds.Spec.Template.Annotations = mergeUserMetadata(ds.Spec.Template.Annotations, o.Annotations)
	}

	podSpec := &ds.Spec.Template.Spec
	if len(o.NodeSelector) > 0 {
		podSpec.NodeSelector = o.NodeSelector
	}

	podSpec.Tolerations = MergeTolerations(podSpec.Tolerations, o.Tolerations)
}

// mergeUserMetadata merges user-defined labels or annotations with operator-managed ones.
// Operator-managed values take precedence and cannot be overwritten.
func mergeUserMetadata(operator, user map[string]string) map[string]string {
	merged := make(map[string]string, len(operator)+len(user))
	maps.Copy(merged, user)
	maps.Copy(merged, operator)
	return merged
}
