// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import "strings"

// InheritNamespaceLabelsOption is the k8s provider inventory option (and resource
// watcher flag) naming the namespace label keys workloads inherit.
const InheritNamespaceLabelsOption = "inherit-namespace-labels"

// InheritNamespaceLabelsOptions converts spec.inheritNamespaceLabels into the
// provider option. Entries are trimmed and empty ones dropped; with nothing left
// the result is an empty map, so callers can maps.Copy it unconditionally.
func InheritNamespaceLabelsOptions(keys []string) map[string]string {
	kept := make([]string, 0, len(keys))
	for _, key := range keys {
		if key = strings.TrimSpace(key); key != "" {
			kept = append(kept, key)
		}
	}
	if len(kept) == 0 {
		return map[string]string{}
	}
	return map[string]string{InheritNamespaceLabelsOption: strings.Join(kept, ",")}
}
