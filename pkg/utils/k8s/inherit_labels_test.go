// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInheritNamespaceLabelsOptions(t *testing.T) {
	tests := []struct {
		name     string
		keys     []string
		expected map[string]string
	}{
		{name: "nil", keys: nil, expected: map[string]string{}},
		{name: "empty entries only", keys: []string{"", "  "}, expected: map[string]string{}},
		{
			name:     "keys and globs, trimmed",
			keys:     []string{"my.company.com/team", " my.company.com/* ", ""},
			expected: map[string]string{InheritNamespaceLabelsOption: "my.company.com/team,my.company.com/*"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, InheritNamespaceLabelsOptions(tt.keys))
		})
	}
}
