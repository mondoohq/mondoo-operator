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
	Connections []*TransportConfig `protobuf:"bytes,17,rep,name=connections,proto3" json:"connections,omitempty"`
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
	// an int32 originally, but we can just treat it as a string
	// Backend TransportBackend `protobuf:"varint,1,opt,name=backend,proto3,enum=mondoo.motor.transports.v1.TransportBackend" json:"backend,omitempty"`
	Backend string `json:"backend,omitempty"`
	Host    string `protobuf:"bytes,2,opt,name=host,proto3" json:"host,omitempty"`
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
