// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package utils

import (
	"fmt"
	"strings"
)

func LabelsToLabelSelector(ls map[string]string) string {
	var selector []string
	for k, v := range ls {
		selector = append(selector, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(selector, ",")
}
