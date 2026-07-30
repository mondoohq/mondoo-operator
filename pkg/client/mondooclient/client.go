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
	IntegrationConfigureEndpoint      = "/IntegrationsManager/Configure"
	IntegrationReportStatusEndpoint   = "/IntegrationsManager/ReportStatus"
	IntegrationCreateEndpoint         = "/IntegrationsManager/Create"
	IntegrationListEndpoint           = "/IntegrationsManager/List"
	IntegrationGetTokenEndpoint       = "/IntegrationsManager/GetTokenForIntegration"
	IntegrationDeleteEndpoint         = "/IntegrationsManager/Delete"
	GarbageCollectAssetsEndpoint      = "/PolicyResolver/PurgeAssets"
	RefreshAssetScoresEndpoint        = "/PolicyResolver/RefreshAssetScores"
)

type MondooClientOptions struct {
	ApiEndpoint string
	Token       string
	HttpProxy   *string
	HttpsProxy  *string
	NoProxy     *string
	HttpTimeout *time.Duration
}

type mondooClient struct {
	ApiEndpoint string
	Token       string
	httpClient  http.Client
}

func NewClient(opts MondooClientOptions) (MondooClient, error) {
	opts.ApiEndpoint = strings.TrimRight(opts.ApiEndpoint, "/")
	client, err := common.DefaultHttpClientWithProxy(opts.HttpProxy, opts.HttpsProxy, opts.NoProxy, opts.HttpTimeout)
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
		return nil, fmt.Errorf("failed to parse response: %w", err)
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
		return nil, fmt.Errorf("failed to parse response: %w", err)
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
		return nil, fmt.Errorf("failed to parse response: %w", err)
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
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	out := &IntegrationCheckInOutput{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return out, nil
}

func (s *mondooClient) IntegrationConfigure(ctx context.Context, in *IntegrationConfigureInput) (*IntegrationConfigureOutput, error) {
	url := s.ApiEndpoint + IntegrationConfigureEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := common.Request(ctx, s.httpClient, url, s.Token, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	out := &IntegrationConfigureOutput{}
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
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return nil
}

func (s *mondooClient) IntegrationCreate(ctx context.Context, in *IntegrationCreateInput) (*IntegrationCreateOutput, error) {
	url := s.ApiEndpoint + IntegrationCreateEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := common.Request(ctx, s.httpClient, url, s.Token, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	out := &IntegrationCreateOutput{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return out, nil
}

func (s *mondooClient) IntegrationList(ctx context.Context, in *IntegrationListInput) (*IntegrationListOutput, error) {
	url := s.ApiEndpoint + IntegrationListEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := common.Request(ctx, s.httpClient, url, s.Token, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	out := &IntegrationListOutput{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return out, nil
}

func (s *mondooClient) IntegrationGetToken(ctx context.Context, in *IntegrationGetTokenInput) (*IntegrationGetTokenOutput, error) {
	url := s.ApiEndpoint + IntegrationGetTokenEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := common.Request(ctx, s.httpClient, url, s.Token, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	out := &IntegrationGetTokenOutput{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return out, nil
}

func (s *mondooClient) IntegrationDelete(ctx context.Context, in *IntegrationDeleteInput) error {
	url := s.ApiEndpoint + IntegrationDeleteEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	_, err = common.Request(ctx, s.httpClient, url, s.Token, reqBodyBytes)
	if err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return nil
}

func (s *mondooClient) GarbageCollectAssets(ctx context.Context, req *GarbageCollectAssetsRequest) error {
	url := s.ApiEndpoint + GarbageCollectAssetsEndpoint

	reqBodyBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	_, err = common.Request(ctx, s.httpClient, url, s.Token, reqBodyBytes)
	if err != nil {
		return fmt.Errorf("failed to make garbage collect assets request: %v", err)
	}

	return nil
}

func (s *mondooClient) RefreshAssetScores(ctx context.Context, req *RefreshAssetScoresRequest) (*RefreshAssetScoresResponse, error) {
	url := s.ApiEndpoint + RefreshAssetScoresEndpoint

	reqBodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := common.Request(ctx, s.httpClient, url, s.Token, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to make refresh asset scores request: %v", err)
	}

	out := &RefreshAssetScoresResponse{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return out, nil
}
