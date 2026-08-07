// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"bytes"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeScanStartDelay_DeterministicMultiNodeExamples(t *testing.T) {
	spread := 30 * time.Minute
	tests := map[string]time.Duration{
		"node-a":                            1084 * time.Second,
		"node-b":                            587 * time.Second,
		"node-c":                            90 * time.Second,
		"ip-10-0-1-1":                       1143 * time.Second,
		"ip-10-0-1-2":                       646 * time.Second,
		"worker-0":                          1296 * time.Second,
		"worker-1":                          799 * time.Second,
		"aks-nodepool1-12345678-vmss000001": 79 * time.Second,
	}

	for nodeName, expectedDelay := range tests {
		assert.Equal(t, expectedDelay, nodeScanStartDelay(nodeName, spread), nodeName)
		assert.Equal(t, expectedDelay, nodeScanStartDelay(nodeName, spread), "repeat check: %s", nodeName)
	}
}

func TestNodeScanStartDelay_FlattensClusterWidePeak(t *testing.T) {
	const totalNodes = 500
	spread := 30 * time.Minute

	concurrentPeak := totalNodes
	bucketCounts := map[time.Duration]int{}
	for i := 0; i < totalNodes; i++ {
		nodeName := fmt.Sprintf("node-%03d", i)
		delay := nodeScanStartDelay(nodeName, spread)
		bucketCounts[delay]++
	}

	staggeredPeak := 0
	for _, count := range bucketCounts {
		if count > staggeredPeak {
			staggeredPeak = count
		}
	}

	t.Logf("cluster-wide startup peak (500 nodes): concurrent=%d, staggered=%d, spread=%s, populated-buckets=%d",
		concurrentPeak, staggeredPeak, spread, len(bucketCounts))

	require.Greater(t, len(bucketCounts), 1)
	assert.Less(t, staggeredPeak, concurrentPeak)
	assert.LessOrEqual(t, staggeredPeak, 5)
}

func TestNodeScanDelayScript_PreservesArgumentBoundaries(t *testing.T) {
	proxyValue := "https://proxy.example:8443; echo injected"
	cmd := exec.Command("/bin/sh", "-c", nodeScanDelayScript, "0", "/bin/echo", proxyValue)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	require.NoError(t, cmd.Run(), out.String())
	assert.Equal(t, proxyValue+"\n", out.String())
}
