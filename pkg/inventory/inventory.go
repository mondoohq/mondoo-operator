package inventory

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MondooInventory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MondooInventorySpec `json:"spec,omitempty"`
}

type MondooInventorySpec struct {
	Assets []Asset `json:"assets,omitempty"`
}

// Asset is copy pasted from the protobuf-generated go files.
// Commenting out fields that have further structure if we don't make use of those fields.
type Asset struct {
	Id   string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Mrn  string `protobuf:"bytes,2,opt,name=mrn,proto3" json:"mrn,omitempty"`
	Name string `protobuf:"bytes,3,opt,name=name,proto3" json:"name,omitempty"`
	// 3rd-party platform id eg. amazon arn, gcp resource name or ssh host key
	PlatformIds []string `protobuf:"bytes,4,rep,name=platform_ids,json=platformIds,proto3" json:"platform_ids,omitempty"`
	// asset state
	// State    State              `protobuf:"varint,5,opt,name=state,proto3,enum=mondoo.motor.asset.v1.State" json:"state,omitempty"`
	// Platform *platform.Platform `protobuf:"bytes,6,opt,name=platform,proto3" json:"platform,omitempty"`
	// key is a lower case string of connection type
	Connections []TransportConfig `protobuf:"bytes,17,rep,name=connections,proto3" json:"connections,omitempty"`
	// labeled assets can be searched by labels
	Labels map[string]string `protobuf:"bytes,18,rep,name=labels,proto3" json:"labels,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// additional information that is not touched by the system
	Annotations map[string]string `protobuf:"bytes,19,rep,name=annotations,proto3" json:"annotations,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// additional options for that asset
	Options map[string]string `protobuf:"bytes,20,rep,name=options,proto3" json:"options,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// relationships to parent
	ParentPlatformId string `protobuf:"bytes,30,opt,name=parent_platform_id,json=parentPlatformId,proto3" json:"parent_platform_id,omitempty"`
	// platform id detection mechanisms
	IdDetector []string `protobuf:"bytes,31,rep,name=id_detector,json=idDetector,proto3" json:"id_detector,omitempty"`
	// indicator is this is a fleet asset or a CI/CD run
	// Category AssetCategory `protobuf:"varint,32,opt,name=category,proto3,enum=mondoo.motor.asset.v1.AssetCategory" json:"category,omitempty"`
}

// TransportConfig is copy pasted from the protobuf-generated go files.
// Commenting out fields that have further structure if we don't make use of those fields.
type TransportConfig struct {
	// Backend TransportBackend `protobuf:"varint,1,opt,name=backend,proto3,enum=mondoo.motor.transports.v1.TransportBackend" json:"backend,omitempty"`
	Backend TransportBackend `json:"backend,omitempty"`
	Host    string           `protobuf:"bytes,2,opt,name=host,proto3" json:"host,omitempty"`
	// Ports are not int by default, eg. docker://centos:latest parses a string as port
	// Therefore it is up to the transport to convert the port to what they need
	Port int32  `protobuf:"varint,3,opt,name=port,proto3" json:"port,omitempty"`
	Path string `protobuf:"bytes,4,opt,name=path,proto3" json:"path,omitempty"`
	// credentials available for this transport configuration
	// Credentials []*vault.Credential `protobuf:"bytes,11,rep,name=credentials,proto3" json:"credentials,omitempty"`
	Insecure bool `protobuf:"varint,8,opt,name=insecure,proto3" json:"insecure,omitempty"` // disable ssl/tls checks
	// Sudo        *Sudo               `protobuf:"bytes,21,opt,name=sudo,proto3" json:"sudo,omitempty"`
	Record  bool              `protobuf:"varint,22,opt,name=record,proto3" json:"record,omitempty"`
	Options map[string]string `protobuf:"bytes,23,rep,name=options,proto3" json:"options,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// flags for additional asset discovery
	// Discover *Discovery `protobuf:"bytes,27,opt,name=discover,proto3" json:"discover,omitempty"`
	// additional platform information, passed-through
	// Kind    Kind   `protobuf:"varint,24,opt,name=kind,proto3,enum=mondoo.motor.transports.v1.Kind" json:"kind,omitempty"`
	Runtime string `protobuf:"bytes,25,opt,name=runtime,proto3" json:"runtime,omitempty"`
	// configuration to uniquly identify an specific asset for multi-asset api connection
	PlatformId string `protobuf:"bytes,26,opt,name=platform_id,json=platformId,proto3" json:"platform_id,omitempty"`
}

type TransportBackend int32

const (
	TransportBackend_CONNECTION_LOCAL_OS                TransportBackend = 0
	TransportBackend_CONNECTION_DOCKER_ENGINE_IMAGE     TransportBackend = 1
	TransportBackend_CONNECTION_DOCKER_ENGINE_CONTAINER TransportBackend = 2
	TransportBackend_CONNECTION_SSH                     TransportBackend = 3
	TransportBackend_CONNECTION_WINRM                   TransportBackend = 4
	TransportBackend_CONNECTION_AWS_SSM_RUN_COMMAND     TransportBackend = 5
	TransportBackend_CONNECTION_CONTAINER_REGISTRY      TransportBackend = 6
	TransportBackend_CONNECTION_TAR                     TransportBackend = 7
	TransportBackend_CONNECTION_MOCK                    TransportBackend = 8
	TransportBackend_CONNECTION_VSPHERE                 TransportBackend = 9
	TransportBackend_CONNECTION_ARISTAEOS               TransportBackend = 10
	TransportBackend_CONNECTION_AWS                     TransportBackend = 12
	TransportBackend_CONNECTION_GCP                     TransportBackend = 13
	TransportBackend_CONNECTION_AZURE                   TransportBackend = 14
	TransportBackend_CONNECTION_MS365                   TransportBackend = 15
	TransportBackend_CONNECTION_IPMI                    TransportBackend = 16
	TransportBackend_CONNECTION_VSPHERE_VM              TransportBackend = 17
	TransportBackend_CONNECTION_FS                      TransportBackend = 18
	TransportBackend_CONNECTION_K8S                     TransportBackend = 19
	TransportBackend_CONNECTION_EQUINIX_METAL           TransportBackend = 20
	TransportBackend_CONNECTION_DOCKER                  TransportBackend = 21 // unspecified if this is a container or image
	TransportBackend_CONNECTION_GITHUB                  TransportBackend = 22
	TransportBackend_CONNECTION_VAGRANT                 TransportBackend = 23
	TransportBackend_CONNECTION_AWS_EC2_EBS             TransportBackend = 24
	TransportBackend_CONNECTION_GITLAB                  TransportBackend = 25
	TransportBackend_CONNECTION_TERRAFORM               TransportBackend = 26
	TransportBackend_CONNECTION_HOST                    TransportBackend = 27
)
