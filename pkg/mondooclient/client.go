/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package mondooclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go.mondoo.com/cnquery/motor/asset"
	v1 "go.mondoo.com/cnquery/motor/inventory/v1"
	"go.mondoo.com/cnquery/motor/providers"
	"go.mondoo.com/cnspec/policy/scan"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	defaultHttpTimeout         = 30 * time.Second
	defaultIdleConnTimeout     = 30 * time.Second
	defaultKeepAlive           = 30 * time.Second
	defaultTLSHandshakeTimeout = 10 * time.Second
	maxIdleConnections         = 100
)

//go:generate ./../../bin/mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

type Client interface {
	ExchangeRegistrationToken(context.Context, *ExchangeRegistrationTokenInput) (*ExchangeRegistrationTokenOutput, error)

	HealthCheck(context.Context, *HealthCheckRequest) (*HealthCheckResponse, error)
	RunAdmissionReview(context.Context, *AdmissionReviewJob) (*ScanResult, error)
	ScanKubernetesResources(ctx context.Context, scanOpts *ScanKubernetesResourcesOpts) (*ScanResult, error)
	ScheduleKubernetesResourceScan(ctx context.Context, integrationMrn, resourceKey, managedBy string) (*Empty, error)
	GarbageCollectAssets(context.Context, *scan.GarbageCollectOptions) error

	IntegrationRegister(context.Context, *IntegrationRegisterInput) (*IntegrationRegisterOutput, error)
	IntegrationCheckIn(context.Context, *IntegrationCheckInInput) (*IntegrationCheckInOutput, error)
	IntegrationReportStatus(context.Context, *ReportStatusRequest) error
}

func DefaultHttpClient() *http.Client {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   defaultHttpTimeout,
			KeepAlive: defaultKeepAlive,
		}).DialContext,
		MaxIdleConns:          maxIdleConnections,
		IdleConnTimeout:       defaultIdleConnTimeout,
		TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{
		Transport: tr,
		Timeout:   defaultHttpTimeout,
	}
	return httpClient
}

type ClientOptions struct {
	ApiEndpoint string
	Token       string
}

type mondooClient struct {
	ApiEndpoint string
	Token       string
	httpclient  http.Client
}

func (s *mondooClient) request(ctx context.Context, url string, reqBodyBytes []byte) ([]byte, error) {
	client := s.httpclient

	header := make(http.Header)
	header.Set("Accept", "application/json")
	header.Set("Content-Type", "application/json")
	if s.Token != "" {
		header.Set("Authorization", fmt.Sprintf("Bearer %s", s.Token))
	}

	reader := bytes.NewReader(reqBodyBytes)
	req, err := http.NewRequest(http.MethodPost, url, reader)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.Header = header

	// do http call
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %v", err)
	}

	defer func() {
		resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read http response body: %s", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode, respBody)
	}

	return respBody, nil
}

const ExchangeRegistrationTokenEndpoint = "/AgentManager/ExchangeRegistrationToken"

func (s *mondooClient) ExchangeRegistrationToken(ctx context.Context, in *ExchangeRegistrationTokenInput) (*ExchangeRegistrationTokenOutput, error) {
	url := s.ApiEndpoint + ExchangeRegistrationTokenEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := s.request(ctx, url, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &ExchangeRegistrationTokenOutput{
		ServiceAccount: string(respBodyBytes),
	}

	return out, nil
}

// ExchangeRegistrationTokenInput is used for converting a JWT to a Mondoo serivce account
type ExchangeRegistrationTokenInput struct {
	// JWT token, only available during creation
	Token string `protobuf:"bytes,1,opt,name=token,proto3" json:"token,omitempty"`
}

type ExchangeRegistrationTokenOutput struct {
	ServiceAccount string `json:"serviceAccount,omitempty"`
}

const HealthCheckEndpoint = "/Health/Check"

func (s *mondooClient) HealthCheck(ctx context.Context, in *HealthCheckRequest) (*HealthCheckResponse, error) {
	url := s.ApiEndpoint + HealthCheckEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := s.request(ctx, url, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &HealthCheckResponse{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proto response: %v", err)
	}

	return out, nil
}

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

const (
	RunAdmissionReviewEndpoint = "/Scan/RunAdmissionReview"
	// A valid result would come back as a '2'
	ValidScanResult = uint32(2)
)

func (s *mondooClient) RunAdmissionReview(ctx context.Context, in *AdmissionReviewJob) (*ScanResult, error) {
	url := s.ApiEndpoint + RunAdmissionReviewEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := s.request(ctx, url, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &ScanResult{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proto response: %v", err)
	}

	return out, nil
}

type AdmissionReviewJob struct {
	Data *structpb.Struct `json:"data,omitempty"`
	// Map of string keys and values that can be used to organize and categorize the assets
	Labels     map[string]string `json:"labels,omitempty"`
	ReportType ReportType        `json:"report_type,omitempty"`
	// Additional options for the manifest job
	Options map[string]string `json:"options,omitempty"`
	// Additional discovery settings for the manifest job
	Discovery *providers.Discovery `json:"discovery,omitempty"`
}

type File struct {
	Data []byte `json:"data,omitempty"`
}

type ScanResult struct {
	WorstScore *Score `json:"worstScore,omitempty"`
	Ok         bool   `json:"ok,omitempty"`
}

type Score struct {
	QrId            string `json:"qr_id,omitempty"`
	Type            uint32 `json:"type,omitempty"`
	Value           uint32 `json:"value,omitempty"`
	Weight          uint32 `json:"weight,omitempty"`
	ScoreCompletion uint32 `json:"score_completion,omitempty"`
	DataTotal       uint32 `json:"data_total,omitempty"`
	DataCompletion  uint32 `json:"data_completion,omitempty"`
	Message         string `json:"message,omitempty"`
}

type ScanKubernetesResourcesOpts struct {
	IntegrationMrn string
	// If set to true, the scan will discover only container images and not Kubernetes resources
	ScanContainerImages bool
	ManagedBy           string
	IncludeNamespaces   []string
	ExcludeNamespaces   []string
}

const ScanKubernetesResourcesEndpoint = "/Scan/Run"

func (s *mondooClient) ScanKubernetesResources(ctx context.Context, scanOpts *ScanKubernetesResourcesOpts) (*ScanResult, error) {
	url := s.ApiEndpoint + ScanKubernetesResourcesEndpoint
	scanJob := &ScanJob{
		ReportType: ReportType_ERROR,
		Inventory: v1.Inventory{
			Spec: &v1.InventorySpec{
				Assets: []*asset.Asset{
					{
						Connections: []*providers.Config{
							{
								Backend: providers.ProviderType_K8S,
								Options: map[string]string{
									"namespaces":         strings.Join(scanOpts.IncludeNamespaces, ","),
									"namespaces-exclude": strings.Join(scanOpts.ExcludeNamespaces, ","),
								},
								Discover: &providers.Discovery{
									Targets: []string{"auto"},
								},
							},
						},
						ManagedBy: scanOpts.ManagedBy,
					},
				},
			},
		},
	}

	setIntegrationMrn(scanOpts.IntegrationMrn, scanJob)

	if scanOpts.ScanContainerImages {
		scanJob.Inventory.Spec.Assets[0].Connections[0].Discover.Targets = []string{"container-images"}
	}

	reqBodyBytes, err := json.Marshal(scanJob)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := s.request(ctx, url, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &ScanResult{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proto response: %v", err)
	}

	return out, nil
}

type Empty struct{}

const ScheduleKubernetesResourceScanEndpoint = "/Scan/Schedule"

func (s *mondooClient) ScheduleKubernetesResourceScan(ctx context.Context, integrationMrn, resourceKey, managedBy string) (*Empty, error) {
	url := s.ApiEndpoint + ScheduleKubernetesResourceScanEndpoint
	scanJob := &ScanJob{
		ReportType: ReportType_ERROR,
		Inventory: v1.Inventory{
			Spec: &v1.InventorySpec{
				Assets: []*asset.Asset{
					{
						Connections: []*providers.Config{
							{
								Backend: providers.ProviderType_K8S,
								Options: map[string]string{
									"k8s-resources": resourceKey,
								},
								Discover: &providers.Discovery{
									Targets: []string{"auto"},
								},
							},
						},
					},
				},
			},
		},
	}

	if len(managedBy) > 0 {
		scanJob.Inventory.Spec.Assets[0].ManagedBy = managedBy
	}

	setIntegrationMrn(integrationMrn, scanJob)

	reqBodyBytes, err := json.Marshal(scanJob)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := s.request(ctx, url, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &Empty{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proto response: %v", err)
	}

	return out, nil
}

type ReportType int

const (
	ReportType_NONE  ReportType = 0
	ReportType_ERROR ReportType = 1
	ReportType_FULL  ReportType = 2
)

type ScanJob struct {
	Inventory  v1.Inventory `json:"inventory"`
	ReportType ReportType   `protobuf:"varint,22,opt,name=report_type,json=reportType,proto3,enum=mondoo.policy.scan.ReportType" json:"report_type,omitempty"`
}

func NewClient(opts ClientOptions) Client {
	opts.ApiEndpoint = strings.TrimRight(opts.ApiEndpoint, "/")
	mClient := &mondooClient{
		ApiEndpoint: opts.ApiEndpoint,
		Token:       opts.Token,
	}
	return mClient
}

type IntegrationRegisterInput struct {
	// Mrn is the MRN of the integration. It should be provided in the JWT under the "owner" claim
	Mrn   string `protobuf:"bytes,1,opt,name=mrn,proto3" json:"mrn,omitempty"`
	Token string `protobuf:"bytes,2,opt,name=token,proto3" json:"token,omitempty"`
}

type IntegrationRegisterOutput struct {
	// Mrn is the integration MRN
	Mrn string `protobuf:"bytes,1,opt,name=mrn,proto3" json:"mrn,omitempty"`
	// Creds holds all the Mondoo serivce account data
	Creds *ServiceAccountCredentials `protobuf:"bytes,2,opt,name=creds,proto3" json:"creds,omitempty"`
}

type ServiceAccountCredentials struct {
	Mrn         string `protobuf:"bytes,1,opt,name=mrn,proto3" json:"mrn,omitempty"`
	SpaceMrn    string `protobuf:"bytes,2,opt,name=space_mrn,json=spaceMrn,proto3" json:"space_mrn,omitempty"`
	PrivateKey  string `protobuf:"bytes,3,opt,name=private_key,json=privateKey,proto3" json:"private_key,omitempty"`
	Certificate string `protobuf:"bytes,4,opt,name=certificate,proto3" json:"certificate,omitempty"`
	ApiEndpoint string `protobuf:"bytes,5,opt,name=api_endpoint,json=apiEndpoint,proto3" json:"api_endpoint,omitempty"`
}

const IntegrationRegisterEndpoint = "/IntegrationsManager/Register"

func (s *mondooClient) IntegrationRegister(ctx context.Context, in *IntegrationRegisterInput) (*IntegrationRegisterOutput, error) {
	url := s.ApiEndpoint + IntegrationRegisterEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := s.request(ctx, url, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &IntegrationRegisterOutput{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proto response: %v", err)
	}

	return out, nil
}

type IntegrationCheckInInput struct {
	// Mrn should hold the MRN of the integration that is having the CheckIn() called for
	Mrn string `protobuf:"bytes,1,opt,name=mrn,proto3" json:"mrn,omitempty"`
	// optional, ensure the client has the exact same configuration options
	// as the ones saved to the integration/db
	ConfigurationHash string `protobuf:"bytes,2,opt,name=configuration_hash,json=configurationHash,proto3" json:"configuration_hash,omitempty"`
	// source identifier for the integration, e.g. AWS account id
	Identifier string `protobuf:"bytes,3,opt,name=identifier,proto3" json:"identifier,omitempty"`
}

type IntegrationCheckInOutput struct {
	Mrn string `protobuf:"bytes,1,opt,name=mrn,proto3" json:"mrn,omitempty"`
	// true if the configuration hash sent in matches the hash of the stored configuration
	ConfigurationMatch bool `protobuf:"varint,2,opt,name=configuration_match,json=configurationMatch,proto3" json:"configuration_match,omitempty"`
}

const IntegrationCheckInEndpoint = "/IntegrationsManager/CheckIn"

func (s *mondooClient) IntegrationCheckIn(ctx context.Context, in *IntegrationCheckInInput) (*IntegrationCheckInOutput, error) {
	url := s.ApiEndpoint + IntegrationCheckInEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	respBodyBytes, err := s.request(ctx, url, reqBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	out := &IntegrationCheckInOutput{}
	if err = json.Unmarshal(respBodyBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return out, nil
}

type ReportStatusRequest struct {
	Mrn string `protobuf:"bytes,1,opt,name=mrn,proto3" json:"mrn,omitempty"`
	// this is the status of the integration itself (is it active/checking in, errored, etc)
	Status Status `protobuf:"varint,2,opt,name=status,proto3,enum=mondoo.integrations.v1.Status" json:"status,omitempty"`
	// this can be any information about the current state of the integration. it will be displayed to the user as-is where supported
	LastState interface{} `protobuf:"bytes,4,opt,name=last_state,json=lastState,proto3" json:"last_state,omitempty"`
	// Allows the agent to report its current version
	Version string `protobuf:"bytes,5,opt,name=version,proto3" json:"version,omitempty"`
	// messages that convey extra information about the integration - these messages can be informational, warnings or errors. Can be used
	// to report non-critical errors/warnings without neccesarily changing the whole integration status.
	Messages Messages `protobuf:"bytes,7,opt,name=messages,proto3" json:"messages,omitempty"`
}

type Messages struct {
	Messages []IntegrationMessage `protobuf:"bytes,1,opt,name=messages,proto3" json:"messages,omitempty"`
}

type Status int32

const (
	Status_NOT_READY         Status = 0
	Status_WAITING_FOR_SETUP Status = 1
	Status_ACTIVE            Status = 2
	Status_ERROR             Status = 3
	Status_DELETED           Status = 4
	Status_MISSING           Status = 5
	Status_WARNING           Status = 6
)

type IntegrationMessage struct {
	Message         string        `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
	Timestamp       string        `protobuf:"bytes,2,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
	Status          MessageStatus `protobuf:"varint,3,opt,name=status,proto3,enum=mondoo.integrations.v1.MessageStatus" json:"status,omitempty"`
	ReportedByAgent bool          `protobuf:"varint,4,opt,name=reported_by_agent,json=reportedByAgent,proto3" json:"reported_by_agent,omitempty"`
	Identifier      string        `protobuf:"bytes,5,opt,name=identifier,proto3" json:"identifier,omitempty"`
	// Anything extra that the message might contain.
	Extra interface{} `protobuf:"bytes,6,opt,name=extra,proto3" json:"extra,omitempty"`
}

type MessageStatus int32

const (
	MessageStatus_MESSAGE_UNKNOWN MessageStatus = 0
	MessageStatus_MESSAGE_WARNING MessageStatus = 1
	MessageStatus_MESSAGE_ERROR   MessageStatus = 2
	MessageStatus_MESSAGE_INFO    MessageStatus = 3
)

const IntegrationReportStatusEndpoint = "/IntegrationsManager/ReportStatus"

func (s *mondooClient) IntegrationReportStatus(ctx context.Context, in *ReportStatusRequest) error {
	url := s.ApiEndpoint + IntegrationReportStatusEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	_, err = s.request(ctx, url, reqBodyBytes)
	if err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}

	return nil
}

func setIntegrationMrn(integrationMrn string, scanJob *ScanJob) {
	if integrationMrn != "" {
		if scanJob.Inventory.Spec.Assets[0].Labels == nil {
			scanJob.Inventory.Spec.Assets[0].Labels = make(map[string]string)
		}
		scanJob.Inventory.Spec.Assets[0].Labels[constants.MondooAssetsIntegrationLabel] = integrationMrn
	}
}

const GarbageCollectAssetsEndpoint = "/Scan/GarbageCollectAssets"

func (s *mondooClient) GarbageCollectAssets(ctx context.Context, in *scan.GarbageCollectOptions) error {
	url := s.ApiEndpoint + GarbageCollectAssetsEndpoint

	reqBodyBytes, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	_, err = s.request(ctx, url, reqBodyBytes)
	if err != nil {
		return fmt.Errorf("error calling GarbageCollectAssets: %s", err)
	}

	return nil
}
