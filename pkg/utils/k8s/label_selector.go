// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InvalidLabelSelectorError identifies invalid scan filtering configuration.
type InvalidLabelSelectorError struct{ err error }

func (e InvalidLabelSelectorError) Error() string { return e.err.Error() }
func (e InvalidLabelSelectorError) Unwrap() error { return e.err }

// AddLabelSelectorOption validates and serializes a non-empty scan selector.
func AddLabelSelectorOption(options map[string]string, optionName, fieldName string, selector *metav1.LabelSelector) error {
	if selector == nil {
		return nil
	}

	parsedSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return InvalidLabelSelectorError{
			err: fmt.Errorf("invalid filtering.%s: %w", fieldName, err),
		}
	}
	if !parsedSelector.Empty() {
		options[optionName] = parsedSelector.String()
	}

	return nil
}
