// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAreCronJobsSuccessful(t *testing.T) {
	tests := []struct {
		name     string
		cronJobs []batchv1.CronJob
		expected bool
	}{
		{
			name:     "empty list is successful",
			cronJobs: []batchv1.CronJob{},
			expected: true,
		},
		{
			name: "active job is successful",
			cronJobs: []batchv1.CronJob{
				{
					Status: batchv1.CronJobStatus{
						Active: []corev1.ObjectReference{{Name: "job-1"}},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AreCronJobsSuccessful(tt.cronJobs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeleteCompletedJobs(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))

	labels := map[string]string{"app": "test-scan", "mondoo_cr": "test"}
	namespace := "test-ns"

	tests := []struct {
		name              string
		existingJobs      []batchv1.Job
		expectedRemaining int
		expectedDeleted   int
	}{
		{
			name:              "no jobs to delete",
			existingJobs:      []batchv1.Job{},
			expectedRemaining: 0,
			expectedDeleted:   0,
		},
		{
			name: "delete completed job",
			existingJobs: []batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "completed-job",
						Namespace: namespace,
						Labels:    labels,
					},
					Status: batchv1.JobStatus{
						Active:    0,
						Succeeded: 1,
					},
				},
			},
			expectedRemaining: 0,
			expectedDeleted:   1,
		},
		{
			name: "delete failed job",
			existingJobs: []batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "failed-job",
						Namespace: namespace,
						Labels:    labels,
					},
					Status: batchv1.JobStatus{
						Active: 0,
						Failed: 1,
					},
				},
			},
			expectedRemaining: 0,
			expectedDeleted:   1,
		},
		{
			name: "preserve active job",
			existingJobs: []batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "active-job",
						Namespace: namespace,
						Labels:    labels,
					},
					Status: batchv1.JobStatus{
						Active: 1,
					},
				},
			},
			expectedRemaining: 1,
			expectedDeleted:   0,
		},
		{
			name: "mixed jobs - delete completed, preserve active",
			existingJobs: []batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "active-job",
						Namespace: namespace,
						Labels:    labels,
					},
					Status: batchv1.JobStatus{
						Active: 1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "completed-job",
						Namespace: namespace,
						Labels:    labels,
					},
					Status: batchv1.JobStatus{
						Active:    0,
						Succeeded: 1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "failed-job",
						Namespace: namespace,
						Labels:    labels,
					},
					Status: batchv1.JobStatus{
						Active: 0,
						Failed: 1,
					},
				},
			},
			expectedRemaining: 1,
			expectedDeleted:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build fake client with existing jobs
			objs := make([]runtime.Object, len(tt.existingJobs))
			for i := range tt.existingJobs {
				objs[i] = &tt.existingJobs[i]
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

			log := ctrl.Log.WithName("test")

			// Call DeleteCompletedJobs
			err := DeleteCompletedJobs(context.Background(), fakeClient, namespace, labels, log)
			require.NoError(t, err)

			// Verify remaining jobs
			jobList := &batchv1.JobList{}
			err = fakeClient.List(context.Background(), jobList)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedRemaining, len(jobList.Items), "unexpected number of remaining jobs")

			// Verify that remaining jobs are all active
			for _, job := range jobList.Items {
				assert.Greater(t, job.Status.Active, int32(0), "remaining job should be active: %s", job.Name)
			}
		})
	}
}
