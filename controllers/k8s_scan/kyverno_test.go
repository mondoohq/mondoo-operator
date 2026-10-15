// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"k8s.io/utils/ptr"
)

func TestAddKyvernoOptions(t *testing.T) {
	options := map[string]string{}
	err := addKyvernoOptions(options, v1alpha2.KyvernoSpec{
		Enable: ptr.To(true),
		MappingAnnotations: v1alpha2.KyvernoMappingAnnotationsSpec{
			CheckUIDs: []string{"example.com/check"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "true", options[kyvernoDefaultMappings])
	assert.Equal(t, "example.com/check", options[kyvernoMappingAnnotationCheckUIDs])
}

func TestAddKyvernoOptionsRejectsCommaInAnnotationKey(t *testing.T) {
	var configErr invalidKyvernoConfigError
	err := addKyvernoOptions(map[string]string{}, v1alpha2.KyvernoSpec{
		Enable:             ptr.To(true),
		MappingAnnotations: v1alpha2.KyvernoMappingAnnotationsSpec{CheckUIDs: []string{"bad,key"}},
	})
	require.Error(t, err)
	assert.True(t, errors.As(err, &configErr))
	assert.Contains(t, err.Error(), kyvernoMappingAnnotationCheckUIDs)
}

func TestKyvernoDiscoveryTargets(t *testing.T) {
	assert.Equal(t, []string{"pods"}, kyvernoDiscoveryTargets([]string{"pods"}, v1alpha2.KyvernoSpec{}))
	assert.Equal(t, []string{"pods", "kyverno"}, kyvernoDiscoveryTargets([]string{"pods"}, v1alpha2.KyvernoSpec{Enable: ptr.To(true)}))
}
