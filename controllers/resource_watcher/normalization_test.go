// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNormalizeResourceTypes(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "singular plural and casing aliases",
			input: []string{"Deployment", "deployments", " DAEMONSET ", "statefulset", "StatefulSets"},
			want:  []string{"deployments", "daemonsets", "statefulsets"},
		},
		{
			name:  "duplicate canonical types retain first occurrence",
			input: []string{"pods", "POD", " pods ", "services", "service"},
			want:  []string{"pods", "services"},
		},
		{
			name:  "blank entries are ignored and unknown types are preserved",
			input: []string{" ", "ConfigMap", "widgets", "", "WIDGETS"},
			want:  []string{"configmaps", "widgets"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeResourceTypes(tt.input))
		})
	}
}

func TestResourceEventHandlerKeepsSameNameInDifferentNamespaces(t *testing.T) {
	d := NewDebouncer(time.Hour, 0, func(_ context.Context, _ []K8sResourceIdentifier) error { return nil })
	h := &resourceEventHandler{
		watcher:      &ResourceWatcher{debouncer: d},
		resourceType: "pods",
	}

	h.handleEvent(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "team-a", Name: "api"}}, "add")
	h.handleEvent(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "team-b", Name: "api"}}, "add")

	d.mu.Lock()
	defer d.mu.Unlock()
	assert.Len(t, d.pending, 2)
	assert.Contains(t, d.pending, "team-a/pods/api")
	assert.Contains(t, d.pending, "team-b/pods/api")
}

func BenchmarkNormalizeResourceTypes(b *testing.B) {
	input := []string{
		"Pod", "pods", "Deployment", "deployments", "DaemonSet", "daemonsets",
		"StatefulSet", "statefulsets", "ReplicaSet", "replicasets", "Job", "jobs",
		"CronJob", "cronjobs", "Service", "services", "Ingress", "ingresses",
		"Namespace", "namespaces", "ConfigMap", "configmaps", "Secret", "secrets",
		"ServiceAccount", "serviceaccounts", "pods", " deployments ", "POD",
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = normalizeResourceTypes(input)
	}
}
