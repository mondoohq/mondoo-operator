// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import batchv1 "k8s.io/api/batch/v1"

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
