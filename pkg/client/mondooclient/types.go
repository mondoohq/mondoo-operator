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
	IntegrationConfigure(context.Context, *IntegrationConfigureInput) (*IntegrationConfigureOutput, error)
	IntegrationReportStatus(context.Context, *ReportStatusRequest) error
	IntegrationCreate(context.Context, *IntegrationCreateInput) (*IntegrationCreateOutput, error)
	IntegrationList(context.Context, *IntegrationListInput) (*IntegrationListOutput, error)
	IntegrationGetToken(context.Context, *IntegrationGetTokenInput) (*IntegrationGetTokenOutput, error)
	IntegrationDelete(context.Context, *IntegrationDeleteInput) error

	GarbageCollectAssets(context.Context, *GarbageCollectAssetsRequest) error
	RefreshAssetScores(context.Context, *RefreshAssetScoresRequest) (*RefreshAssetScoresResponse, error)
}

// ExchangeRegistrationTokenInput is used for converting a JWT to a Mondoo service account
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
	// Creds holds all the Mondoo service account data
	Creds *ServiceAccountCredentials `protobuf:"bytes,2,opt,name=creds,proto3" json:"creds,omitempty"`
}

type ServiceAccountCredentials struct {
	Mrn         string `protobuf:"bytes,1,opt,name=mrn,proto3" json:"mrn,omitempty"`
	SpaceMrn    string `protobuf:"bytes,2,opt,name=space_mrn,json=spaceMrn,proto3" json:"space_mrn,omitempty"`
	PrivateKey  string `protobuf:"bytes,3,opt,name=private_key,json=privateKey,proto3" json:"private_key,omitempty"` //nolint:gosec
	Certificate string `protobuf:"bytes,4,opt,name=certificate,proto3" json:"certificate,omitempty"`
	ApiEndpoint string `protobuf:"bytes,5,opt,name=api_endpoint,json=apiEndpoint,proto3" json:"api_endpoint,omitempty"`
	ScopeMrn    string `json:"scope_mrn,omitempty"`
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

type IntegrationConfigureInput struct {
	Mrn string `protobuf:"bytes,1,opt,name=mrn,proto3" json:"mrn,omitempty"`
}

type IntegrationConfigureOutput struct {
	Details *IntegrationConfigureDetails `json:"details,omitempty"`
}

type IntegrationConfigureDetails struct {
	Config string `json:"config,omitempty"`
}

type K8sIntegrationConfig struct {
	PauseScanning bool `json:"pauseScanning,omitempty"`
}

// IntegrationTypeK8s is the IntegrationsManager Type enum value name for Kubernetes client
// integrations (mondoo.integrations.v1.Type, K8S = 2). Sent as the enum name string, which
// protojson accepts on the server side.
const IntegrationTypeK8s = "K8S"

// Node scanning style enum value names (mondoo.integrations.v1.ScanNodesStyle).
const (
	ScanNodesStyleCronJob    = "CRONJOB"
	ScanNodesStyleDeployment = "DEPLOYMENT"
	ScanNodesStyleDaemonSet  = "DAEMONSET"
)

// IntegrationCreateInput matches the server-side CreateIntegrationRequest proto on the
// IntegrationsManager service (/IntegrationsManager/Create). JSON field names follow the
// proto field names, which the server's protojson unmarshaling accepts.
type IntegrationCreateInput struct {
	// ScopeMrn is the space (or organization) MRN the integration is created in.
	ScopeMrn string `protobuf:"bytes,1,opt,name=scope_mrn,json=scopeMrn,proto3" json:"scope_mrn,omitempty"`
	// Name is the display name shown in the Mondoo console.
	Name string `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	// Identifiers are source identifiers for the integration (e.g. an AWS account id, or for
	// the operator a cluster UID). They can be filtered on via IntegrationList and are the
	// operator's idempotency key.
	Identifiers []string `protobuf:"bytes,3,rep,name=identifiers,proto3" json:"identifiers,omitempty"`
	// ConfigurationInput holds the type-specific scan configuration.
	ConfigurationInput *IntegrationConfigurationInput `protobuf:"bytes,4,opt,name=configuration_input,json=configurationInput,proto3" json:"configuration_input,omitempty"`
	Labels             map[string]string              `protobuf:"bytes,5,rep,name=labels,proto3" json:"labels,omitempty"`
	// Type is the integration type enum value name, e.g. IntegrationTypeK8s.
	Type string `protobuf:"varint,6,opt,name=type,proto3,enum=mondoo.integrations.v1.Type" json:"type,omitempty"`
	// LongLivedToken controls whether the registration token returned with the created
	// integration expires (false ⇒ 30 minutes). Service accounts derived from short-lived
	// tokens do not expire, so the operator can keep this false.
	LongLivedToken bool `protobuf:"varint,7,opt,name=long_lived_token,json=longLivedToken,proto3" json:"long_lived_token,omitempty"`
}

type IntegrationConfigurationInput struct {
	K8sOptions *K8sConfigurationOptionsInput `protobuf:"bytes,4,opt,name=k8s_options,json=k8sOptions,proto3,oneof" json:"k8s_options,omitempty"`
}

// K8sConfigurationOptionsInput matches mondoo.integrations.v1.K8sConfigurationOptionsInput.
// pause_scanning is deliberately omitted: it is server-managed and force-cleared on create.
type K8sConfigurationOptionsInput struct {
	ScanNodes          bool     `protobuf:"varint,1,opt,name=scan_nodes,json=scanNodes,proto3" json:"scan_nodes,omitempty"`
	ScanNodesStyle     string   `protobuf:"varint,8,opt,name=scan_nodes_style,json=scanNodesStyle,proto3,enum=mondoo.integrations.v1.ScanNodesStyle" json:"scan_nodes_style,omitempty"`
	ScanWorkloads      bool     `protobuf:"varint,2,opt,name=scan_workloads,json=scanWorkloads,proto3" json:"scan_workloads,omitempty"`
	ScanDeploys        bool     `protobuf:"varint,3,opt,name=scan_deploys,json=scanDeploys,proto3" json:"scan_deploys,omitempty"`
	ScanPublicImages   bool     `protobuf:"varint,5,opt,name=scan_public_images,json=scanPublicImages,proto3" json:"scan_public_images,omitempty"`
	NamespaceAllowList []string `protobuf:"bytes,6,rep,name=namespace_allow_list,json=namespaceAllowList,proto3" json:"namespace_allow_list,omitempty"`
	NamespaceDenyList  []string `protobuf:"bytes,7,rep,name=namespace_deny_list,json=namespaceDenyList,proto3" json:"namespace_deny_list,omitempty"`
	ScanLocalCluster   bool     `protobuf:"varint,10,opt,name=scan_local_cluster,json=scanLocalCluster,proto3" json:"scan_local_cluster,omitempty"`
	Schedule           string   `protobuf:"bytes,13,opt,name=schedule,proto3" json:"schedule,omitempty"`
	NodesSchedule      string   `protobuf:"bytes,14,opt,name=nodes_schedule,json=nodesSchedule,proto3" json:"nodes_schedule,omitempty"`
	ContainersSchedule string   `protobuf:"bytes,15,opt,name=containers_schedule,json=containersSchedule,proto3" json:"containers_schedule,omitempty"`
}

type IntegrationCreateOutput struct {
	Integration *Integration `protobuf:"bytes,1,opt,name=integration,proto3" json:"integration,omitempty"`
}

// Integration mirrors the fields of the server-side Integration proto that the operator
// consumes. Responses are marshaled by the server with proto field names, and enums are
// serialized as their value names (e.g. status "ACTIVE", type "K8S").
type Integration struct {
	Mrn    string `protobuf:"bytes,1,opt,name=mrn,proto3" json:"mrn,omitempty"`
	Name   string `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Status string `protobuf:"varint,3,opt,name=status,proto3,enum=mondoo.integrations.v1.Status" json:"status,omitempty"`
	// Token is the registration token bound to the integration. Only populated by
	// IntegrationCreate; IntegrationList returns integrations without tokens.
	Token       string            `protobuf:"bytes,9,opt,name=token,proto3" json:"token,omitempty"`
	Identifiers []string          `protobuf:"bytes,10,rep,name=identifiers,proto3" json:"identifiers,omitempty"`
	Labels      map[string]string `protobuf:"bytes,11,rep,name=labels,proto3" json:"labels,omitempty"`
	Type        string            `protobuf:"varint,12,opt,name=type,proto3,enum=mondoo.integrations.v1.Type" json:"type,omitempty"`
	ScopeMrn    string            `protobuf:"bytes,20,opt,name=scope_mrn,json=scopeMrn,proto3" json:"scope_mrn,omitempty"`
}

// IntegrationListInput matches the server-side Query proto for IntegrationsManager/List.
// Note: the proto field is literally named "scopeMrn" (camelCase) in the .proto file.
type IntegrationListInput struct {
	ScopeMrn        string   `protobuf:"bytes,1,opt,name=scopeMrn,proto3" json:"scopeMrn,omitempty"`
	Types           []string `protobuf:"varint,2,rep,name=types,proto3,enum=mondoo.integrations.v1.Type" json:"types,omitempty"`
	ExcludeStatuses []Status `protobuf:"varint,3,rep,packed,name=exclude_statuses,json=excludeStatuses,proto3,enum=mondoo.integrations.v1.Status" json:"exclude_statuses,omitempty"`
	// Identifiers filters to integrations that contain any of these identifiers.
	Identifiers []string `protobuf:"bytes,6,rep,name=identifiers,proto3" json:"identifiers,omitempty"`
}

type IntegrationListOutput struct {
	Integrations []Integration `protobuf:"bytes,1,rep,name=integrations,proto3" json:"integrations,omitempty"`
}

type IntegrationGetTokenInput struct {
	Mrn string `protobuf:"bytes,1,opt,name=mrn,proto3" json:"mrn,omitempty"`
	// LongLivedToken requests a non-expiring token; the operator keeps this false and uses
	// the (30-minute) token immediately for IntegrationRegister.
	LongLivedToken bool `protobuf:"varint,7,opt,name=long_lived_token,json=longLivedToken,proto3" json:"long_lived_token,omitempty"`
}

type IntegrationGetTokenOutput struct {
	Token string `protobuf:"bytes,1,opt,name=token,proto3" json:"token,omitempty"`
}

type IntegrationDeleteInput struct {
	Mrn string `protobuf:"bytes,1,opt,name=mrn,proto3" json:"mrn,omitempty"`
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
	// to report non-critical errors/warnings without necessarily changing the whole integration status.
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

// GarbageCollectAssetsRequest matches the server-side PurgeAssetsRequest proto
// on the PolicyResolver service (/PolicyResolver/PurgeAssets).
type GarbageCollectAssetsRequest struct {
	SpaceMrn        string            `protobuf:"bytes,1,opt,name=spaceMrn,proto3" json:"spaceMrn,omitempty"` // Deprecated: use ScopeMrn
	AssetMrns       []string          `protobuf:"bytes,2,rep,name=asset_mrns,json=assetMrns,proto3" json:"asset_mrns,omitempty"`
	PurgeAll        bool              `protobuf:"varint,3,opt,name=purge_all,json=purgeAll,proto3" json:"purge_all,omitempty"`
	DateFilter      *DateFilter       `protobuf:"bytes,4,opt,name=date_filter,json=dateFilter,proto3" json:"date_filter,omitempty"`
	ManagedBy       string            `protobuf:"bytes,5,opt,name=managed_by,json=managedBy,proto3" json:"managed_by,omitempty"`
	PlatformRuntime string            `protobuf:"bytes,6,opt,name=platform_runtime,json=platformRuntime,proto3" json:"platform_runtime,omitempty"`
	Labels          map[string]string `protobuf:"bytes,7,rep,name=labels,proto3" json:"labels,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	ScopeMrn        string            `protobuf:"bytes,8,opt,name=scope_mrn,json=scopeMrn,proto3" json:"scope_mrn,omitempty"`
}

type DateFilter struct {
	Timestamp  string          `protobuf:"bytes,1,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
	Comparison Comparison      `protobuf:"varint,2,opt,name=comparison,proto3,enum=cnspec.policy.v1.Comparison" json:"comparison,omitempty"`
	Field      DateFilterField `protobuf:"varint,3,opt,name=field,proto3,enum=cnspec.policy.v1.DateFilterField" json:"field,omitempty"`
}

type Comparison int32

const (
	Comparison_GREATER_THAN Comparison = 0
	Comparison_LESS_THAN    Comparison = 1
)

type DateFilterField int32

const (
	DateFilterField_FILTER_LAST_UPDATED DateFilterField = 0
	DateFilterField_FILTER_CREATED      DateFilterField = 1
)

// RefreshAssetScores — server-side proto spec:
//
//	service PolicyResolver {
//	  rpc RefreshAssetScores(RefreshAssetScoresRequest)
//	      returns (RefreshAssetScoresResponse);
//	}
//
//	message RefreshAssetScoresRequest {
//	  string scope_mrn = 1;        // space MRN to scope the query
//	  string managed_by = 2;       // e.g. "mondoo-operator-<clusterUID>"
//	  string platform_runtime = 3; // e.g. "docker-image"
//	  map<string, string> labels = 4;
//	}
//
//	message RefreshAssetScoresResponse {
//	  // Assets whose scores were successfully re-evaluated.
//	  repeated AssetRefreshResult refreshed = 1;
//	  // Assets found matching the filter but whose scores could not
//	  // be refreshed (e.g. no policies resolved yet).
//	  repeated AssetRefreshResult missing = 2;
//	}
//
//	message AssetRefreshResult {
//	  string asset_mrn = 1;
//	  // Platform IDs for this asset, e.g.
//	  // "//platformid.api.mondoo.app/runtime/docker/images/<sha256-hex>"
//	  repeated string platform_ids = 2;
//	}
//
// The operator collects platform_ids from refreshed entries and passes
// them to cnspec as "platformids-exclude". Missing entries are NOT
// excluded — they represent assets whose scores could not be refreshed,
// so they need to be re-scanned.
type RefreshAssetScoresRequest struct {
	ScopeMrn          string            `protobuf:"bytes,1,opt,name=scope_mrn,json=scopeMrn,proto3" json:"scope_mrn,omitempty"`
	ManagedBy         string            `protobuf:"bytes,2,opt,name=managed_by,json=managedBy,proto3" json:"managed_by,omitempty"`
	PlatformRuntime   string            `protobuf:"bytes,3,opt,name=platform_runtime,json=platformRuntime,proto3" json:"platform_runtime,omitempty"`
	Labels            map[string]string `protobuf:"bytes,4,rep,name=labels,proto3" json:"labels,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	EnableCacheExpiry bool              `protobuf:"varint,5,opt,name=enable_cache_expiry,json=enableCacheExpiry,proto3" json:"enable_cache_expiry,omitempty"`
	CacheTTLSeconds   int64             `protobuf:"varint,6,opt,name=cache_ttl_seconds,json=cacheTtlSeconds,proto3" json:"cache_ttl_seconds,omitempty"`
}

type RefreshAssetScoresResponse struct {
	Refreshed []AssetRefreshResult `protobuf:"bytes,1,rep,name=refreshed,proto3" json:"refreshed,omitempty"`
	Missing   []AssetRefreshResult `protobuf:"bytes,2,rep,name=missing,proto3" json:"missing,omitempty"`
}

type AssetRefreshResult struct {
	AssetMrn    string   `protobuf:"bytes,1,opt,name=asset_mrn,json=assetMrn,proto3" json:"asset_mrn,omitempty"`
	PlatformIds []string `protobuf:"bytes,2,rep,name=platform_ids,json=platformIds,proto3" json:"platform_ids,omitempty"`
}
