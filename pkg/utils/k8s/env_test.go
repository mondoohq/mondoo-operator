// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestMergeEnv_NoDuplicates(t *testing.T) {
	a := []corev1.EnvVar{
		{Name: "a", Value: "2"},
		{Name: "a1", Value: "3"},
	}

	b := []corev1.EnvVar{
		{Name: "b", Value: "6"},
		{Name: "b1", Value: "7"},
	}

	env := MergeEnv(a, b)
	expected := []corev1.EnvVar{
		{Name: "a", Value: "2"},
		{Name: "a1", Value: "3"},
		{Name: "b", Value: "6"},
		{Name: "b1", Value: "7"},
	}

	assert.ElementsMatch(t, expected, env)
}

func TestMergeEnv_Duplicates(t *testing.T) {
	a := []corev1.EnvVar{
		{Name: "a", Value: "2"},
		{Name: "a1", Value: "3"},
	}

	b := []corev1.EnvVar{
		{Name: "b", Value: "6"},
		{Name: "b1", Value: "7"},
		{Name: "a1", Value: "17"},
	}

	env := MergeEnv(a, b)
	expected := []corev1.EnvVar{
		{Name: "a", Value: "2"},
		{Name: "a1", Value: "17"}, // value is from b
		{Name: "b", Value: "6"},
		{Name: "b1", Value: "7"},
	}

	assert.ElementsMatch(t, expected, env)
}

func TestMergeEnv_AEmpty(t *testing.T) {
	a := []corev1.EnvVar{}

	b := []corev1.EnvVar{
		{Name: "b", Value: "6"},
		{Name: "b1", Value: "7"},
		{Name: "a1", Value: "17"},
	}

	env := MergeEnv(a, b)
	expected := []corev1.EnvVar{
		{Name: "b", Value: "6"},
		{Name: "b1", Value: "7"},
		{Name: "a1", Value: "17"},
	}

	assert.ElementsMatch(t, expected, env)
}

func TestMergeEnv_BEmpty(t *testing.T) {
	a := []corev1.EnvVar{
		{Name: "a", Value: "2"},
		{Name: "a1", Value: "3"},
	}

	b := []corev1.EnvVar{}

	env := MergeEnv(a, b)
	expected := []corev1.EnvVar{
		{Name: "a", Value: "2"},
		{Name: "a1", Value: "3"},
	}

	assert.ElementsMatch(t, expected, env)
}
