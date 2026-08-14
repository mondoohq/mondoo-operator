// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package utils

import (
	"fmt"
	"strings"

	"github.com/gobwas/glob"
)

// globMetaChars are the characters gobwas/glob interprets as pattern syntax. A
// Kubernetes namespace name is a DNS-1123 label, so it can never contain any of
// them: their presence unambiguously marks an entry as a pattern rather than a
// literal name.
const globMetaChars = `*?[]{}!\`

// IsGlobPattern reports whether s uses glob syntax, i.e. whether it can match a
// name other than itself.
func IsGlobPattern(s string) bool {
	return strings.ContainsAny(s, globMetaChars)
}

// HasGlobPattern reports whether any entry in patterns uses glob syntax.
func HasGlobPattern(patterns []string) bool {
	for _, p := range patterns {
		if IsGlobPattern(p) {
			return true
		}
	}
	return false
}

// NamespaceFilter decides whether a namespace is in scope for watching or
// scanning. Entries may be literal namespace names or glob patterns such as
// "prod-*".
//
// Include takes precedence over Exclude: when the include list is non-empty only
// namespaces matching it are allowed, and the exclude list is not consulted at
// all. When both lists are empty every namespace is allowed.
//
// These are the same semantics the cnspec Kubernetes provider applies to the
// "namespaces" and "namespaces-exclude" inventory options, so a given
// MondooAuditConfig selects the same namespaces whether it is evaluated here or
// inside a scan job.
type NamespaceFilter struct {
	include []glob.Glob
	exclude []glob.Glob
}

// NewNamespaceFilter compiles the include and exclude patterns. It returns an
// error if any pattern is not valid glob syntax, so a typo surfaces at startup
// instead of silently matching nothing.
func NewNamespaceFilter(includeNamespaces, excludeNamespaces []string) (*NamespaceFilter, error) {
	include, err := compileGlobs(includeNamespaces)
	if err != nil {
		return nil, fmt.Errorf("invalid namespace include pattern: %w", err)
	}

	exclude, err := compileGlobs(excludeNamespaces)
	if err != nil {
		return nil, fmt.Errorf("invalid namespace exclude pattern: %w", err)
	}

	return &NamespaceFilter{include: include, exclude: exclude}, nil
}

// Allow reports whether the namespace is in scope.
func (f *NamespaceFilter) Allow(namespace string) bool {
	// Anything on the include list means accept only from that list.
	if len(f.include) > 0 {
		return matchesAny(f.include, namespace)
	}

	// Nothing was explicitly included, so the namespace is in scope unless it is
	// explicitly excluded.
	return !matchesAny(f.exclude, namespace)
}

func compileGlobs(patterns []string) ([]glob.Glob, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	compiled := make([]glob.Glob, 0, len(patterns))
	for _, p := range patterns {
		g, err := glob.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", p, err)
		}
		compiled = append(compiled, g)
	}
	return compiled, nil
}

func matchesAny(globs []glob.Glob, s string) bool {
	for _, g := range globs {
		if g.Match(s) {
			return true
		}
	}
	return false
}

// AllowNamespace reports whether the namespace is in scope for the given include
// and exclude lists. Prefer NewNamespaceFilter when the same lists are evaluated
// repeatedly, since this recompiles the patterns on every call.
func AllowNamespace(namespace string, includeNamespaces, excludeNamespaces []string) (bool, error) {
	f, err := NewNamespaceFilter(includeNamespaces, excludeNamespaces)
	if err != nil {
		return false, err
	}
	return f.Allow(namespace), nil
}
