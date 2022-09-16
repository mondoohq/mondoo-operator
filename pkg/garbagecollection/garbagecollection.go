package garbagecollection

// copied from protobuf generated files
type GarbageCollectOptions struct {
	// state         protoimpl.MessageState
	// sizeCache     protoimpl.SizeCache
	// unknownFields protoimpl.UnknownFields

	OlderThan       string `protobuf:"bytes,1,opt,name=older_than,json=olderThan,proto3" json:"older_than,omitempty"` // RFC3339
	MangagedBy      string `protobuf:"bytes,2,opt,name=mangaged_by,json=mangagedBy,proto3" json:"mangaged_by,omitempty"`
	PlatformRuntime string `protobuf:"bytes,3,opt,name=platform_runtime,json=platformRuntime,proto3" json:"platform_runtime,omitempty"`
}
