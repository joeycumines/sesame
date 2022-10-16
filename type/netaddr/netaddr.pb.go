// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        v3.21.6
// source: sesame/type/netaddr.proto

package netaddr

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// NetAddr represents a network end point address.
//
// The two fields network and address are conventionally strings that can be passed as the arguments to Go's net.Dial,
// but the exact form and meaning of the strings is up to the implementation.
//
// See also Go's net.Addr interface.
type NetAddr struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Network string `protobuf:"bytes,1,opt,name=network,proto3" json:"network,omitempty"`
	Address string `protobuf:"bytes,2,opt,name=address,proto3" json:"address,omitempty"`
}

func (x *NetAddr) Reset() {
	*x = NetAddr{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_type_netaddr_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *NetAddr) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NetAddr) ProtoMessage() {}

func (x *NetAddr) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_type_netaddr_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use NetAddr.ProtoReflect.Descriptor instead.
func (*NetAddr) Descriptor() ([]byte, []int) {
	return file_sesame_type_netaddr_proto_rawDescGZIP(), []int{0}
}

func (x *NetAddr) GetNetwork() string {
	if x != nil {
		return x.Network
	}
	return ""
}

func (x *NetAddr) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

var File_sesame_type_netaddr_proto protoreflect.FileDescriptor

var file_sesame_type_netaddr_proto_rawDesc = []byte{
	0x0a, 0x19, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2f, 0x74, 0x79, 0x70, 0x65, 0x2f, 0x6e, 0x65,
	0x74, 0x61, 0x64, 0x64, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0b, 0x73, 0x65, 0x73,
	0x61, 0x6d, 0x65, 0x2e, 0x74, 0x79, 0x70, 0x65, 0x22, 0x3d, 0x0a, 0x07, 0x4e, 0x65, 0x74, 0x41,
	0x64, 0x64, 0x72, 0x12, 0x18, 0x0a, 0x07, 0x6e, 0x65, 0x74, 0x77, 0x6f, 0x72, 0x6b, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6e, 0x65, 0x74, 0x77, 0x6f, 0x72, 0x6b, 0x12, 0x18, 0x0a,
	0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07,
	0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x42, 0x2c, 0x5a, 0x2a, 0x67, 0x69, 0x74, 0x68, 0x75,
	0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6a, 0x6f, 0x65, 0x79, 0x63, 0x75, 0x6d, 0x69, 0x6e, 0x65,
	0x73, 0x2f, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2f, 0x74, 0x79, 0x70, 0x65, 0x2f, 0x6e, 0x65,
	0x74, 0x61, 0x64, 0x64, 0x72, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_sesame_type_netaddr_proto_rawDescOnce sync.Once
	file_sesame_type_netaddr_proto_rawDescData = file_sesame_type_netaddr_proto_rawDesc
)

func file_sesame_type_netaddr_proto_rawDescGZIP() []byte {
	file_sesame_type_netaddr_proto_rawDescOnce.Do(func() {
		file_sesame_type_netaddr_proto_rawDescData = protoimpl.X.CompressGZIP(file_sesame_type_netaddr_proto_rawDescData)
	})
	return file_sesame_type_netaddr_proto_rawDescData
}

var file_sesame_type_netaddr_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_sesame_type_netaddr_proto_goTypes = []interface{}{
	(*NetAddr)(nil), // 0: sesame.type.NetAddr
}
var file_sesame_type_netaddr_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_sesame_type_netaddr_proto_init() }
func file_sesame_type_netaddr_proto_init() {
	if File_sesame_type_netaddr_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_sesame_type_netaddr_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*NetAddr); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_sesame_type_netaddr_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_sesame_type_netaddr_proto_goTypes,
		DependencyIndexes: file_sesame_type_netaddr_proto_depIdxs,
		MessageInfos:      file_sesame_type_netaddr_proto_msgTypes,
	}.Build()
	File_sesame_type_netaddr_proto = out.File
	file_sesame_type_netaddr_proto_rawDesc = nil
	file_sesame_type_netaddr_proto_goTypes = nil
	file_sesame_type_netaddr_proto_depIdxs = nil
}