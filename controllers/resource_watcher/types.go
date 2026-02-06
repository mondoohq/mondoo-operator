// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"fmt"
	"strings"
)

// K8sResourceIdentifier identifies a specific K8s resource.
type K8sResourceIdentifier struct {
	Type      string // plural form, e.g., "deployments", "ingresses"
	Namespace string // empty for cluster-scoped resources
	Name      string
}

// String returns the resource identifier in the format expected by cnspec's k8s-resources option.
// Format: type:namespace:name for namespaced, type:name for cluster-scoped
// Note: cnspec expects singular type names (e.g., "deployment" not "deployments")
func (r K8sResourceIdentifier) String() string {
	singularType := ToSingular(r.Type)
	if r.Namespace == "" {
		return fmt.Sprintf("%s:%s", singularType, r.Name)
	}
	return fmt.Sprintf("%s:%s:%s", singularType, r.Namespace, r.Name)
}

// resourceTypePluralization maps plural resource type names to their singular form.
// This is needed because Kubernetes uses plural forms (e.g., "ingresses") but cnspec
// expects singular forms (e.g., "ingress") for resource filtering.
var resourceTypePluralization = map[string]string{
	"pods":            "pod",
	"deployments":     "deployment",
	"daemonsets":      "daemonset",
	"statefulsets":    "statefulset",
	"replicasets":     "replicaset",
	"jobs":            "job",
	"cronjobs":        "cronjob",
	"services":        "service",
	"ingresses":       "ingress",
	"namespaces":      "namespace",
	"configmaps":      "configmap",
	"secrets":         "secret",
	"serviceaccounts": "serviceaccount",
}

// ToSingular converts a plural resource type to singular form.
func ToSingular(plural string) string {
	lower := strings.ToLower(plural)
	if singular, ok := resourceTypePluralization[lower]; ok {
		return singular
	}
	// Unknown type — return lowercase as-is rather than guessing.
	// If this fires, the type should be added to resourceTypePluralization.
	return lower
}
