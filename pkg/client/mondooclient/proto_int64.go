// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondooclient

import (
	"fmt"
	"strconv"
	"strings"
)

// ProtoInt64 decodes an int64 from either a JSON number or a JSON string. The server
// stores the integration config with protojson, which encodes int64 fields as strings.
type ProtoInt64 int64

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
