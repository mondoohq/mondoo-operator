// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFiltering(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		includedList   []string
		excludedList   []string
		expectedResult bool
	}{
		{
			name:           "no lists provided",
			input:          "any-namespace",
			expectedResult: true,
		},
		{
			name:           "explictly excluded",
			input:          "test-namespace",
			excludedList:   []string{"test-namespace"},
			expectedResult: false,
		},
		{
			name:           "explictly included",
			input:          "test-namespace",
			includedList:   []string{"test-namespace"},
			expectedResult: true,
		},
		{
			name:           "on both include and exclude list",
			input:          "test-namespace",
			includedList:   []string{"test-namespace"},
			excludedList:   []string{"test-namespace"},
			expectedResult: true,
		},
		{
			name:           "not on include list",
			input:          "test-namespace",
			includedList:   []string{"other-namespace"},
			expectedResult: false,
		},
		{
			name:           "not on exclude list",
			input:          "test-namespace",
			excludedList:   []string{"other-namespace"},
			expectedResult: true,
		},
		{
			name:           "include glob middle of string",
			input:          "test-namespace",
			includedList:   []string{"*name*"},
			expectedResult: true,
		},
		{
			name:           "exclude glob beginning of string",
			input:          "test-namespace",
			excludedList:   []string{"test*"},
			expectedResult: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := AllowNamespace(test.input, test.includedList, test.excludedList)

			require.NoError(t, err, "unexpected error testing whether a Namespace should be allowed or not")

			assert.Equal(t, test.expectedResult, result, "unexpected result when checking namespace filtering")
		})
	}
}
