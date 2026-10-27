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

// Name joins prefix and suffix into an object name that fits ResourceNameMaxLength.
//
// A name that fits is returned unchanged, so every object that exists today keeps
// its name. When it does not fit, the prefix (the user's part: a MondooAuditConfig
// or Secret name) is cut to make room and a short hash of the whole prefix is
// appended to the cut, so two long prefixes that differ only past the cut still
// name different objects; the suffix (the operator's own, short) stays whole at
// the end, where it says what the object is:
//
//	{prefix}{suffix}                     (fits)
//	{cutPrefix}-{prefixHash}{suffix}     (cut)
func Name(prefix, suffix string) string {
	if full := prefix + suffix; len(full) <= ResourceNameMaxLength {
		return full
	}
	prefixHash := fmt.Sprintf("%x", sha256.Sum256([]byte(prefix)))[:hashLen]
	room := ResourceNameMaxLength - len(suffix) - 1 - hashLen
	if room <= 0 {
		// The suffix alone leaves no room for any of the prefix.
		return trimName(prefixHash + suffix)
	}
	return strings.TrimRight(prefix[:room], "-") + "-" + prefixHash + suffix
}

// ScopedName builds {prefix}{infix}-{scope} for an object that belongs to one
// scope of a MondooAuditConfig (an external cluster), within ResourceNameMaxLength.
// The scope is user input too. A name that fits is returned unchanged. Otherwise
// the prefix is cut first (as in Name) and the scope kept whole; when the scope
// itself leaves no room, it is replaced by its hash and the prefix cut as needed:
//
//	{prefix}{infix}-{scope}                           (fits)
//	{cutPrefix}-{prefixHash}{infix}-{scope}           (prefix cut)
//	{prefix or cut}{infix}-{scopeHash}                (scope hashed)
func ScopedName(prefix, infix, scope string) string {
	if full := prefix + infix + "-" + scope; len(full) <= ResourceNameMaxLength {
		return full
	}
	tail := infix + "-" + scope
	if room := ResourceNameMaxLength - len(tail) - 1 - hashLen; room > 0 {
		return Name(prefix, tail)
	}
	scopeHash := fmt.Sprintf("%x", sha256.Sum256([]byte(scope)))[:hashLen]
	return Name(prefix, infix+"-"+scopeHash)
}

// trimName cuts s to ResourceNameMaxLength without leaving a trailing dash.
func trimName(s string) string {
	if len(s) > ResourceNameMaxLength {
		s = s[:ResourceNameMaxLength]
	}
	return strings.TrimRight(s, "-")
}

// integrationID extracts the integration ID from an audit config name.
// "mondoo-client" → "" (default, no integration ID)
// "mondoo-client-abc123" → "abc123"
// "custom-name" → "custom-name" (non-standard name, use as-is)
func integrationID(auditConfigName string) string {
	if auditConfigName == "mondoo-client" {
		return ""
	}
	if id, ok := strings.CutPrefix(auditConfigName, "mondoo-client-"); ok && id != "" {
		return id
	}
	return auditConfigName
}
