// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MondooAuditConfigSpec defines the desired state of MondooAuditConfig
type MondooAuditConfigSpec struct {
	// Important: Run "make" to regenerate code after modifying this file

	// Config is an example field of MondooAuditConfig. Edit mondooauditconfig_types.go to remove/update
	// +kubebuilder:validation:Required
	// +required
	MondooCredsSecretRef corev1.LocalObjectReference `json:"mondooCredsSecretRef"`

	// MondooTokenSecretRef can optionally hold a time-limited token that the mondoo-operator will use
	// to create a Mondoo service account saved to the Secret specified in .spec.mondooCredsSecretRef
	// if that Secret does not exist.
	MondooTokenSecretRef corev1.LocalObjectReference ` json:"mondooTokenSecretRef,omitempty"`

	Scanner             Scanner             `json:"scanner,omitempty"`
	KubernetesResources KubernetesResources `json:"kubernetesResources,omitempty"`
	Nodes               Nodes               `json:"nodes,omitempty"`
	ConsoleIntegration  ConsoleIntegration  `json:"consoleIntegration,omitempty"`
	Filtering           Filtering           `json:"filtering,omitempty"`
	Containers          Containers          `json:"containers,omitempty"`

	// Admission is DEPRECATED and ignored. Admission webhooks were removed in v12.1.0.
	// The operator will automatically clean up any orphaned admission resources.
	// See docs/admission-migration-guide.md for migration instructions.
	// +optional
	Admission *DeprecatedAdmission `json:"admission,omitempty"`
}

type Filtering struct {
	Namespaces FilteringSpec `json:"namespaces,omitempty"`
}

type FilteringSpec struct {
	// Include is the list of resources to watch/scan. Setting Include overrides anything in the
	// Exclude list as specifying an Include list is effectively excluding everything except for what
	// is on the Include list.
	Include []string `json:"include,omitempty"`

	// Exclude is the list of resources to ignore for any watching/scanning actions. Use this if
	// the goal is to watch/scan all resources except for this Exclude list.
	Exclude []string `json:"exclude,omitempty"`
}

type ConsoleIntegration struct {
	Enable bool `json:"enable,omitempty"`
}

// Scanner defines the settings for the Mondoo scanner that will be running in the cluster. The same scanner
// is used for scanning the Kubernetes API and the nodes.
type Scanner struct {
	// +kubebuilder:default=mondoo-operator-k8s-resources-scanning
	ServiceAccountName string                      `json:"serviceAccountName,omitempty"`
	Image              Image                       `json:"image,omitempty"`
	Resources          corev1.ResourceRequirements `json:"resources,omitempty"`
	// Number of replicas for the scanner.
	// For enforcing mode, the minimum should be two to prevent problems during Pod failures,
	// e.g. node failure, node scaling, etc.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`

	// PrivateRegistryScanning defines the name of a secret that contains the credentials for the private
	// registries we have to pull images from.
	PrivateRegistriesPullSecretRef corev1.LocalObjectReference `json:"privateRegistriesPullSecretRef,omitempty"`

	// Env allows setting extra environment variables for the scanner. If the operator sets already an env
	// variable with the same name, the value specified here will override it.
	Env []corev1.EnvVar `json:"env,omitempty"`
}

type KubernetesResources struct {
	Enable bool `json:"enable,omitempty"`

	// DEPRECATED: ContainerImageScanning determines whether container images are being scanned. The current implementation
	// runs a separate job once every 24h that scans the container images running in the cluster.
	ContainerImageScanning bool `json:"containerImageScanning,omitempty"`
	// Specify a custom crontab schedule for the Kubernetes resource scanning job. If not specified, the default schedule is used.
	Schedule string `json:"schedule,omitempty"`

	// ResourceWatcher configures real-time resource watching and scanning.
	// When enabled, a deployment will be created that watches for K8s resource changes
	// and scans them immediately rather than waiting for the CronJob schedule.
	ResourceWatcher ResourceWatcherSpec `json:"resourceWatcher,omitempty"`

	// ExternalClusters defines remote K8s clusters to scan from this operator instance.
	// Each external cluster will have its own CronJob created with the appropriate kubeconfig.
	// +optional
	ExternalClusters []ExternalCluster `json:"externalClusters,omitempty"`
}

// ResourceWatcherSpec defines the configuration for real-time resource watching.
type ResourceWatcherSpec struct {
	// Enable enables real-time resource watching and scanning.
	// When enabled, a deployment will be created that watches K8s resources for changes
	// and scans them using cnspec.
	Enable bool `json:"enable,omitempty"`

	// DebounceInterval specifies how long to batch changes before triggering a scan.
	// This prevents excessive scanning when multiple resources change in quick succession.
	// Default is 10 seconds.
	// +kubebuilder:default="10s"
	DebounceInterval metav1.Duration `json:"debounceInterval,omitempty"`

	// MinimumScanInterval specifies the minimum time between scans (rate limit).
	// This provides a hard limit on scan frequency even when resources are changing continuously.
	// Default is 2 minutes.
	// +kubebuilder:default="2m"
	MinimumScanInterval metav1.Duration `json:"minimumScanInterval,omitempty"`

	// WatchAllResources controls whether to watch all resource types or only high-priority ones.
	// When false (default), only watches stable workload resources: Deployments, DaemonSets,
	// StatefulSets, and ReplicaSets. When true, watches all resources including ephemeral ones
	// like Pods, Jobs, and CronJobs.
	WatchAllResources bool `json:"watchAllResources,omitempty"`

	// ResourceTypes specifies which resource types to watch. If not specified, defaults are used
	// based on WatchAllResources setting. When WatchAllResources is false (default), defaults to:
	// deployments, daemonsets, statefulsets, replicasets. When true, defaults to:
	// pods, deployments, daemonsets, statefulsets, replicasets, jobs, cronjobs, services, ingresses, namespaces
	ResourceTypes []string `json:"resourceTypes,omitempty"`
}

// ExternalCluster defines configuration for scanning a remote K8s cluster
type ExternalCluster struct {
	// Name is a unique identifier for this cluster (used in resource names).
	// Must be a valid Kubernetes name (lowercase, alphanumeric, hyphens allowed).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// KubeconfigSecretRef references a Secret containing kubeconfig for the remote cluster.
	// The Secret must have a key "kubeconfig" with the kubeconfig content.
	// Mutually exclusive with ServiceAccountAuth and WorkloadIdentity.
	// +optional
	KubeconfigSecretRef *corev1.LocalObjectReference `json:"kubeconfigSecretRef,omitempty"`

	// ServiceAccountAuth configures authentication using a service account token.
	// Mutually exclusive with KubeconfigSecretRef and WorkloadIdentity.
	// +optional
	ServiceAccountAuth *ServiceAccountAuth `json:"serviceAccountAuth,omitempty"`

	// WorkloadIdentity configures cloud-native Workload Identity Federation authentication.
	// Mutually exclusive with KubeconfigSecretRef, ServiceAccountAuth, and SPIFFEAuth.
	// +optional
	WorkloadIdentity *WorkloadIdentityConfig `json:"workloadIdentity,omitempty"`

	// SPIFFEAuth configures SPIFFE/SPIRE-based authentication using X.509 SVIDs.
	// Mutually exclusive with KubeconfigSecretRef, ServiceAccountAuth, and WorkloadIdentity.
	// +optional
	SPIFFEAuth *SPIFFEAuthConfig `json:"spiffeAuth,omitempty"`

	// Schedule overrides the default schedule for this cluster (optional).
	// If not specified, uses the schedule from KubernetesResources.Schedule.
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// Filtering allows namespace filtering specific to this external cluster.
	// If not specified, uses the global filtering from MondooAuditConfigSpec.Filtering.
	// +optional
	Filtering Filtering `json:"filtering,omitempty"`

	// ContainerImageScanning enables scanning of container images in this external cluster.
	// +optional
	ContainerImageScanning bool `json:"containerImageScanning,omitempty"`

	// PrivateRegistriesPullSecretRef references a Secret containing registry credentials
	// for pulling/scanning private images in this remote cluster.
	// +optional
	PrivateRegistriesPullSecretRef *corev1.LocalObjectReference `json:"privateRegistriesPullSecretRef,omitempty"`
}

// ServiceAccountAuth defines authentication using a Kubernetes service account token
type ServiceAccountAuth struct {
	// Server is the URL of the Kubernetes API server.
	// Example: "https://my-cluster.example.com:6443"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https://.*`
	Server string `json:"server"`

	// CredentialsSecretRef references a Secret containing:
	//   - "token": The service account token (required)
	//   - "ca.crt": The CA certificate (required)
	// +kubebuilder:validation:Required
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`

	// SkipTLSVerify skips TLS verification. NOT RECOMMENDED for production.
	// +optional
	// +kubebuilder:default=false
	SkipTLSVerify bool `json:"skipTLSVerify,omitempty"`
}

// CloudProvider specifies the cloud provider for Workload Identity Federation
type CloudProvider string

const (
	CloudProviderGKE CloudProvider = "gke"
	CloudProviderEKS CloudProvider = "eks"
	CloudProviderAKS CloudProvider = "aks"
)

// WorkloadIdentityConfig configures Workload Identity Federation
type WorkloadIdentityConfig struct {
	// Provider specifies the cloud provider for WIF.
	// +kubebuilder:validation:Enum=gke;eks;aks
	// +kubebuilder:validation:Required
	Provider CloudProvider `json:"provider"`

	// GKE contains GKE-specific Workload Identity configuration.
	// Required when provider is "gke".
	// +optional
	GKE *GKEWorkloadIdentity `json:"gke,omitempty"`

	// EKS contains EKS-specific IRSA configuration.
	// Required when provider is "eks".
	// +optional
	EKS *EKSWorkloadIdentity `json:"eks,omitempty"`

	// AKS contains AKS-specific Azure Workload Identity configuration.
	// Required when provider is "aks".
	// +optional
	AKS *AKSWorkloadIdentity `json:"aks,omitempty"`
}

// GKEWorkloadIdentity configures GKE Workload Identity
type GKEWorkloadIdentity struct {
	// ProjectID is the GCP project ID.
	// +kubebuilder:validation:Required
	ProjectID string `json:"projectId"`

	// ClusterName is the GKE cluster name.
	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName"`

	// ClusterLocation is the region or zone (e.g., "us-central1" or "us-central1-a").
	// +kubebuilder:validation:Required
	ClusterLocation string `json:"clusterLocation"`

	// GoogleServiceAccount is the Google service account to impersonate.
	// Format: <name>@<project>.iam.gserviceaccount.com
	// +kubebuilder:validation:Required
	GoogleServiceAccount string `json:"googleServiceAccount"`
}

// EKSWorkloadIdentity configures AWS EKS IRSA (IAM Roles for Service Accounts)
type EKSWorkloadIdentity struct {
	// Region is the AWS region.
	// +kubebuilder:validation:Required
	Region string `json:"region"`

	// ClusterName is the EKS cluster name.
	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName"`

	// RoleARN is the IAM role to assume.
	// Format: arn:aws:iam::<account>:role/<name>
	// +kubebuilder:validation:Required
	RoleARN string `json:"roleArn"`
}

// AKSWorkloadIdentity configures Azure Workload Identity
type AKSWorkloadIdentity struct {
	// SubscriptionID is the Azure subscription.
	// +kubebuilder:validation:Required
	SubscriptionID string `json:"subscriptionId"`

	// ResourceGroup containing the AKS cluster.
	// +kubebuilder:validation:Required
	ResourceGroup string `json:"resourceGroup"`

	// ClusterName is the AKS cluster name.
	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName"`

	// ClientID is the Azure AD app client ID.
	// +kubebuilder:validation:Required
	ClientID string `json:"clientId"`

	// TenantID is the Azure AD tenant ID.
	// +kubebuilder:validation:Required
	TenantID string `json:"tenantId"`
}

// SPIFFEAuthConfig configures SPIFFE/SPIRE authentication using X.509 SVIDs
type SPIFFEAuthConfig struct {
	// Server is the URL of the remote Kubernetes API server.
	// Example: "https://remote-cluster.example.com:6443"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https://.*`
	Server string `json:"server"`

	// SocketPath is the path to the SPIRE agent's Workload API socket.
	// Defaults to "/run/spire/sockets/agent.sock" if not specified.
	// +optional
	// +kubebuilder:default="/run/spire/sockets/agent.sock"
	SocketPath string `json:"socketPath,omitempty"`

	// TrustBundleSecretRef references a Secret containing the remote cluster's
	// CA certificate for TLS verification. The Secret must have a key "ca.crt".
	// +kubebuilder:validation:Required
	TrustBundleSecretRef corev1.LocalObjectReference `json:"trustBundleSecretRef"`

	// Audience is the intended audience for the SVID (optional).
	// Some SPIRE configurations require this for workload attestation.
	// +optional
	Audience string `json:"audience,omitempty"`
}

// NodeScanStyle specifies the scan style for nodes
type NodeScanStyle string

const (
	NodeScanStyle_CronJob    NodeScanStyle = "cronjob"
	NodeScanStyle_Deployment NodeScanStyle = "deployment"
	NodeScanStyle_DaemonSet  NodeScanStyle = "daemonset"
)

type Nodes struct {
	Enable    bool                        `json:"enable,omitempty"`
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// Schedule specifies a custom crontab schedule for the node scanning job. If not specified, the default schedule is
	// used. Only applicable for CronJob style
	Schedule string `json:"schedule,omitempty"`
	// IntervalTimer is the interval (in minutes) for the node scanning. The default is "60". Only applicable for Deployment
	// style.
	// +kubebuilder:default=60
	IntervalTimer int `json:"intervalTimer,omitempty"`
	// Style specifies how node scanning is deployed. The default is "cronjob" which will create a CronJob for the node scanning.
	// +kubebuilder:validation:Enum=cronjob;deployment;daemonset
	// +kubebuilder:default=cronjob
	Style NodeScanStyle `json:"style,omitempty"`
	// PriorityClassName specifies the name of the PriorityClass for the node scanning workloads.
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// Env allows setting extra environment variables for the node scanner. If the operator sets already an env
	// variable with the same name, the value specified here will override it.
	Env []corev1.EnvVar `json:"env,omitempty"`
}

type Containers struct {
	Enable    bool                        `json:"enable,omitempty"`
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// Specify a custom crontab schedule for the container image scanning job. If not specified, the default schedule is used.
	Schedule string `json:"schedule,omitempty"`
	// Env allows setting extra environment variables for the node scanner. If the operator sets already an env
	// variable with the same name, the value specified here will override it.
	Env []corev1.EnvVar `json:"env,omitempty"`
}

// DeprecatedAdmission exists for backward compatibility during upgrades.
// This field is ignored and will be removed in a future version.
type DeprecatedAdmission struct {
	// Enable is DEPRECATED. Admission webhooks are no longer supported.
	// +optional
	Enable bool `json:"enable,omitempty"`
	// Mode is DEPRECATED.
	// +optional
	Mode string `json:"mode,omitempty"`
	// Replicas is DEPRECATED.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
	// ServiceAccountName is DEPRECATED.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// CertificateProvisioning is DEPRECATED.
	// +optional
	CertificateProvisioning *DeprecatedCertificateProvisioning `json:"certificateProvisioning,omitempty"`
	// Image is DEPRECATED.
	// +optional
	Image *Image `json:"image,omitempty"`
}

// DeprecatedCertificateProvisioning exists for backward compatibility.
type DeprecatedCertificateProvisioning struct {
	// Mode is DEPRECATED.
	// +optional
	Mode string `json:"mode,omitempty"`
}

type Image struct {
	Name string `json:"name,omitempty"`
	Tag  string `json:"tag,omitempty"`
}

// MondooAuditConfigStatus defines the observed state of MondooAuditConfig
type MondooAuditConfigStatus struct {
	// Important: Run "make" to regenerate code after modifying this file

	// Pods store the name of the pods which are running mondoo instances
	Pods []string `json:"pods,omitempty"`

	// Conditions includes detailed status for the MondooAuditConfig
	Conditions []MondooAuditConfigCondition `json:"conditions,omitempty"`

	// ReconciledByOperatorVersion contains the version of the operator which reconciled this MondooAuditConfig
	ReconciledByOperatorVersion string `json:"reconciledByOperatorVersion,omitempty"`
}

type MondooAuditConfigCondition struct {
	// Type is the specific type of the condition
	// +kubebuilder:validation:Required
	// +required
	Type MondooAuditConfigConditionType `json:"type"`
	// Status is the status of the condition
	// +kubebuilder:validation:Required
	// +required
	Status corev1.ConditionStatus `json:"status"`
	// LastUpdateTime is the last time we probed the condition
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// LastTransitionTime is the last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason is a unique, one-word, CamelCase reason for the condition's last transition
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable message indicating details about the last transition
	Message string `json:"message,omitempty"`
	// AffectedPods, when filled, contains a list which are affected by an issue
	AffectedPods []string `json:"affectedPods,omitempty"`
	// MemoryLimit contains the currently active memory limit for a Pod
	MemoryLimit string `json:"memoryLimit,omitempty"`
}

// MondooOperatorConfigConditionType is a valid value for MondooOperatorConfig.Status.Condition[].Type
type MondooAuditConfigConditionType string

const (
	// Indicates weather NodeScanning is Degraded
	NodeScanningDegraded MondooAuditConfigConditionType = "NodeScanningDegraded"
	// Indicates weather Kubernetes resources scanning is Degraded
	K8sResourcesScanningDegraded MondooAuditConfigConditionType = "K8sResourcesScanningDegraded"
	// Indicates weather Kubernetes container image scanning is Degraded
	K8sContainerImageScanningDegraded MondooAuditConfigConditionType = "K8sContainerImageScanningDegraded"
	// Indicates whether the resource watcher is Degraded
	ResourceWatcherDegraded MondooAuditConfigConditionType = "ResourceWatcherDegraded"
	// Indicates weather the operator itself is Degraded
	MondooOperatorDegraded MondooAuditConfigConditionType = "MondooOperatorDegraded"
	// MondooIntegrationDegraded will hold the status for any issues encountered while trying to CheckIn()
	// on behalf of the Mondoo integration MRN
	MondooIntegrationDegraded MondooAuditConfigConditionType = "IntegrationDegraded"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MondooAuditConfig is the Schema for the mondooauditconfigs API
type MondooAuditConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MondooAuditConfigSpec   `json:"spec,omitempty"`
	Status MondooAuditConfigStatus `json:"status,omitempty"`
}

// Hub marks this type as a conversion hub.
func (*MondooAuditConfig) Hub() {}

//+kubebuilder:object:root=true

// MondooAuditConfigList contains a list of MondooAuditConfig
type MondooAuditConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MondooAuditConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MondooAuditConfig{}, &MondooAuditConfigList{})
}
