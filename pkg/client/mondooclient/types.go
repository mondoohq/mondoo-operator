// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondooclient

import (
	"context"

	"go.mondoo.com/mondoo-operator/pkg/client/common"
)

//go:generate ./../../../bin/mockgen -source=./types.go -destination=./mock/client_generated.go -package=mock

type MondooClient interface {
	common.HealthCheckClient
	ExchangeRegistrationToken(context.Context, *ExchangeRegistrationTokenInput) (*ExchangeRegistrationTokenOutput, error)

	IntegrationRegister(context.Context, *IntegrationRegisterInput) (*IntegrationRegisterOutput, error)
	IntegrationCheckIn(context.Context, *IntegrationCheckInInput) (*IntegrationCheckInOutput, error)
	IntegrationReportStatus(context.Context, *ReportStatusRequest) error

	GarbageCollectAssets(context.Context, *GarbageCollectOptions) error
}

// ExchangeRegistrationTokenInput is used for converting a JWT to a Mondoo serivce account
type ExchangeRegistrationTokenInput struct {
	// JWT token, only available during creation
	Token string `protobuf:"bytes,1,opt,name=token,proto3" json:"token,omitempty"`
}

type ExchangeRegistrationTokenOutput struct {
	ServiceAccount string `json:"serviceAccount,omitempty"`
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
	PrivateKey  string `protobuf:"bytes,3,opt,name=private_key,json=privateKey,proto3" json:"private_key,omitempty"` //nolint:gosec
	Certificate string `protobuf:"bytes,4,opt,name=certificate,proto3" json:"certificate,omitempty"`
	ApiEndpoint string `protobuf:"bytes,5,opt,name=api_endpoint,json=apiEndpoint,proto3" json:"api_endpoint,omitempty"`
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

// GarbageCollectOptions contains filters for garbage collection of assets
type GarbageCollectOptions struct {
	ManagedBy       string            `json:"managed_by,omitempty"`
	PlatformRuntime string            `json:"platform_runtime,omitempty"`
	OlderThan       string            `json:"older_than,omitempty"` // RFC3339 timestamp
	Labels          map[string]string `json:"labels,omitempty"`
}
