// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestAddPrivateRegistrySecretsToSpec_NoSecrets(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	AddPrivateRegistrySecretsToSpec(podSpec, nil, "test-image:latest")

	assert.Empty(t, podSpec.Volumes)
	assert.Empty(t, podSpec.InitContainers)
	assert.Empty(t, podSpec.Containers[0].VolumeMounts)
}

func TestAddPrivateRegistrySecretsToSpec_SingleSecret(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	AddPrivateRegistrySecretsToSpec(podSpec, []string{"my-secret"}, "test-image:latest")

	// Should have 1 volume (pull-secrets)
	require.Len(t, podSpec.Volumes, 1)
	assert.Equal(t, PullSecretsVolumeName, podSpec.Volumes[0].Name)

	// Should NOT have an init container for single secret
	assert.Empty(t, podSpec.InitContainers)

	// Main container should have volume mount
	require.Len(t, podSpec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, PullSecretsVolumeName, podSpec.Containers[0].VolumeMounts[0].Name)
	assert.Equal(t, DockerConfigMountPath, podSpec.Containers[0].VolumeMounts[0].MountPath)

	// Should have DOCKER_CONFIG env var
	require.Len(t, podSpec.Containers[0].Env, 1)
	assert.Equal(t, "DOCKER_CONFIG", podSpec.Containers[0].Env[0].Name)
	assert.Equal(t, DockerConfigMountPath, podSpec.Containers[0].Env[0].Value)

	// Verify the projected volume has the correct secret
	projected := podSpec.Volumes[0].VolumeSource.Projected
	require.NotNil(t, projected)
	require.Len(t, projected.Sources, 1)
	assert.Equal(t, "my-secret", projected.Sources[0].Secret.LocalObjectReference.Name)
}

func TestAddPrivateRegistrySecretsToSpec_MultipleSecrets(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	secrets := []string{"secret-1", "secret-2", "secret-3"}
	AddPrivateRegistrySecretsToSpec(podSpec, secrets, "test-image:latest")

	// Should have 2 volumes (pull-secrets for individual secrets, merged-pull-secrets for merged config)
	require.Len(t, podSpec.Volumes, 2)

	var pullSecretsVolume, mergedVolume *corev1.Volume
	for i := range podSpec.Volumes {
		switch podSpec.Volumes[i].Name {
		case PullSecretsVolumeName:
			pullSecretsVolume = &podSpec.Volumes[i]
		case MergedPullSecretsVolumeName:
			mergedVolume = &podSpec.Volumes[i]
		}
	}
	require.NotNil(t, pullSecretsVolume, "pull-secrets volume should exist")
	require.NotNil(t, mergedVolume, "merged-pull-secrets volume should exist")

	// Pull secrets volume should be projected with all 3 secrets
	projected := pullSecretsVolume.VolumeSource.Projected
	require.NotNil(t, projected)
	require.Len(t, projected.Sources, 3)
	for i, secret := range secrets {
		assert.Equal(t, secret, projected.Sources[i].Secret.LocalObjectReference.Name)
	}

	// Merged volume should be an emptyDir
	require.NotNil(t, mergedVolume.VolumeSource.EmptyDir)

	// Should have an init container
	require.Len(t, podSpec.InitContainers, 1)
	initContainer := podSpec.InitContainers[0]
	assert.Equal(t, InitContainerName, initContainer.Name)
	assert.Equal(t, "test-image:latest", initContainer.Image)

	// Init container should mount both volumes
	require.Len(t, initContainer.VolumeMounts, 2)

	// Main container should mount the merged volume
	require.Len(t, podSpec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, MergedPullSecretsVolumeName, podSpec.Containers[0].VolumeMounts[0].Name)
	assert.Equal(t, DockerConfigMountPath, podSpec.Containers[0].VolumeMounts[0].MountPath)

	// Should have DOCKER_CONFIG env var
	require.Len(t, podSpec.Containers[0].Env, 1)
	assert.Equal(t, "DOCKER_CONFIG", podSpec.Containers[0].Env[0].Name)
}

func TestAddPrivateRegistrySecretsToSpec_PreservesExistingSpec(t *testing.T) {
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

	AddPrivateRegistrySecretsToSpec(podSpec, []string{"my-secret"}, "test-image:latest")

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
