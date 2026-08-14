// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ProfileForNode returns the node scan profile for a node. It returns the first profile in
// the list whose node selector matches the labels of the node. It returns nil when no profile
// matches, and the caller then uses the settings of spec.nodes.
func ProfileForNode(nodes v1alpha2.Nodes, node corev1.Node) *v1alpha2.NodeScanProfile {
	nodeLabels := labels.Set(node.Labels)
	for i := range nodes.Profiles {
		if labels.Set(nodes.Profiles[i].NodeSelector).AsSelector().Matches(nodeLabels) {
			return &nodes.Profiles[i]
		}
	}
	return nil
}

// ProfileResources returns the resource requirements for a node. A profile without resources
// falls back to spec.nodes.resources, which itself falls back to the node scanning defaults.
func ProfileResources(nodes v1alpha2.Nodes, profile *v1alpha2.NodeScanProfile) corev1.ResourceRequirements {
	if profile != nil && profile.Resources.Size() != 0 {
		return profile.Resources
	}
	return nodes.Resources
}

// ProfileTolerations returns the tolerations for the scan pod of a node. The taints of the node
// always produce tolerations. A matching profile appends its own tolerations after them.
func ProfileTolerations(node corev1.Node, profile *v1alpha2.NodeScanProfile) []corev1.Toleration {
	tolerations := k8s.TaintsToTolerations(node.Spec.Taints)
	if profile == nil || len(profile.Tolerations) == 0 {
		return tolerations
	}
	out := make([]corev1.Toleration, 0, len(tolerations)+len(profile.Tolerations))
	out = append(out, tolerations...)
	out = append(out, profile.Tolerations...)
	return out
}
