// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func nodeWithAllocatableMemory(mem string) corev1.Node {
	node := corev1.Node{}
	if mem != "" {
		node.Status.Allocatable = corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(mem),
		}
	}
	return node
}

func resourcesWithMemoryLimit(mem string) corev1.ResourceRequirements {
	if mem == "" {
		return corev1.ResourceRequirements{}
	}
	return corev1.ResourceRequirements{
		Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(mem)},
	}
}

func TestUnsafeNodeScanMemoryLimit(t *testing.T) {
	tests := []struct {
		name         string
		limit        string
		allocatable  string
		expectUnsafe bool
	}{
		{
			name:         "operator default on a small node is safe",
			limit:        "250M",
			allocatable:  "8130536Ki",
			expectUnsafe: false,
		},
		{
			name:         "limit equal to the whole node is unsafe",
			limit:        "8G",
			allocatable:  "8130536Ki",
			expectUnsafe: true,
		},
		{
			name:         "limit larger than the node is unsafe",
			limit:        "16G",
			allocatable:  "8130536Ki",
			expectUnsafe: true,
		},
		{
			name:         "the same limit on a large node is safe",
			limit:        "8G",
			allocatable:  "64Gi",
			expectUnsafe: false,
		},
		{
			name:         "just under the threshold is safe",
			limit:        "5G",
			allocatable:  "8G",
			expectUnsafe: false,
		},
		{
			name:         "just over the threshold is unsafe",
			limit:        "6G",
			allocatable:  "8G",
			expectUnsafe: true,
		},
		{
			name:         "no limit set is not reported",
			limit:        "",
			allocatable:  "8130536Ki",
			expectUnsafe: false,
		},
		{
			name:         "node without allocatable memory is not reported",
			limit:        "8G",
			allocatable:  "",
			expectUnsafe: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			limit, allocatable, unsafe := UnsafeNodeScanMemoryLimit(
				resourcesWithMemoryLimit(test.limit),
				nodeWithAllocatableMemory(test.allocatable),
			)
			assert.Equal(t, test.expectUnsafe, unsafe)

			if !unsafe {
				return
			}

			// The rendered warning must name both numbers so an operator can act on it
			// without going back to the cluster.
			msg := nodeScanMemoryLimitWarning(limit, allocatable)
			assert.Contains(t, msg, limit.String())
			assert.Contains(t, msg, allocatable.String())
			assert.Contains(t, msg, "spec.nodes.resources.limits.memory")
		})
	}
}

func TestNodeScanMemoryLimitWarning_ReportsPercentage(t *testing.T) {
	limit, allocatable, unsafe := UnsafeNodeScanMemoryLimit(
		resourcesWithMemoryLimit("8G"),
		nodeWithAllocatableMemory("8G"),
	)
	assert.True(t, unsafe)
	assert.Contains(t, nodeScanMemoryLimitWarning(limit, allocatable), "100%")
}
