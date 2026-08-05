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
	ScanNodes        bool   `json:"scanNodes,omitempty"`
	ScanNodesStyle   string `json:"scanNodesStyle,omitempty"`
	ScanWorkloads    bool   `json:"scanWorkloads,omitempty"`
	ScanDeploys      bool   `json:"scanDeploys,omitempty"`
	ScanPublicImages bool   `json:"scanPublicImages,omitempty"`
	ScanLocalCluster bool   `json:"scanLocalCluster,omitempty"`

	NamespaceAllowList []string `json:"namespaceAllowList,omitempty"`
	NamespaceDenyList  []string `json:"namespaceDenyList,omitempty"`

	Schedule           string `json:"schedule,omitempty"`
	NodesSchedule      string `json:"nodesSchedule,omitempty"`
	ContainersSchedule string `json:"containersSchedule,omitempty"`

	ExternalClusters                []K8sExternalClusterConfig `json:"externalClusters,omitempty"`
	PrivateRegistriesPullSecretRefs []string                   `json:"privateRegistriesPullSecretRefs,omitempty"`
	ContainersWif                   *K8sContainersWifConfig    `json:"containersWif,omitempty"`

	PauseScanning bool `json:"pauseScanning,omitempty"`

	ScannerReplicas     int32                          `json:"scannerReplicas,omitempty"`
	ScannerResources    *K8sResourceRequirementsConfig `json:"scannerResources,omitempty"`
	NodesResources      *K8sResourceRequirementsConfig `json:"nodesResources,omitempty"`
	ContainersResources *K8sResourceRequirementsConfig `json:"containersResources,omitempty"`

	ResourceWatcher *K8sResourceWatcherConfig `json:"resourceWatcher,omitempty"`

	ContainerRepositoriesAllowList []string `json:"containerRepositoriesAllowList,omitempty"`
	ContainerRepositoriesDenyList  []string `json:"containerRepositoriesDenyList,omitempty"`

	ScanCacheEnabled bool   `json:"scanCacheEnabled,omitempty"`
	ScanCacheTTL     string `json:"scanCacheTtl,omitempty"`

	K8sActiveDeadline        int64 `json:"k8sActiveDeadline,omitempty"`
	ContainersActiveDeadline int64 `json:"containersActiveDeadline,omitempty"`

	JobOverrides           *K8sJobOverridesConfig `json:"jobOverrides,omitempty"`
	ScannerJobOverrides    *K8sJobOverridesConfig `json:"scannerJobOverrides,omitempty"`
	NodesJobOverrides      *K8sJobOverridesConfig `json:"nodesJobOverrides,omitempty"`
	ContainersJobOverrides *K8sJobOverridesConfig `json:"containersJobOverrides,omitempty"`

	AssetAnnotations map[string]string `json:"assetAnnotations,omitempty"`
	SpaceID          string            `json:"spaceId,omitempty"`

	NodesPriorityClassName string `json:"nodesPriorityClassName,omitempty"`
	NodesIntervalTimer     int32  `json:"nodesIntervalTimer,omitempty"`

	ScannerEnv    []K8sEnvVarConfig `json:"scannerEnv,omitempty"`
	NodesEnv      []K8sEnvVarConfig `json:"nodesEnv,omitempty"`
	ContainersEnv []K8sEnvVarConfig `json:"containersEnv,omitempty"`
}

type K8sResourceRequirementsConfig struct {
	CPURequest string `json:"cpuRequest,omitempty"`
	CPULimit   string `json:"cpuLimit,omitempty"`
	MemRequest string `json:"memRequest,omitempty"`
	MemLimit   string `json:"memLimit,omitempty"`
}

type K8sResourceWatcherConfig struct {
	Enable              bool     `json:"enable,omitempty"`
	DebounceInterval    string   `json:"debounceInterval,omitempty"`
	MinimumScanInterval string   `json:"minimumScanInterval,omitempty"`
	WatchAllResources   bool     `json:"watchAllResources,omitempty"`
	ResourceTypes       []string `json:"resourceTypes,omitempty"`
}

type K8sJobOverridesConfig struct {
	TTLSecondsAfterFinished int32                 `json:"ttlSecondsAfterFinished,omitempty"`
	Annotations             map[string]string     `json:"annotations,omitempty"`
	NodeSelector            map[string]string     `json:"nodeSelector,omitempty"`
	Tolerations             []K8sTolerationConfig `json:"tolerations,omitempty"`
	Labels                  map[string]string     `json:"labels,omitempty"`
}

type K8sTolerationConfig struct {
	Key      string `json:"key,omitempty"`
	Operator string `json:"operator,omitempty"`
	Value    string `json:"value,omitempty"`
	Effect   string `json:"effect,omitempty"`
}

type K8sEnvVarConfig struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

type K8sExternalClusterConfig struct {
	Name                   string           `json:"name,omitempty"`
	KubeconfigSecret       string           `json:"kubeconfigSecret,omitempty"`
	Server                 string           `json:"server,omitempty"`
	CredentialsSecret      string           `json:"credentialsSecret,omitempty"`
	SkipTlsVerify          bool             `json:"skipTlsVerify,omitempty"`
	NamespaceAllowList     []string         `json:"namespaceAllowList,omitempty"`
	NamespaceDenyList      []string         `json:"namespaceDenyList,omitempty"`
	ContainerImageScanning bool             `json:"containerImageScanning,omitempty"`
	WifProvider            string           `json:"wifProvider,omitempty"`
	Gke                    *K8sGkeWifConfig `json:"gke,omitempty"`
	Eks                    *K8sEksWifConfig `json:"eks,omitempty"`
	Aks                    *K8sAksWifConfig `json:"aks,omitempty"`
	Spiffe                 *K8sSpiffeConfig `json:"spiffe,omitempty"`
	Vault                  *K8sVaultConfig  `json:"vault,omitempty"`
}

type K8sContainersWifConfig struct {
	Provider string           `json:"provider,omitempty"`
	Gke      *K8sGkeWifConfig `json:"gke,omitempty"`
	Eks      *K8sEksWifConfig `json:"eks,omitempty"`
	Aks      *K8sAksWifConfig `json:"aks,omitempty"`
}

type K8sGkeWifConfig struct {
	ProjectID            string `json:"projectId,omitempty"`
	ClusterName          string `json:"clusterName,omitempty"`
	ClusterLocation      string `json:"clusterLocation,omitempty"`
	GoogleServiceAccount string `json:"googleServiceAccount,omitempty"`
	Endpoint             string `json:"endpoint,omitempty"`
}

type K8sEksWifConfig struct {
	Region      string `json:"region,omitempty"`
	ClusterName string `json:"clusterName,omitempty"`
	RoleARN     string `json:"roleArn,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
}

type K8sAksWifConfig struct {
	SubscriptionID string `json:"subscriptionId,omitempty"`
	ResourceGroup  string `json:"resourceGroup,omitempty"`
	ClusterName    string `json:"clusterName,omitempty"`
	ClientID       string `json:"clientId,omitempty"`
	TenantID       string `json:"tenantId,omitempty"`
	LoginServer    string `json:"loginServer,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`
}

type K8sSpiffeConfig struct {
	Server            string `json:"server,omitempty"`
	TrustBundleSecret string `json:"trustBundleSecret,omitempty"`
	SocketPath        string `json:"socketPath,omitempty"`
	Audience          string `json:"audience,omitempty"`
}

type K8sVaultConfig struct {
	Server              string `json:"server,omitempty"`
	VaultAddr           string `json:"vaultAddr,omitempty"`
	AuthRole            string `json:"authRole,omitempty"`
	CredsRole           string `json:"credsRole,omitempty"`
	AuthPath            string `json:"authPath,omitempty"`
	SecretsPath         string `json:"secretsPath,omitempty"`
	KubernetesNamespace string `json:"kubernetesNamespace,omitempty"`
	TTL                 string `json:"ttl,omitempty"`
	CaCertSecret        string `json:"caCertSecret,omitempty"`
	TargetCaCertSecret  string `json:"targetCaCertSecret,omitempty"`
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
	ScanNodes          bool                       `protobuf:"varint,1,opt,name=scan_nodes,json=scanNodes,proto3" json:"scan_nodes,omitempty"`
	ScanNodesStyle     string                     `protobuf:"varint,8,opt,name=scan_nodes_style,json=scanNodesStyle,proto3,enum=mondoo.integrations.v1.ScanNodesStyle" json:"scan_nodes_style,omitempty"`
	ScanWorkloads      bool                       `protobuf:"varint,2,opt,name=scan_workloads,json=scanWorkloads,proto3" json:"scan_workloads,omitempty"`
	ScanDeploys        bool                       `protobuf:"varint,3,opt,name=scan_deploys,json=scanDeploys,proto3" json:"scan_deploys,omitempty"`
	ScanPublicImages   bool                       `protobuf:"varint,5,opt,name=scan_public_images,json=scanPublicImages,proto3" json:"scan_public_images,omitempty"`
	NamespaceAllowList []string                   `protobuf:"bytes,6,rep,name=namespace_allow_list,json=namespaceAllowList,proto3" json:"namespace_allow_list,omitempty"`
	NamespaceDenyList  []string                   `protobuf:"bytes,7,rep,name=namespace_deny_list,json=namespaceDenyList,proto3" json:"namespace_deny_list,omitempty"`
	ExternalClusters   []K8sExternalClusterConfig `protobuf:"bytes,9,rep,name=external_clusters,json=externalClusters,proto3" json:"external_clusters,omitempty"`
	ScanLocalCluster   bool                       `protobuf:"varint,10,opt,name=scan_local_cluster,json=scanLocalCluster,proto3" json:"scan_local_cluster,omitempty"`
	Schedule           string                     `protobuf:"bytes,13,opt,name=schedule,proto3" json:"schedule,omitempty"`
	NodesSchedule      string                     `protobuf:"bytes,14,opt,name=nodes_schedule,json=nodesSchedule,proto3" json:"nodes_schedule,omitempty"`
	ContainersSchedule string                     `protobuf:"bytes,15,opt,name=containers_schedule,json=containersSchedule,proto3" json:"containers_schedule,omitempty"`
	ContainersWif      *K8sContainersWifConfig    `protobuf:"bytes,16,opt,name=containers_wif,json=containersWif,proto3" json:"containers_wif,omitempty"`

	ScannerReplicas     int32                          `protobuf:"varint,18,opt,name=scanner_replicas,json=scannerReplicas,proto3" json:"scanner_replicas,omitempty"`
	ScannerResources    *K8sResourceRequirementsConfig `protobuf:"bytes,19,opt,name=scanner_resources,json=scannerResources,proto3" json:"scanner_resources,omitempty"`
	NodesResources      *K8sResourceRequirementsConfig `protobuf:"bytes,20,opt,name=nodes_resources,json=nodesResources,proto3" json:"nodes_resources,omitempty"`
	ContainersResources *K8sResourceRequirementsConfig `protobuf:"bytes,21,opt,name=containers_resources,json=containersResources,proto3" json:"containers_resources,omitempty"`
	ResourceWatcher     *K8sResourceWatcherConfig      `protobuf:"bytes,22,opt,name=resource_watcher,json=resourceWatcher,proto3" json:"resource_watcher,omitempty"`

	ContainerRepositoriesAllowList []string `protobuf:"bytes,23,rep,name=container_repositories_allow_list,json=containerRepositoriesAllowList,proto3" json:"container_repositories_allow_list,omitempty"`
	ContainerRepositoriesDenyList  []string `protobuf:"bytes,24,rep,name=container_repositories_deny_list,json=containerRepositoriesDenyList,proto3" json:"container_repositories_deny_list,omitempty"`

	ScanCacheEnabled bool   `protobuf:"varint,25,opt,name=scan_cache_enabled,json=scanCacheEnabled,proto3" json:"scan_cache_enabled,omitempty"`
	ScanCacheTTL     string `protobuf:"bytes,26,opt,name=scan_cache_ttl,json=scanCacheTtl,proto3" json:"scan_cache_ttl,omitempty"`

	K8sActiveDeadline        int64 `protobuf:"varint,27,opt,name=k8s_active_deadline,json=k8sActiveDeadline,proto3" json:"k8s_active_deadline,omitempty"`
	ContainersActiveDeadline int64 `protobuf:"varint,28,opt,name=containers_active_deadline,json=containersActiveDeadline,proto3" json:"containers_active_deadline,omitempty"`

	JobOverrides           *K8sJobOverridesConfig `protobuf:"bytes,29,opt,name=job_overrides,json=jobOverrides,proto3" json:"job_overrides,omitempty"`
	ScannerJobOverrides    *K8sJobOverridesConfig `protobuf:"bytes,30,opt,name=scanner_job_overrides,json=scannerJobOverrides,proto3" json:"scanner_job_overrides,omitempty"`
	NodesJobOverrides      *K8sJobOverridesConfig `protobuf:"bytes,31,opt,name=nodes_job_overrides,json=nodesJobOverrides,proto3" json:"nodes_job_overrides,omitempty"`
	ContainersJobOverrides *K8sJobOverridesConfig `protobuf:"bytes,32,opt,name=containers_job_overrides,json=containersJobOverrides,proto3" json:"containers_job_overrides,omitempty"`

	AssetAnnotations map[string]string `protobuf:"bytes,33,rep,name=asset_annotations,json=assetAnnotations,proto3" json:"asset_annotations,omitempty"`
	SpaceID          string            `protobuf:"bytes,34,opt,name=space_id,json=spaceId,proto3" json:"space_id,omitempty"`

	NodesPriorityClassName string `protobuf:"bytes,35,opt,name=nodes_priority_class_name,json=nodesPriorityClassName,proto3" json:"nodes_priority_class_name,omitempty"`
	NodesIntervalTimer     int32  `protobuf:"varint,36,opt,name=nodes_interval_timer,json=nodesIntervalTimer,proto3" json:"nodes_interval_timer,omitempty"`

	ScannerEnv    []K8sEnvVarConfig `protobuf:"bytes,37,rep,name=scanner_env,json=scannerEnv,proto3" json:"scanner_env,omitempty"`
	NodesEnv      []K8sEnvVarConfig `protobuf:"bytes,38,rep,name=nodes_env,json=nodesEnv,proto3" json:"nodes_env,omitempty"`
	ContainersEnv []K8sEnvVarConfig `protobuf:"bytes,39,rep,name=containers_env,json=containersEnv,proto3" json:"containers_env,omitempty"`
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
