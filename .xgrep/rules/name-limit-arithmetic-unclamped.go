// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

//go:build ignore

// xgrep test fixture, not compiled. Run: xgrep test .xgrep/rules
package nodes

func nodeNameOrHashOld(prefix, infix, nodeName string) string {
	base := prefix + infix
	// ruleid: mondoo-name-limit-arithmetic-unclamped
	return base + NodeNameOrHash(k8s.ResourceNameMaxLength-len(base), nodeName)
}

func twoSubtractions(prefix, infix, nodeName string) string {
	// ruleid: mondoo-name-limit-arithmetic-unclamped
	return prefix + NodeNameOrHash(k8s.ResourceNameMaxLength-len(infix)-minNodeHashLen, nodeName)
}

func insidePackageK8s(id string) string {
	// ruleid: mondoo-name-limit-arithmetic-unclamped
	return truncateTo(ResourceNameMaxLength-len(id), id)
}

func guarded(prefix, infix, nodeName string) string {
	base := prefix + infix
	allowedLen := k8s.ResourceNameMaxLength - len(base)
	if len(nodeName) <= allowedLen {
		return base + nodeName
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(nodeName)))
	// ok: mondoo-name-limit-arithmetic-unclamped
	return base + hash[:min(allowedLen, len(hash))]
}

// known gap (metadata.known-gaps): a slice bound built from the limit matches no AST pattern in
// xgrep 0.84, so this shape is not annotated; the call form above is the one #1651 had
func slicedByLimit(prefix, infix string) string {
	return prefix[:k8s.ResourceNameMaxLength-len(infix)] + infix
}