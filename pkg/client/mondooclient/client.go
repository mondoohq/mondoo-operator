// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondooclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.mondoo.com/mondoo-operator/pkg/client/common"
)

const (
	ExchangeRegistrationTokenEndpoint = "/AgentManager/ExchangeRegistrationToken"
	IntegrationRegisterEndpoint       = "/IntegrationsManager/Register"
	IntegrationCheckInEndpoint        = "/IntegrationsManager/CheckIn"
	IntegrationReportStatusEndpoint   = "/IntegrationsManager/ReportStatus"
)

type MondooClientOptions struct {
	ApiEndpoint string
	Token       string
	HttpProxy   *string
	HttpTimeout *time.Duration
}

type mondooClient struct {
	ApiEndpoint string
	Token       string
	httpClient  http.Client
}

func NewClient(opts MondooClientOptions) (MondooClient, error) {
	opts.ApiEndpoint = strings.TrimRight(opts.ApiEndpoint, "/")
	client, err := common.DefaultHttpClient(opts.HttpProxy, opts.HttpTimeout)
	if err != nil {
		return nil, err
	}
	mClient := &mondooClient{
		ApiEndpoint: opts.ApiEndpoint,
		Token:       opts.Token,
		httpClient:  client,
	}
	return mClient, nil
}

func (s *mondooClient) ExchangeRegistrationToken(ctx context.Context, in *ExchangeRegistrationTokenInput) (*ExchangeRegistrationTokenOutput, error) {
	url := s.ApiEndpoint + ExchangeRegistrationTokenEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := common.Request(ctx, s.httpClient, url, s.Token, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &ExchangeRegistrationTokenOutput{
		ServiceAccount: string(respBodyBytes),
	}

	return out, nil
}

func (s *mondooClient) HealthCheck(ctx context.Context, in *common.HealthCheckRequest) (*common.HealthCheckResponse, error) {
	url := s.ApiEndpoint + common.HealthCheckEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := common.Request(ctx, s.httpClient, url, s.Token, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &common.HealthCheckResponse{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proto response: %v", err)
	}

	return out, nil
}

func (s *mondooClient) IntegrationRegister(ctx context.Context, in *IntegrationRegisterInput) (*IntegrationRegisterOutput, error) {
	url := s.ApiEndpoint + IntegrationRegisterEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := common.Request(ctx, s.httpClient, url, s.Token, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &IntegrationRegisterOutput{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proto response: %v", err)
	}

	return out, nil
}

func (s *mondooClient) IntegrationCheckIn(ctx context.Context, in *IntegrationCheckInInput) (*IntegrationCheckInOutput, error) {
	url := s.ApiEndpoint + IntegrationCheckInEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := common.Request(ctx, s.httpClient, url, s.Token, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &IntegrationCheckInOutput{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return out, nil
}

func (s *mondooClient) IntegrationReportStatus(ctx context.Context, in *ReportStatusRequest) error {
	url := s.ApiEndpoint + IntegrationReportStatusEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	_, err = common.Request(ctx, s.httpClient, url, s.Token, reqBodyBytes)
	if err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}

	return nil
}
