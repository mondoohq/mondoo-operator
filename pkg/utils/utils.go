// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package utils

import (
	"github.com/gobwas/glob"
)

func AllowNamespace(namespace string, includeNamespaces, excludeNamespaces []string) (bool, error) {
	if len(includeNamespaces) > 0 {
		for _, ns := range includeNamespaces {
			g, err := glob.Compile(ns)
			if err != nil {
				return false, err
			}
			if g.Match(namespace) {
				return true, nil
			}
		}
		return false, nil
	}

	for _, ns := range excludeNamespaces {
		g, err := glob.Compile(ns)
		if err != nil {
			return false, nil
		}
		if g.Match(namespace) {
			return false, nil
		}
	}
	return true, nil
}
