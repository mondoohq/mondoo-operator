// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/annotations"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/feature_flags"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

const (
	DeploymentNameSuffix       = "-resource-watcher"
	defaultDebounceInterval    = 10 * time.Second
	defaultMinimumScanInterval = 2 * time.Minute
)

// DeploymentName returns the name of the resource watcher deployment for a given MondooAuditConfig.
func DeploymentName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, DeploymentNameSuffix)
}

// DeploymentLabels returns the labels for the resource watcher deployment.
func DeploymentLabels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo-resource-watcher",
		"mondoo_cr": m.Name,
	}
}

// Deployment creates a Deployment spec for the resource watcher.
func Deployment(image, integrationMRN, clusterUID string, m *v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) *appsv1.Deployment {
	ls := DeploymentLabels(*m)

	// Build command arguments
	cmd := []string{
		"/mondoo-operator", "resource-watcher",
		"--config", "/etc/opt/mondoo/config/mondoo.yml",
	}

	// Add cluster UID for asset labeling
	if clusterUID != "" {
		cmd = append(cmd, "--cluster-uid", clusterUID)
	}

	// Add integration MRN for asset labeling
	if integrationMRN != "" {
		cmd = append(cmd, "--integration-mrn", integrationMRN)
	}

	// Add debounce interval if configured
	debounceInterval := defaultDebounceInterval
	if m.Spec.KubernetesResources.ResourceWatcher.DebounceInterval.Duration > 0 {
		debounceInterval = m.Spec.KubernetesResources.ResourceWatcher.DebounceInterval.Duration
	}
	cmd = append(cmd, "--debounce-interval", debounceInterval.String())

	// Add minimum scan interval if configured
	minimumScanInterval := defaultMinimumScanInterval
	if m.Spec.KubernetesResources.ResourceWatcher.MinimumScanInterval.Duration > 0 {
		minimumScanInterval = m.Spec.KubernetesResources.ResourceWatcher.MinimumScanInterval.Duration
	}
	cmd = append(cmd, "--minimum-scan-interval", minimumScanInterval.String())

	// Add watch all resources flag if enabled
	if m.Spec.KubernetesResources.ResourceWatcher.WatchAllResources {
		cmd = append(cmd, "--watch-all-resources")
	}

	// Add resource types if configured (overrides watch-all-resources)
	if len(m.Spec.KubernetesResources.ResourceWatcher.ResourceTypes) > 0 {
		cmd = append(cmd, "--resource-types", strings.Join(m.Spec.KubernetesResources.ResourceWatcher.ResourceTypes, ","))
	}

	// Add namespace filtering
	if len(m.Spec.Filtering.Namespaces.Include) > 0 {
		cmd = append(cmd, "--namespaces", strings.Join(m.Spec.Filtering.Namespaces.Include, ","))
	}
	if len(m.Spec.Filtering.Namespaces.Exclude) > 0 {
		cmd = append(cmd, "--namespaces-exclude", strings.Join(m.Spec.Filtering.Namespaces.Exclude, ","))
	}

	// Add API proxy if configured
	if cfg.Spec.HttpProxy != nil {
		cmd = append(cmd, "--api-proxy", *cfg.Spec.HttpProxy)
	}

	// Add annotations (sorted for deterministic ordering)
	cmd = append(cmd, annotations.AnnotationArgs(m.Spec.Annotations)...)

	envVars := feature_flags.AllFeatureFlagsAsEnv()
	envVars = append(envVars, corev1.EnvVar{Name: "MONDOO_AUTO_UPDATE", Value: "false"})

	// Add custom scanner env vars
	envVars = append(envVars, m.Spec.Scanner.Env...)

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      DeploymentName(m.Name),
			Namespace: m.Namespace,
			Labels:    ls,
		},
		Spec: appsv1.DeploymentSpec{
			// Resource watcher should only have one replica to avoid duplicate scanning
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "mondoo-resource-watcher",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         cmd,
							Resources:       k8s.ResourcesRequirementsWithDefaults(m.Spec.Scanner.Resources, k8s.DefaultK8sResourceScanningResources),
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
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									ReadOnly:  true,
									MountPath: "/etc/opt/mondoo/config",
								},
								{
									Name:      "temp",
									MountPath: "/tmp",
								},
							},
							Env: envVars,
						},
					},
					ServiceAccountName: m.Spec.Scanner.ServiceAccountName,
					Volumes: []corev1.Volume{
						{
							Name: "temp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									DefaultMode: ptr.To(int32(corev1.ProjectedVolumeSourceDefaultMode)),
									Sources: []corev1.VolumeProjection{
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: m.Spec.MondooCredsSecretRef,
												Items: []corev1.KeyToPath{{
													Key:  constants.MondooCredsSecretServiceAccountKey,
													Path: "mondoo.yml",
												}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return deployment
}
