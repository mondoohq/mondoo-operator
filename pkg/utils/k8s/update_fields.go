// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
)

const daemonSetTemplateGenerationAnnotation = "deprecated.daemonset.template.generation"

// UpdateCronJobFields copies managed fields from desired to obj,
// preserving server-set defaults on unmanaged fields like
// Completions, Parallelism, DNSPolicy, SchedulerName, etc.
func UpdateCronJobFields(obj, desired *batchv1.CronJob) {
	obj.Labels = desired.Labels
	obj.Annotations = desired.Annotations
	obj.Spec.Schedule = desired.Spec.Schedule
	obj.Spec.ConcurrencyPolicy = desired.Spec.ConcurrencyPolicy
	obj.Spec.SuccessfulJobsHistoryLimit = desired.Spec.SuccessfulJobsHistoryLimit
	obj.Spec.FailedJobsHistoryLimit = desired.Spec.FailedJobsHistoryLimit
	obj.Spec.Suspend = desired.Spec.Suspend

	obj.Spec.JobTemplate.Labels = desired.Spec.JobTemplate.Labels
	obj.Spec.JobTemplate.Annotations = desired.Spec.JobTemplate.Annotations
	obj.Spec.JobTemplate.Spec.BackoffLimit = desired.Spec.JobTemplate.Spec.BackoffLimit
	obj.Spec.JobTemplate.Spec.ActiveDeadlineSeconds = desired.Spec.JobTemplate.Spec.ActiveDeadlineSeconds

	obj.Spec.JobTemplate.Spec.Template.Labels = desired.Spec.JobTemplate.Spec.Template.Labels
	obj.Spec.JobTemplate.Spec.Template.Annotations = desired.Spec.JobTemplate.Spec.Template.Annotations

	ps := &obj.Spec.JobTemplate.Spec.Template.Spec
	dps := &desired.Spec.JobTemplate.Spec.Template.Spec
	ps.RestartPolicy = dps.RestartPolicy
	ps.NodeName = dps.NodeName
	ps.Tolerations = dps.Tolerations
	ps.ServiceAccountName = dps.ServiceAccountName
	ps.AutomountServiceAccountToken = dps.AutomountServiceAccountToken
	ps.PriorityClassName = dps.PriorityClassName
	ps.InitContainers = dps.InitContainers
	ps.Containers = dps.Containers
	ps.Volumes = dps.Volumes
	ps.ImagePullSecrets = dps.ImagePullSecrets
}

// UpdateDeploymentFields copies managed fields from desired to obj,
// preserving server-set defaults on unmanaged fields like
// RevisionHistoryLimit, ProgressDeadlineSeconds, Strategy, DNSPolicy, etc.
func UpdateDeploymentFields(obj, desired *appsv1.Deployment) {
	obj.Labels = desired.Labels
	obj.Spec.Replicas = desired.Spec.Replicas
	obj.Spec.Selector = desired.Spec.Selector
	obj.Spec.Template.Labels = desired.Spec.Template.Labels
	obj.Spec.Template.Annotations = desired.Spec.Template.Annotations

	ps := &obj.Spec.Template.Spec
	dps := &desired.Spec.Template.Spec
	ps.ServiceAccountName = dps.ServiceAccountName
	ps.InitContainers = dps.InitContainers
	ps.Containers = dps.Containers
	ps.Volumes = dps.Volumes
	ps.ImagePullSecrets = dps.ImagePullSecrets
}

// UpdateDaemonSetFields copies managed fields from desired to obj,
// preserving server-set defaults on unmanaged fields like
// RevisionHistoryLimit, DNSPolicy, etc.
func UpdateDaemonSetFields(obj, desired *appsv1.DaemonSet) {
	templateGeneration := ""
	if obj.Annotations != nil {
		templateGeneration = obj.Annotations[daemonSetTemplateGenerationAnnotation]
	}
	existingUpdateStrategy := obj.Spec.UpdateStrategy.DeepCopy()

	obj.Labels = desired.Labels
	obj.Annotations = copyStringMap(desired.Annotations)
	if templateGeneration != "" {
		if obj.Annotations == nil {
			obj.Annotations = map[string]string{}
		}
		if _, ok := obj.Annotations[daemonSetTemplateGenerationAnnotation]; !ok {
			obj.Annotations[daemonSetTemplateGenerationAnnotation] = templateGeneration
		}
	}
	obj.Spec.Selector = desired.Spec.Selector
	if desired.Spec.UpdateStrategy.Type == "" && desired.Spec.UpdateStrategy.RollingUpdate == nil {
		obj.Spec.UpdateStrategy = *existingUpdateStrategy
	} else {
		obj.Spec.UpdateStrategy = desired.Spec.UpdateStrategy
		if existingUpdateStrategy.RollingUpdate != nil &&
			obj.Spec.UpdateStrategy.RollingUpdate != nil &&
			obj.Spec.UpdateStrategy.RollingUpdate.MaxSurge == nil {
			obj.Spec.UpdateStrategy.RollingUpdate.MaxSurge = existingUpdateStrategy.RollingUpdate.MaxSurge
		}
	}
	obj.Spec.Template.Labels = desired.Spec.Template.Labels
	obj.Spec.Template.Annotations = desired.Spec.Template.Annotations

	ps := &obj.Spec.Template.Spec
	dps := &desired.Spec.Template.Spec
	ps.ServiceAccountName = dps.ServiceAccountName
	ps.PriorityClassName = dps.PriorityClassName
	ps.AutomountServiceAccountToken = dps.AutomountServiceAccountToken
	ps.NodeSelector = dps.NodeSelector
	ps.Affinity = dps.Affinity
	ps.Tolerations = dps.Tolerations
	ps.InitContainers = dps.InitContainers
	ps.Containers = dps.Containers
	ps.Volumes = dps.Volumes
	ps.ImagePullSecrets = dps.ImagePullSecrets
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
