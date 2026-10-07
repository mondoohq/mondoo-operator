// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

func TestLabelSelectorOptions(t *testing.T) {
	tests := []struct {
		name      string
		filtering v1alpha2.Filtering
		expected  map[string]string
	}{
		{
			name:      "unset selectors",
			filtering: v1alpha2.Filtering{},
			expected:  map[string]string{},
		},
		{
			name: "empty selectors",
			filtering: v1alpha2.Filtering{
				NamespaceLabelSelector: &metav1.LabelSelector{},
				ObjectLabelSelector:    &metav1.LabelSelector{},
			},
			expected: map[string]string{},
		},
		{
			name: "both selectors",
			filtering: v1alpha2.Filtering{
				NamespaceLabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"tenant": "team-a"},
				},
				ObjectLabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "scan", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"disabled"}},
					},
				},
			},
			expected: map[string]string{
				NamespaceLabelSelectorOption: "tenant=team-a",
				ObjectLabelSelectorOption:    "scan notin (disabled)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := LabelSelectorOptions(tt.filtering)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, opts)
		})
	}
}

func TestLabelSelectorOptions_Invalid(t *testing.T) {
	_, err := LabelSelectorOptions(v1alpha2.Filtering{
		NamespaceLabelSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "tenant", Operator: metav1.LabelSelectorOperator("DefinitelyInvalid")},
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filtering.namespaceLabelSelector")
	var selectorErr InvalidLabelSelectorError
	assert.True(t, errors.As(err, &selectorErr))
}
