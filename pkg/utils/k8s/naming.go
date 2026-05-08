// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

const (
	cronJobPrefix = "mondoo-"
	hashLen       = 8
)

// CronJobName builds a CronJob name in the format:
//
//	mondoo-{scanType}                                     (default audit config)
//	mondoo-{scanType}-{integrationID}                     (with integration ID, fits in limit)
//	mondoo-{scanType}-{truncatedID}-{hash}                (with integration ID, truncated)
//
// The hash is always derived from the integration ID alone, so CronJobs of different
// scan types for the same audit config share the same suffix.
func CronJobName(scanType, auditConfigName string) string {
	return cronJobName(scanType, integrationID(auditConfigName), "")
}

// CronJobNameWithCluster builds a CronJob name for external cluster scans:
//
//	mondoo-{scanType}-{clusterName}                                  (default audit config)
//	mondoo-{scanType}-{integrationID}-{clusterName}                  (fits in limit)
//	mondoo-{scanType}-{truncatedID}-{hash}-{clusterName}             (truncated)
func CronJobNameWithCluster(scanType, auditConfigName, clusterName string) string {
	return cronJobName(scanType, integrationID(auditConfigName), clusterName)
}

func cronJobName(scanType, id, clusterName string) string {
	base := cronJobPrefix + scanType

	suffix := id
	if suffix != "" && clusterName != "" {
		suffix += "-" + clusterName
	} else if clusterName != "" {
		suffix = clusterName
	}

	if suffix == "" {
		return base
	}

	full := base + "-" + suffix
	if len(full) <= ResourceNameMaxLength {
		return full
	}

	// Truncate the integration ID, keep cluster name intact when possible
	idHash := fmt.Sprintf("%x", sha256.Sum256([]byte(id)))[:hashLen]
	if clusterName != "" {
		clusterHash := fmt.Sprintf("%x", sha256.Sum256([]byte(clusterName)))[:hashLen]

		// Try keeping full cluster name, truncating only the ID
		// Budget: base + "-" + truncatedID + "-" + hash + "-" + clusterName
		reserved := len(base) + 1 + 1 + hashLen + 1 + len(clusterName)
		available := ResourceNameMaxLength - reserved
		if available > 0 {
			return base + "-" + id[:available] + "-" + idHash + "-" + clusterName
		}

		// Full cluster name doesn't fit — try hash-only ID + full cluster name
		candidate := base + "-" + idHash + "-" + clusterName
		if len(candidate) <= ResourceNameMaxLength {
			return candidate
		}

		// Still too long — truncate cluster name too
		if id == "" {
			// No integration ID: base + "-" + truncatedCluster + "-" + clusterHash
			reserved = len(base) + 1 + 1 + hashLen
			available = ResourceNameMaxLength - reserved
			if available > 0 {
				return base + "-" + clusterName[:available] + "-" + clusterHash
			}
			return base + "-" + clusterHash
		}
		// Both ID and cluster: base + "-" + idHash + "-" + truncatedCluster + "-" + clusterHash
		reserved = len(base) + 1 + hashLen + 1 + 1 + hashLen
		available = ResourceNameMaxLength - reserved
		if available > 0 {
			return base + "-" + idHash + "-" + clusterName[:available] + "-" + clusterHash
		}
		return base + "-" + idHash + "-" + clusterHash
	}

	// Budget: base + "-" + truncatedID + "-" + hash
	reserved := len(base) + 1 + 1 + hashLen
	available := ResourceNameMaxLength - reserved
	if available > 0 {
		return base + "-" + id[:available] + "-" + idHash
	}
	return base + "-" + idHash
}

// integrationID extracts the integration ID from an audit config name.
// "mondoo-client" → "" (default, no integration ID)
// "mondoo-client-abc123" → "abc123"
// "custom-name" → "custom-name" (non-standard name, use as-is)
func integrationID(auditConfigName string) string {
	if auditConfigName == "mondoo-client" {
		return ""
	}
	if strings.HasPrefix(auditConfigName, "mondoo-client-") {
		return strings.TrimPrefix(auditConfigName, "mondoo-client-")
	}
	return auditConfigName
}
