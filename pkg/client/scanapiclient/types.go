// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package scanapiclient

import (
	"context"

	"go.mondoo.com/cnquery/v11/providers-sdk/v1/inventory"
	"go.mondoo.com/cnspec/v11/policy/scan"
	"go.mondoo.com/mondoo-operator/pkg/client/common"
	"google.golang.org/protobuf/types/known/structpb"
)

//go:generate ./../../../bin/mockgen -source=./types.go -destination=./mock/client_generated.go -package=mock

type ScanApiClient interface {
	common.HealthCheckClient
	RunAdmissionReview(context.Context, *AdmissionReviewJob) (*ScanResult, error)
	ScanKubernetesResources(ctx context.Context, scanOpts *ScanKubernetesResourcesOpts) (*ScanResult, error)
	ScheduleKubernetesResourceScan(ctx context.Context, integrationMrn, resourceKey, managedBy string) (*Empty, error)
	GarbageCollectAssets(context.Context, *scan.GarbageCollectOptions) error
}

type AdmissionReviewJob struct {
	Data *structpb.Struct `json:"data,omitempty"`
	// Map of string keys and values that can be used to organize and categorize the assets
	Labels     map[string]string `json:"labels,omitempty"`
	ReportType ReportType        `json:"report_type,omitempty"`
	// Additional options for the manifest job
	Options map[string]string `json:"options,omitempty"`
	// Additional discovery settings for the manifest job
	Discovery *inventory.Discovery `json:"discovery,omitempty"`
}

type File struct {
	Data []byte `json:"data,omitempty"`
}

// A valid result would come back as a '2'
const ValidScanResult = uint32(2)

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

type ReportType int

const (
	ReportType_NONE  ReportType = 0
	ReportType_ERROR ReportType = 1
	ReportType_FULL  ReportType = 2
)

type ScanJob struct {
	Inventory  inventory.Inventory `json:"inventory"`
	ReportType ReportType          `protobuf:"varint,22,opt,name=report_type,json=reportType,proto3,enum=mondoo.policy.scan.ReportType" json:"report_type,omitempty"`
}

type ScanKubernetesResourcesOpts struct {
	IntegrationMrn string
	// If set to true, the scan will discover only container images and not Kubernetes resources
	ScanContainerImages bool
	ManagedBy           string
	IncludeNamespaces   []string
	ExcludeNamespaces   []string
}

type Empty struct{}
