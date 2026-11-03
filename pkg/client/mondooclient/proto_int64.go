// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondooclient

import (
	"fmt"
	"strconv"
	"strings"
)

// ProtoInt64 is an int64 that the remote config encodes as a JSON string. It decodes
// from either a JSON string or a JSON number and encodes as a JSON string.
type ProtoInt64 int64

func (p ProtoInt64) MarshalJSON() ([]byte, error) {
	return []byte(`"` + strconv.FormatInt(int64(p), 10) + `"`), nil
}

func (p *ProtoInt64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*p = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid int64 %s: %w", string(b), err)
	}
	*p = ProtoInt64(v)
	return nil
}
