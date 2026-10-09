// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"hash/fnv"
	"strconv"
	"time"
)

const nodeScanDelayScript = `sleep "$0"; exec "$@"`

// nodeScanStartDelay returns a stable delay for a node within the configured
// spread window. A zero or sub-second window disables the delay.
func nodeScanStartDelay(nodeName string, spread time.Duration) time.Duration {
	maxSeconds := int64(spread / time.Second)
	if nodeName == "" || maxSeconds <= 0 {
		return 0
	}

	hash := fnv.New32a()
	_, _ = hash.Write([]byte(nodeName))
	return time.Duration(int64(hash.Sum32())%(maxSeconds+1)) * time.Second
}

// nodeScanCommand wraps a node scan command with a startup delay while keeping
// the actual cnspec arguments as separate shell arguments.
func nodeScanCommand(command []string, delay time.Duration) ([]string, []string) {
	if delay <= 0 {
		return command, nil
	}

	args := make([]string, 1, len(command)+1)
	args[0] = strconv.FormatInt(int64(delay/time.Second), 10)
	args = append(args, command...)
	return []string{"/bin/sh", "-c", nodeScanDelayScript}, args
}
