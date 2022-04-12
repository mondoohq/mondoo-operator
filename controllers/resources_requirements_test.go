package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestRquriementComparison(t *testing.T) {
	assert.True(t, equalResouceRequirements(defaultMondooClientResources, defaultMondooClientResources))
	assert.True(t, equalResouceRequirements(defaultMondooClientResources, corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("1G"),
			corev1.ResourceCPU:    resource.MustParse("0.5"), // used instead of 500m
		},
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("500M"), // 50% of the limit
			corev1.ResourceCPU:    resource.MustParse("50m"),  // 10% of the limit
		},
	}))
}
