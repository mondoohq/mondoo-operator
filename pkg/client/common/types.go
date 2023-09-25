// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package common

import "context"

const HealthCheckEndpoint = "/Health/Check"

type HealthCheckRequest struct{}

type HealthCheckResponse struct {
	Status string `json:"status,omitempty"`
	// returns rfc 3339 timestamp
	Time string `json:"time,omitempty"`
	// returns the major api version
	ApiVersion string `json:"apiVersion,omitempty"`
	// returns the git commit checksum
	Build string `json:"build,omitempty"`
}

type HealthCheckClient interface {
	HealthCheck(context.Context, *HealthCheckRequest) (*HealthCheckResponse, error)
}
