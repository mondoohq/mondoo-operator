/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package utils

func AllowNamespace(namespace string, includeNamespaces, excludeNamespaces []string) bool {
	if len(includeNamespaces) > 0 {
		for _, ns := range includeNamespaces {
			if ns == namespace {
				return true
			}
		}
		return false
	}

	for _, ns := range excludeNamespaces {
		if ns == namespace {
			return false
		}
	}
	return true
}
