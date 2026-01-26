// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

const (
	// PullSecretsVolumeName is the volume name for mounted pull secrets
	PullSecretsVolumeName = "pull-secrets"
	// MergedPullSecretsVolumeName is the volume name for the merged config (used with init container)
	MergedPullSecretsVolumeName = "merged-pull-secrets"
	// DockerConfigMountPath is where the Docker config is mounted
	DockerConfigMountPath = "/etc/opt/mondoo/docker"
	// InitContainerName is the name of the init container that merges Docker configs
	InitContainerName = "merge-docker-configs"
)

// AddPrivateRegistrySecretsToSpec adds the necessary volumes, volume mounts, and environment variables
// to a pod spec for handling private registry authentication.
//
// When a single secret is provided, it mounts the secret directly.
// When multiple secrets are provided, it uses an init container to merge the Docker configs.
func AddPrivateRegistrySecretsToSpec(podSpec *corev1.PodSpec, secretNames []string, image string) {
	if len(secretNames) == 0 {
		return
	}

	if len(secretNames) == 1 {
		// Simple case: single secret, mount directly
		addSingleSecretToSpec(podSpec, secretNames[0])
	} else {
		// Multiple secrets: use init container to merge
		addMultipleSecretsToSpec(podSpec, secretNames, image)
	}
}

// addSingleSecretToSpec adds a single private registry secret to the pod spec
func addSingleSecretToSpec(podSpec *corev1.PodSpec, secretName string) {
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: PullSecretsVolumeName,
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources: []corev1.VolumeProjection{
					{
						Secret: &corev1.SecretProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: secretName,
							},
							Items: []corev1.KeyToPath{
								{
									Key:  ".dockerconfigjson",
									Path: "config.json",
								},
							},
						},
					},
				},
				DefaultMode: ptr.To(int32(0o440)),
			},
		},
	})

	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      PullSecretsVolumeName,
		ReadOnly:  true,
		MountPath: DockerConfigMountPath,
	})

	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, corev1.EnvVar{
		Name:  "DOCKER_CONFIG",
		Value: DockerConfigMountPath, // the client automatically adds '/config.json' to the path
	})
}

// addMultipleSecretsToSpec adds multiple private registry secrets to the pod spec using an init container
func addMultipleSecretsToSpec(podSpec *corev1.PodSpec, secretNames []string, image string) {
	// Create volume projections for all secrets
	var projections []corev1.VolumeProjection
	for i, secretName := range secretNames {
		projections = append(projections, corev1.VolumeProjection{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  ".dockerconfigjson",
						Path: fmt.Sprintf("config-%d.json", i),
					},
				},
			},
		})
	}

	// Volume for individual secrets (read by init container)
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: PullSecretsVolumeName,
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources:     projections,
				DefaultMode: ptr.To(int32(0o440)),
			},
		},
	})

	// EmptyDir volume for merged config (written by init container, read by main container)
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: MergedPullSecretsVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	// Init container that merges the configs using pure shell (no jq dependency)
	// The script reads all config-*.json files and merges their .auths objects
	mergeScript := `#!/bin/sh
set -e
cd /secrets

# Initialize merged auths content
merged_auths=""
file_count=0

# Process each config file
for f in config-*.json; do
  if [ -f "$f" ]; then
    file_count=$((file_count + 1))
    # Read file and remove whitespace
    content=$(cat "$f" | tr -d '\n\t\r')
    # Extract the auths object content (the part inside "auths":{...})
    # Pattern: {"auths":{...}} or {"auths":{...},...}
    auths_content=$(echo "$content" | sed 's/.*"auths"[[:space:]]*:[[:space:]]*{\([^}]*\)}.*/\1/')
    if [ -n "$auths_content" ] && [ "$auths_content" != "$content" ]; then
      if [ -n "$merged_auths" ]; then
        merged_auths="${merged_auths},${auths_content}"
      else
        merged_auths="${auths_content}"
      fi
    fi
  fi
done

# Write merged config
echo "{\"auths\":{${merged_auths}}}" > /merged/config.json
echo "Merged Docker configs from ${file_count} files"
`

	initContainer := corev1.Container{
		Name:  InitContainerName,
		Image: image,
		Command: []string{
			"/bin/sh",
			"-c",
			mergeScript,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      PullSecretsVolumeName,
				ReadOnly:  true,
				MountPath: "/secrets",
			},
			{
				Name:      MergedPullSecretsVolumeName,
				MountPath: "/merged",
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			ReadOnlyRootFilesystem:   ptr.To(true),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			RunAsNonRoot: ptr.To(true),
			RunAsUser:    ptr.To(int64(101)),
			Privileged:   ptr.To(false),
		},
	}

	podSpec.InitContainers = append(podSpec.InitContainers, initContainer)

	// Main container mounts the merged config
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      MergedPullSecretsVolumeName,
		ReadOnly:  true,
		MountPath: DockerConfigMountPath,
	})

	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, corev1.EnvVar{
		Name:  "DOCKER_CONFIG",
		Value: DockerConfigMountPath,
	})
}
