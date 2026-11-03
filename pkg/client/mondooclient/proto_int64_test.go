// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondooclient

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtoInt64(t *testing.T) {
	for _, in := range []string{`"300"`, `300`} {
		var p ProtoInt64
		require.NoError(t, json.Unmarshal([]byte(in), &p), in)
		assert.Equal(t, ProtoInt64(300), p, in)
	}

	var p ProtoInt64
	assert.Error(t, json.Unmarshal([]byte(`"5m"`), &p))

	out, err := json.Marshal(ProtoInt64(300))
	require.NoError(t, err)
	assert.JSONEq(t, `"300"`, string(out))
}
