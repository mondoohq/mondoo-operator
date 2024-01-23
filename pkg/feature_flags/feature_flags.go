// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package feature_flags

import (
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const FeatureFlagPrefix = "FEATURE_"

var (
	enableAdmissionReviewDiscovery bool
	enableV10                      bool
	allFeatureFlags                = make(map[string]string)
)

func init() {
	envs := os.Environ()
	for _, e := range envs {
		// If it has the feature flag prefix, then parse the env var.
		if strings.HasPrefix(e, FeatureFlagPrefix) {
			val := strings.Split(e, "=")
			allFeatureFlags[val[0]] = val[1]
			setGlobalFlags(val[0], val[1])
		}
	}
}

func AllFeatureFlags() map[string]string {
	return allFeatureFlags
}

func AllFeatureFlagsAsEnv() []corev1.EnvVar {
	var env []corev1.EnvVar
	for k, v := range allFeatureFlags {
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}
	return env
}

func GetAdmissionReviewDiscovery() bool {
	return enableAdmissionReviewDiscovery
}

func GetEnableV10() bool {
	return enableV10
}

func setGlobalFlags(k, v string) {
	if v != "true" && v != "1" {
		return
	}
	switch k {
	case "FEATURE_ENABLE_ADMISSION_REVIEW_DISCOVERY":
		enableAdmissionReviewDiscovery = true
	case "FEATURE_ENABLE_V10":
		enableV10 = true
	}
}
