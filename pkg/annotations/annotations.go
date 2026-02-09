// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package annotations

import (
	"fmt"
	"sort"
	"strings"
)

// AnnotationArgs converts a map of annotations into sorted CLI arguments
// suitable for passing to cnspec via --annotation key=value flags.
func AnnotationArgs(annotations map[string]string) []string {
	if len(annotations) == 0 {
		return nil
	}

	keys := make([]string, 0, len(annotations))
	for k := range annotations {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	args := make([]string, 0, len(annotations)*2)
	for _, key := range keys {
		args = append(args, "--annotation", fmt.Sprintf("%s=%s", key, annotations[key]))
	}
	return args
}

const maxAnnotationLength = 256

// Validate checks that annotation keys and values are well-formed for use as
// cnspec --annotation key=value CLI arguments. Keys must be non-empty, must
// not contain '=', and both keys and values must not exceed 256 characters.
// Values must be non-empty.
func Validate(annotations map[string]string) error {
	for k, v := range annotations {
		if k == "" {
			return fmt.Errorf("annotation key must not be empty")
		}
		if strings.Contains(k, "=") {
			return fmt.Errorf("annotation key %q must not contain '='", k)
		}
		if len(k) > maxAnnotationLength {
			return fmt.Errorf("annotation key %q exceeds maximum length of %d characters", k, maxAnnotationLength)
		}
		if v == "" {
			return fmt.Errorf("annotation value for key %q must not be empty", k)
		}
		if len(v) > maxAnnotationLength {
			return fmt.Errorf("annotation value for key %q exceeds maximum length of %d characters", k, maxAnnotationLength)
		}
	}
	return nil
}
