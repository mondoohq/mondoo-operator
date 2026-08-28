// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// nodeScanMemoryLimitWarnRatio is the fraction of a node's allocatable memory above
// which a configured node scan memory limit is reported as unsafe.
//
// The concern is not that the scan might be OOMKilled - that is a contained,
// self-announcing failure. It is that node scan pods are pinned with
// PodSpec.NodeName and therefore bypass the scheduler, so nothing validates the
// limit against the node. Once the limit approaches the node's capacity the
// pod's cgroup ceiling can no longer be reached before the node itself runs out
// of memory, and the kernel switches from a contained per-cgroup OOM kill
// (oom-kill:constraint=CONSTRAINT_MEMCG) to global reclaim, which can starve
// kubelet and take the node NotReady. GOMEMLIMIT is derived from the same limit,
// so cnspec is actively told to grow into that space.
//
// 0.7 is deliberately permissive: it is meant to catch "this limit is the size of
// the node", not to second-guess reasonable tuning.
const nodeScanMemoryLimitWarnRatio = 0.7

// UnsafeNodeScanMemoryLimit reports whether the configured node scan memory limit is
// large enough relative to the node's allocatable memory that the kernel may not be
// able to contain an overrunning scan within its own cgroup. It returns the limit,
// the node's allocatable memory and true when the limit is considered unsafe.
//
// Both a missing/zero limit and missing/zero allocatable memory are treated as "not
// unsafe": there is nothing meaningful to compare.
func UnsafeNodeScanMemoryLimit(resources corev1.ResourceRequirements, node corev1.Node) (limit, allocatable resource.Quantity, unsafe bool) {
	memLimit := resources.Limits.Memory()
	if memLimit == nil || memLimit.IsZero() {
		return resource.Quantity{}, resource.Quantity{}, false
	}

	memAllocatable, ok := node.Status.Allocatable[corev1.ResourceMemory]
	if !ok || memAllocatable.IsZero() {
		return resource.Quantity{}, resource.Quantity{}, false
	}

	if memLimit.AsApproximateFloat64() <= nodeScanMemoryLimitWarnRatio*memAllocatable.AsApproximateFloat64() {
		return *memLimit, memAllocatable, false
	}

	return *memLimit, memAllocatable, true
}

// nodeScanMemoryLimitWarning renders the operator-facing warning message for an
// unsafe node scan memory limit.
func nodeScanMemoryLimitWarning(limit, allocatable resource.Quantity) string {
	return fmt.Sprintf(
		"Node scan memory limit (%s) is %.0f%% of the node's allocatable memory (%s). "+
			"Node scan pods are pinned with spec.nodeName and bypass the scheduler, so this limit is not "+
			"validated against the node. At this size the kernel may be unable to contain an overrunning "+
			"scan within its own cgroup and can starve kubelet instead of OOMKilling the scan. "+
			"Consider lowering spec.nodes.resources.limits.memory to at most %d%% of the smallest node's "+
			"allocatable memory.",
		limit.String(),
		100*float64(limit.Value())/float64(allocatable.Value()),
		allocatable.String(),
		int(nodeScanMemoryLimitWarnRatio*100),
	)
}
