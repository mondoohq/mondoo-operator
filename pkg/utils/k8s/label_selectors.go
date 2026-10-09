// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

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
	if err := addLabelSelectorOption(opts, NamespaceLabelSelectorOption, "namespaceLabelSelector", f.NamespaceLabelSelector); err != nil {
		return nil, err
	}
	if err := addLabelSelectorOption(opts, ObjectLabelSelectorOption, "objectLabelSelector", f.ObjectLabelSelector); err != nil {
		return nil, err
	}
	return opts, nil
}

func addLabelSelectorOption(opts map[string]string, option, field string, labelSelector *metav1.LabelSelector) error {
	if labelSelector == nil {
		return nil
	}
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return fmt.Errorf("invalid filtering.%s: %w", field, err)
	}
	if selector.Empty() {
		return nil
	}
	opts[option] = selector.String()
	return nil
}
