// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import "go.mondoo.com/mondoo-operator/api/v1alpha2"

const (
	// NamespaceLabelSelectorOption is the k8s provider inventory option (and resource watcher flag)
	// for selecting namespaces by label.
	NamespaceLabelSelectorOption = "namespace-label-selector"
	// ObjectLabelSelectorOption is the k8s provider inventory option (and resource watcher flag)
	// for selecting objects by label.
	ObjectLabelSelectorOption = "object-label-selector"
)

// LabelSelectorOptions converts the label selectors in the filtering config into k8s provider
// inventory options. Unset or empty selectors are omitted from the result.
func LabelSelectorOptions(f v1alpha2.Filtering) (map[string]string, error) {
	opts := map[string]string{}
	if err := AddLabelSelectorOption(opts, NamespaceLabelSelectorOption, "namespaceLabelSelector", f.NamespaceLabelSelector); err != nil {
		return nil, err
	}
	if err := AddLabelSelectorOption(opts, ObjectLabelSelectorOption, "objectLabelSelector", f.ObjectLabelSelector); err != nil {
		return nil, err
	}
	return opts, nil
}
