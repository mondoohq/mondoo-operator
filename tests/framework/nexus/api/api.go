// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package api

//go:generate protoc --proto_path=../:. --go_out=. --go_opt=paths=source_relative --rangerrpc_out=. pagination.proto
