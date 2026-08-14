// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldWatchNamespace(t *testing.T) {
	tests := []struct {
		name      string
		include   []string
		exclude   []string
		namespace string
		expected  bool
	}{
		{
			name:      "no filtering watches everything",
			namespace: "any-namespace",
			expected:  true,
		},
		{
			name:      "literal include matches",
			include:   []string{"default", "prod"},
			namespace: "prod",
			expected:  true,
		},
		{
			name:      "literal include does not match",
			include:   []string{"default", "prod"},
			namespace: "staging",
			expected:  false,
		},
		{
			name:      "literal exclude matches",
			exclude:   []string{"kube-system"},
			namespace: "kube-system",
			expected:  false,
		},
		{
			name:      "literal exclude does not match",
			exclude:   []string{"kube-system"},
			namespace: "prod",
			expected:  true,
		},
		{
			name:      "glob include matches",
			include:   []string{"prod-*"},
			namespace: "prod-api",
			expected:  true,
		},
		{
			name:      "glob include does not match",
			include:   []string{"prod-*"},
			namespace: "staging-api",
			expected:  false,
		},
		{
			name:      "glob exclude matches",
			exclude:   []string{"kube-*"},
			namespace: "kube-public",
			expected:  false,
		},
		{
			name:      "glob exclude does not match",
			exclude:   []string{"kube-*"},
			namespace: "prod-api",
			expected:  true,
		},
		{
			name:      "glob mixed with literals in include",
			include:   []string{"default", "team-*"},
			namespace: "team-payments",
			expected:  true,
		},
		{
			name:      "include takes precedence over exclude",
			include:   []string{"prod-*"},
			exclude:   []string{"prod-canary"},
			namespace: "prod-canary",
			expected:  true,
		},
		{
			name:      "suffix glob exclude",
			exclude:   []string{"*-canary"},
			namespace: "api-canary",
			expected:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w, err := NewResourceWatcher(nil, nil, WatcherConfig{
				Namespaces:        test.include,
				NamespacesExclude: test.exclude,
			})
			require.NoError(t, err)

			assert.Equal(t, test.expected, w.shouldWatchNamespace(test.namespace))
		})
	}
}

func TestNewResourceWatcherRejectsInvalidNamespacePattern(t *testing.T) {
	t.Run("include", func(t *testing.T) {
		_, err := NewResourceWatcher(nil, nil, WatcherConfig{Namespaces: []string{"[bad"}})
		require.Error(t, err)
	})

	t.Run("exclude", func(t *testing.T) {
		_, err := NewResourceWatcher(nil, nil, WatcherConfig{NamespacesExclude: []string{"[bad"}})
		require.Error(t, err)
	})
}

func TestNewResourceWatcherDefaultResourceTypes(t *testing.T) {
	w, err := NewResourceWatcher(nil, nil, WatcherConfig{})
	require.NoError(t, err)
	assert.Equal(t, HighPriorityResourceTypes, w.config.ResourceTypes)

	w, err = NewResourceWatcher(nil, nil, WatcherConfig{WatchAllResources: true})
	require.NoError(t, err)
	assert.Equal(t, DefaultResourceTypes, w.config.ResourceTypes)
}
