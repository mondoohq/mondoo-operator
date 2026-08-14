// Copyright Mondoo, Inc. 2026
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
			name:           "explicitly excluded",
			input:          "test-namespace",
			excludedList:   []string{"test-namespace"},
			expectedResult: false,
		},
		{
			name:           "explicitly included",
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
		{
			name:           "include glob prefix matches",
			input:          "prod-api",
			includedList:   []string{"prod-*"},
			expectedResult: true,
		},
		{
			name:           "include glob prefix does not match",
			input:          "staging-api",
			includedList:   []string{"prod-*"},
			expectedResult: false,
		},
		{
			name:           "include glob among literals",
			input:          "dev-web",
			includedList:   []string{"default", "dev-*"},
			expectedResult: true,
		},
		{
			name:           "exclude glob suffix",
			input:          "api-canary",
			excludedList:   []string{"*-canary"},
			expectedResult: false,
		},
		{
			name:           "exclude glob among literals leaves others allowed",
			input:          "prod-api",
			excludedList:   []string{"kube-system", "*-canary"},
			expectedResult: true,
		},
		{
			name:           "include glob wins over overlapping exclude glob",
			input:          "prod-api",
			includedList:   []string{"prod-*"},
			excludedList:   []string{"prod-*"},
			expectedResult: true,
		},
		{
			name:           "single char wildcard",
			input:          "team-a",
			includedList:   []string{"team-?"},
			expectedResult: true,
		},
		{
			name:           "alternation",
			input:          "backend",
			includedList:   []string{"{frontend,backend}"},
			expectedResult: true,
		},
		{
			name:           "match everything",
			input:          "anything-at-all",
			includedList:   []string{"*"},
			expectedResult: true,
		},
		{
			name:           "exclude everything",
			input:          "anything-at-all",
			excludedList:   []string{"*"},
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

func TestAllowNamespaceInvalidPattern(t *testing.T) {
	// An unterminated character class is not valid glob syntax. Both branches must
	// surface the error rather than silently allowing or denying the namespace.
	t.Run("include", func(t *testing.T) {
		_, err := AllowNamespace("test-namespace", []string{"[bad"}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "include")
	})

	t.Run("exclude", func(t *testing.T) {
		_, err := AllowNamespace("test-namespace", nil, []string{"[bad"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exclude")
	})
}

func TestNewNamespaceFilterCompilesOnce(t *testing.T) {
	f, err := NewNamespaceFilter([]string{"prod-*", "shared"}, []string{"prod-canary"})
	require.NoError(t, err)

	// Include is non-empty, so exclude is not consulted at all.
	assert.True(t, f.Allow("prod-api"))
	assert.True(t, f.Allow("prod-canary"))
	assert.True(t, f.Allow("shared"))
	assert.False(t, f.Allow("staging-api"))
}

func TestIsGlobPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"default", false},
		{"kube-system", false},
		{"mondoo-operator", false},
		{"prod-*", true},
		{"*", true},
		{"team-?", true},
		{"{a,b}", true},
		{"[abc]-ns", true},
		{`esc\-aped`, true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			assert.Equal(t, test.expected, IsGlobPattern(test.input))
		})
	}
}

func TestHasGlobPattern(t *testing.T) {
	assert.False(t, HasGlobPattern(nil))
	assert.False(t, HasGlobPattern([]string{}))
	assert.False(t, HasGlobPattern([]string{"default", "kube-system"}))
	assert.True(t, HasGlobPattern([]string{"default", "prod-*"}))
	assert.True(t, HasGlobPattern([]string{"prod-*"}))
}
