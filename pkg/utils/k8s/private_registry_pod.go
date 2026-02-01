// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

const (
	// PullSecretsVolumeName is the volume name for mounted pull secrets
	PullSecretsVolumeName = "pull-secrets"
	// DockerConfigMountPath is where the Docker config is mounted
	DockerConfigMountPath = "/etc/opt/mondoo/docker"
)

// AddPrivateRegistryPullSecretToSpec adds the necessary volumes, volume mounts, and environment variables
// to a pod spec for handling private registry authentication using a single secret.
func AddPrivateRegistryPullSecretToSpec(podSpec *corev1.PodSpec, secretName string) {
	if secretName == "" {
		return
	}

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
