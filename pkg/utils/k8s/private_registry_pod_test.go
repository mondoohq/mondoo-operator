// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestAddPrivateRegistryPullSecretToSpec_NoSecret(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	AddPrivateRegistryPullSecretToSpec(podSpec, "")

	assert.Empty(t, podSpec.Volumes)
	assert.Empty(t, podSpec.Containers[0].VolumeMounts)
	assert.Empty(t, podSpec.Containers[0].Env)
}

func TestAddPrivateRegistryPullSecretToSpec_WithSecret(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	AddPrivateRegistryPullSecretToSpec(podSpec, "my-secret")

	// Should have 1 volume (pull-secrets)
	require.Len(t, podSpec.Volumes, 1)
	assert.Equal(t, PullSecretsVolumeName, podSpec.Volumes[0].Name)

	// Main container should have volume mount
	require.Len(t, podSpec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, PullSecretsVolumeName, podSpec.Containers[0].VolumeMounts[0].Name)
	assert.Equal(t, DockerConfigMountPath, podSpec.Containers[0].VolumeMounts[0].MountPath)

	// Should have DOCKER_CONFIG env var
	require.Len(t, podSpec.Containers[0].Env, 1)
	assert.Equal(t, "DOCKER_CONFIG", podSpec.Containers[0].Env[0].Name)
	assert.Equal(t, DockerConfigMountPath, podSpec.Containers[0].Env[0].Value)

	// Verify the projected volume has the correct secret
	projected := podSpec.Volumes[0].Projected
	require.NotNil(t, projected)
	require.Len(t, projected.Sources, 1)
	assert.Equal(t, "my-secret", projected.Sources[0].Secret.Name)
}

func TestAddPrivateRegistryPullSecretToSpec_PreservesExistingSpec(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name: "main",
				VolumeMounts: []corev1.VolumeMount{
					{Name: "existing-mount", MountPath: "/existing"},
				},
				Env: []corev1.EnvVar{
					{Name: "EXISTING_VAR", Value: "value"},
				},
			},
		},
		Volumes: []corev1.Volume{
			{Name: "existing-volume"},
		},
	}

	AddPrivateRegistryPullSecretToSpec(podSpec, "my-secret")

	// Should preserve existing volume and add new one
	assert.Len(t, podSpec.Volumes, 2)
	assert.Equal(t, "existing-volume", podSpec.Volumes[0].Name)

	// Should preserve existing volume mount and add new one
	assert.Len(t, podSpec.Containers[0].VolumeMounts, 2)
	assert.Equal(t, "existing-mount", podSpec.Containers[0].VolumeMounts[0].Name)

	// Should preserve existing env var and add new one
	assert.Len(t, podSpec.Containers[0].Env, 2)
	assert.Equal(t, "EXISTING_VAR", podSpec.Containers[0].Env[0].Name)
}
