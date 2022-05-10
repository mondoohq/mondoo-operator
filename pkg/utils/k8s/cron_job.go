package k8s

import batchv1 "k8s.io/api/batch/v1"

// AreCronJobsSuccessful returns true if the latest runs of all of the provided CronJobs has been
// successful.
func AreCronJobsSuccessful(cs []batchv1.CronJob) bool {
	for _, c := range cs {
		if c.Status.LastSuccessfulTime.Before(c.Status.LastScheduleTime) {
			return false
		}
	}
	return true
}
