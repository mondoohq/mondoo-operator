// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSpaceMrnFromServiceAccountMrn(t *testing.T) {
	tests := []struct {
		name   string
		saMrn  string
		expect string
	}{
		{
			name:   "standard SA MRN",
			saMrn:  "//agents.api.mondoo.app/spaces/abc123/serviceaccounts/sa456",
			expect: "//captain.api.mondoo.app/spaces/abc123",
		},
		{
			name:   "empty MRN",
			saMrn:  "",
			expect: "",
		},
		{
			name:   "no spaces segment",
			saMrn:  "//agents.api.mondoo.app/organizations/org1",
			expect: "",
		},
		{
			name:   "spaces segment with no ID after it",
			saMrn:  "//agents.api.mondoo.app/spaces/",
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, SpaceMrnFromServiceAccountMrn(tt.saMrn))
		})
	}
}
