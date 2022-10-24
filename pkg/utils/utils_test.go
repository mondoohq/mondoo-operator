package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := AllowNamespace(test.input, test.includedList, test.excludedList)

			assert.Equal(t, test.expectedResult, result, "unexpected result when checking namespace filtering")
		})
	}
}
