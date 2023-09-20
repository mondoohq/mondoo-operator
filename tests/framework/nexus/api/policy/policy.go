// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package policy

//go:generate protoc --proto_path=../:../../../../../cnquery:. --go_out=. --go_opt=paths=source_relative --rangerrpc_out=. policy.proto
