// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package common

import (
	"testing"
)

func TestShouldBypassProxy(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		noProxy  string
		expected bool
	}{
		// Empty noProxy
		{
			name:     "empty noProxy returns false",
			host:     "api.mondoo.com",
			noProxy:  "",
			expected: false,
		},
		// Wildcard "*"
		{
			name:     "wildcard matches any host",
			host:     "api.mondoo.com",
			noProxy:  "*",
			expected: true,
		},
		{
			name:     "wildcard matches localhost",
			host:     "localhost",
			noProxy:  "*",
			expected: true,
		},
		{
			name:     "wildcard matches IP address",
			host:     "10.1.2.3",
			noProxy:  "*",
			expected: true,
		},
		// Exact match
		{
			name:     "exact match matches",
			host:     "api.mondoo.com",
			noProxy:  "api.mondoo.com",
			expected: true,
		},
		{
			name:     "exact match does not match different host",
			host:     "other.mondoo.com",
			noProxy:  "api.mondoo.com",
			expected: false,
		},
		{
			name:     "exact match is case insensitive",
			host:     "API.MONDOO.COM",
			noProxy:  "api.mondoo.com",
			expected: true,
		},
		// Domain suffix with leading dot
		{
			name:     "domain suffix .example.com matches api.example.com",
			host:     "api.example.com",
			noProxy:  ".example.com",
			expected: true,
		},
		{
			name:     "domain suffix .example.com matches foo.bar.example.com",
			host:     "foo.bar.example.com",
			noProxy:  ".example.com",
			expected: true,
		},
		{
			name:     "domain suffix .example.com matches example.com exactly",
			host:     "example.com",
			noProxy:  ".example.com",
			expected: true,
		},
		{
			name:     "domain suffix .example.com does not match notexample.com",
			host:     "notexample.com",
			noProxy:  ".example.com",
			expected: false,
		},
		// Suffix without leading dot
		{
			name:     "suffix example.com matches api.example.com",
			host:     "api.example.com",
			noProxy:  "example.com",
			expected: true,
		},
		{
			name:     "suffix example.com matches example.com exactly",
			host:     "example.com",
			noProxy:  "example.com",
			expected: true,
		},
		{
			name:     "suffix example.com does not match notexample.com",
			host:     "notexample.com",
			noProxy:  "example.com",
			expected: false,
		},
		// CIDR notation
		{
			name:     "CIDR 10.0.0.0/8 matches 10.1.2.3",
			host:     "10.1.2.3",
			noProxy:  "10.0.0.0/8",
			expected: true,
		},
		{
			name:     "CIDR 10.0.0.0/8 matches 10.255.255.255",
			host:     "10.255.255.255",
			noProxy:  "10.0.0.0/8",
			expected: true,
		},
		{
			name:     "CIDR 10.0.0.0/8 does not match 192.168.1.1",
			host:     "192.168.1.1",
			noProxy:  "10.0.0.0/8",
			expected: false,
		},
		{
			name:     "CIDR 192.168.0.0/16 matches 192.168.1.1",
			host:     "192.168.1.1",
			noProxy:  "192.168.0.0/16",
			expected: true,
		},
		{
			name:     "CIDR does not apply to hostnames",
			host:     "api.mondoo.com",
			noProxy:  "10.0.0.0/8",
			expected: false,
		},
		// Host with port
		{
			name:     "host with port strips port before matching",
			host:     "api.mondoo.com:443",
			noProxy:  "api.mondoo.com",
			expected: true,
		},
		{
			name:     "host with port matches domain suffix",
			host:     "api.example.com:8080",
			noProxy:  ".example.com",
			expected: true,
		},
		{
			name:     "IP with port matches CIDR",
			host:     "10.1.2.3:9000",
			noProxy:  "10.0.0.0/8",
			expected: true,
		},
		// Multiple entries
		{
			name:     "multiple entries matches first",
			host:     "localhost",
			noProxy:  "localhost,.local,10.0.0.0/8",
			expected: true,
		},
		{
			name:     "multiple entries matches second",
			host:     "myhost.local",
			noProxy:  "localhost,.local,10.0.0.0/8",
			expected: true,
		},
		{
			name:     "multiple entries matches third CIDR",
			host:     "10.1.2.3",
			noProxy:  "localhost,.local,10.0.0.0/8",
			expected: true,
		},
		{
			name:     "multiple entries does not match any",
			host:     "api.mondoo.com",
			noProxy:  "localhost,.local,10.0.0.0/8",
			expected: false,
		},
		// Whitespace handling
		{
			name:     "whitespace around entries is trimmed",
			host:     "localhost",
			noProxy:  " localhost , .local ",
			expected: true,
		},
		{
			name:     "whitespace in entries is trimmed for domain",
			host:     "api.local",
			noProxy:  " localhost , .local ",
			expected: true,
		},
		{
			name:     "empty entries after split are ignored",
			host:     "localhost",
			noProxy:  "localhost,,,.local",
			expected: true,
		},
		// Edge cases
		{
			name:     "single localhost entry",
			host:     "localhost",
			noProxy:  "localhost",
			expected: true,
		},
		{
			name:     "invalid CIDR is ignored",
			host:     "10.1.2.3",
			noProxy:  "invalid/cidr",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldBypassProxy(tt.host, tt.noProxy)
			if result != tt.expected {
				t.Errorf("shouldBypassProxy(%q, %q) = %v, expected %v",
					tt.host, tt.noProxy, result, tt.expected)
			}
		})
	}
}
