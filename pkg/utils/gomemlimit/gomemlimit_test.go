// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package gomemlimit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestCalculateGoMemLimit(t *testing.T) {
	tests := []struct {
		name               string
		resources          v1.ResourceRequirements
		expectedGoMemLimit string
	}{
		{
			name: "resources should match default",
			resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceMemory: resource.MustParse("250M"),
				},
			},
			expectedGoMemLimit: "225000000",
		},
		{
			name: "resources should match spec",
			resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceMemory: resource.MustParse("100Mi"),
				},
			},
			expectedGoMemLimit: "94371840",
		},
		{
			name: "resources should match off",
			resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceCPU: resource.MustParse("1"),
				},
			},
			expectedGoMemLimit: "off",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedGoMemLimit, CalculateGoMemLimit(test.resources))
		})
	}
}
