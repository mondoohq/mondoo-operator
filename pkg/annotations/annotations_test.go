// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package annotations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotationArgs(t *testing.T) {
	t.Run("nil map returns nil", func(t *testing.T) {
		assert.Nil(t, AnnotationArgs(nil))
	})

	t.Run("empty map returns nil", func(t *testing.T) {
		assert.Nil(t, AnnotationArgs(map[string]string{}))
	})

	t.Run("single annotation", func(t *testing.T) {
		args := AnnotationArgs(map[string]string{"env": "prod"})
		assert.Equal(t, []string{"--annotation", "env=prod"}, args)
	})

	t.Run("multiple annotations are sorted by key", func(t *testing.T) {
		args := AnnotationArgs(map[string]string{
			"team": "platform",
			"env":  "prod",
			"app":  "mondoo",
		})
		assert.Equal(t, []string{
			"--annotation", "app=mondoo",
			"--annotation", "env=prod",
			"--annotation", "team=platform",
		}, args)
	})
}

func TestValidate(t *testing.T) {
	t.Run("valid annotations", func(t *testing.T) {
		err := Validate(map[string]string{
			"env":  "prod",
			"team": "platform",
		})
		assert.NoError(t, err)
	})

	t.Run("nil map is valid", func(t *testing.T) {
		assert.NoError(t, Validate(nil))
	})

	t.Run("empty map is valid", func(t *testing.T) {
		assert.NoError(t, Validate(map[string]string{}))
	})

	t.Run("empty key is rejected", func(t *testing.T) {
		err := Validate(map[string]string{"": "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not be empty")
	})

	t.Run("key with equals sign is rejected", func(t *testing.T) {
		err := Validate(map[string]string{"key=bad": "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not contain '='")
	})

	t.Run("empty value is rejected", func(t *testing.T) {
		err := Validate(map[string]string{"key": ""})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not be empty")
	})

	t.Run("key at max length is valid", func(t *testing.T) {
		longKey := strings.Repeat("k", 256)
		assert.NoError(t, Validate(map[string]string{longKey: "value"}))
	})

	t.Run("key exceeding max length is rejected", func(t *testing.T) {
		longKey := strings.Repeat("k", 257)
		err := Validate(map[string]string{longKey: "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum length of 256")
	})

	t.Run("value at max length is valid", func(t *testing.T) {
		longVal := strings.Repeat("v", 256)
		assert.NoError(t, Validate(map[string]string{"key": longVal}))
	})

	t.Run("value exceeding max length is rejected", func(t *testing.T) {
		longVal := strings.Repeat("v", 257)
		err := Validate(map[string]string{"key": longVal})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum length of 256")
	})
}
