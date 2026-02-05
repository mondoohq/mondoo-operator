// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AreCronJobsSuccessful returns true if the latest runs of all of the provided CronJobs has been
// successful.
func AreCronJobsSuccessful(cs []batchv1.CronJob) bool {
	for _, c := range cs {
		// If there are no active jobs at the moment and the last successful run is not before the last
		// scheduled job everything is working correctly.
		if len(c.Status.Active) == 0 && c.Status.LastSuccessfulTime.Before(c.Status.LastScheduleTime) {
			return false
		}
	}
	return true
}

// DeleteCompletedJobs deletes only completed or failed jobs matching the given labels.
// Active/running jobs are preserved to avoid killing in-progress scans.
func DeleteCompletedJobs(ctx context.Context, kubeClient client.Client, namespace string, jobLabels map[string]string, log logr.Logger) error {
	jobList := &batchv1.JobList{}
	listOpts := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labels.SelectorFromSet(jobLabels),
	}

	if err := kubeClient.List(ctx, jobList, listOpts); err != nil {
		log.Error(err, "Failed to list Jobs", "namespace", namespace)
		return err
	}

	for _, job := range jobList.Items {
		// Skip active jobs - only delete completed or failed ones
		if job.Status.Active > 0 {
			log.V(1).Info("Skipping deletion of active job", "namespace", job.Namespace, "name", job.Name)
			continue
		}

		// Delete the job with foreground propagation to also delete its pods
		if err := kubeClient.Delete(ctx, &job, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
			// Ignore NotFound errors - job may have been deleted by TTL controller or another reconcile
			if apierrors.IsNotFound(err) {
				continue
			}
			log.Error(err, "Failed to delete completed job", "namespace", job.Namespace, "name", job.Name)
			return err
		}
		log.V(1).Info("Deleted completed job", "namespace", job.Namespace, "name", job.Name)
	}

	return nil
}
