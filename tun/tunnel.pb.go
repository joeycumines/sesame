// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.34.2
// 	protoc        v5.27.3
// source: sesame/v1alpha1/tunnel.proto

package tun

import (
	grpctunnel "github.com/joeycumines/sesame/type/grpctunnel"
	status "google.golang.org/genproto/googleapis/rpc/status"
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

type TunnelRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id uint64 `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	// Types that are assignable to Data:
	//
	//	*TunnelRequest_Connect_
	//	*TunnelRequest_Stream_
	Data isTunnelRequest_Data `protobuf_oneof:"data"`
}

func (x *TunnelRequest) Reset() {
	*x = TunnelRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TunnelRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TunnelRequest) ProtoMessage() {}

func (x *TunnelRequest) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TunnelRequest.ProtoReflect.Descriptor instead.
func (*TunnelRequest) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_tunnel_proto_rawDescGZIP(), []int{0}
}

func (x *TunnelRequest) GetId() uint64 {
	if x != nil {
		return x.Id
	}
	return 0
}

func (m *TunnelRequest) GetData() isTunnelRequest_Data {
	if m != nil {
		return m.Data
	}
	return nil
}

func (x *TunnelRequest) GetConnect() *TunnelRequest_Connect {
	if x, ok := x.GetData().(*TunnelRequest_Connect_); ok {
		return x.Connect
	}
	return nil
}

func (x *TunnelRequest) GetStream() *TunnelRequest_Stream {
	if x, ok := x.GetData().(*TunnelRequest_Stream_); ok {
		return x.Stream
	}
	return nil
}

type isTunnelRequest_Data interface {
	isTunnelRequest_Data()
}

type TunnelRequest_Connect_ struct {
	Connect *TunnelRequest_Connect `protobuf:"bytes,2,opt,name=connect,proto3,oneof"` // request
}

type TunnelRequest_Stream_ struct {
	Stream *TunnelRequest_Stream `protobuf:"bytes,3,opt,name=stream,proto3,oneof"` // response (only after stream successfully connected or error)
}

func (*TunnelRequest_Connect_) isTunnelRequest_Data() {}

func (*TunnelRequest_Stream_) isTunnelRequest_Data() {}

type TunnelResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id uint64 `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	// Types that are assignable to Data:
	//
	//	*TunnelResponse_Connect_
	//	*TunnelResponse_Stream_
	Data isTunnelResponse_Data `protobuf_oneof:"data"`
}

func (x *TunnelResponse) Reset() {
	*x = TunnelResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TunnelResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TunnelResponse) ProtoMessage() {}

func (x *TunnelResponse) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TunnelResponse.ProtoReflect.Descriptor instead.
func (*TunnelResponse) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_tunnel_proto_rawDescGZIP(), []int{1}
}

func (x *TunnelResponse) GetId() uint64 {
	if x != nil {
		return x.Id
	}
	return 0
}

func (m *TunnelResponse) GetData() isTunnelResponse_Data {
	if m != nil {
		return m.Data
	}
	return nil
}

func (x *TunnelResponse) GetConnect() *TunnelResponse_Connect {
	if x, ok := x.GetData().(*TunnelResponse_Connect_); ok {
		return x.Connect
	}
	return nil
}

func (x *TunnelResponse) GetStream() *TunnelResponse_Stream {
	if x, ok := x.GetData().(*TunnelResponse_Stream_); ok {
		return x.Stream
	}
	return nil
}

type isTunnelResponse_Data interface {
	isTunnelResponse_Data()
}

type TunnelResponse_Connect_ struct {
	Connect *TunnelResponse_Connect `protobuf:"bytes,2,opt,name=connect,proto3,oneof"` // response
}

type TunnelResponse_Stream_ struct {
	Stream *TunnelResponse_Stream `protobuf:"bytes,3,opt,name=stream,proto3,oneof"` // request
}

func (*TunnelResponse_Connect_) isTunnelResponse_Data() {}

func (*TunnelResponse_Stream_) isTunnelResponse_Data() {}

type StreamRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Data:
	//
	//	*StreamRequest_Connect_
	//	*StreamRequest_Dial_
	//	*StreamRequest_Tunnel
	Data isStreamRequest_Data `protobuf_oneof:"data"`
}

func (x *StreamRequest) Reset() {
	*x = StreamRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StreamRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StreamRequest) ProtoMessage() {}

func (x *StreamRequest) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StreamRequest.ProtoReflect.Descriptor instead.
func (*StreamRequest) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_tunnel_proto_rawDescGZIP(), []int{2}
}

func (m *StreamRequest) GetData() isStreamRequest_Data {
	if m != nil {
		return m.Data
	}
	return nil
}

func (x *StreamRequest) GetConnect() *StreamRequest_Connect {
	if x, ok := x.GetData().(*StreamRequest_Connect_); ok {
		return x.Connect
	}
	return nil
}

func (x *StreamRequest) GetDial() *StreamRequest_Dial {
	if x, ok := x.GetData().(*StreamRequest_Dial_); ok {
		return x.Dial
	}
	return nil
}

func (x *StreamRequest) GetTunnel() *grpctunnel.ServerToClient {
	if x, ok := x.GetData().(*StreamRequest_Tunnel); ok {
		return x.Tunnel
	}
	return nil
}

type isStreamRequest_Data interface {
	isStreamRequest_Data()
}

type StreamRequest_Connect_ struct {
	Connect *StreamRequest_Connect `protobuf:"bytes,1,opt,name=connect,proto3,oneof"` // request
}

type StreamRequest_Dial_ struct {
	Dial *StreamRequest_Dial `protobuf:"bytes,2,opt,name=dial,proto3,oneof"` // response
}

type StreamRequest_Tunnel struct {
	Tunnel *grpctunnel.ServerToClient `protobuf:"bytes,3,opt,name=tunnel,proto3,oneof"`
}

func (*StreamRequest_Connect_) isStreamRequest_Data() {}

func (*StreamRequest_Dial_) isStreamRequest_Data() {}

func (*StreamRequest_Tunnel) isStreamRequest_Data() {}

type StreamResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Data:
	//
	//	*StreamResponse_Connect_
	//	*StreamResponse_Dial_
	//	*StreamResponse_Tunnel
	Data isStreamResponse_Data `protobuf_oneof:"data"`
}

func (x *StreamResponse) Reset() {
	*x = StreamResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StreamResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StreamResponse) ProtoMessage() {}

func (x *StreamResponse) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StreamResponse.ProtoReflect.Descriptor instead.
func (*StreamResponse) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_tunnel_proto_rawDescGZIP(), []int{3}
}

func (m *StreamResponse) GetData() isStreamResponse_Data {
	if m != nil {
		return m.Data
	}
	return nil
}

func (x *StreamResponse) GetConnect() *StreamResponse_Connect {
	if x, ok := x.GetData().(*StreamResponse_Connect_); ok {
		return x.Connect
	}
	return nil
}

func (x *StreamResponse) GetDial() *StreamResponse_Dial {
	if x, ok := x.GetData().(*StreamResponse_Dial_); ok {
		return x.Dial
	}
	return nil
}

func (x *StreamResponse) GetTunnel() *grpctunnel.ClientToServer {
	if x, ok := x.GetData().(*StreamResponse_Tunnel); ok {
		return x.Tunnel
	}
	return nil
}

type isStreamResponse_Data interface {
	isStreamResponse_Data()
}

type StreamResponse_Connect_ struct {
	Connect *StreamResponse_Connect `protobuf:"bytes,1,opt,name=connect,proto3,oneof"` // response
}

type StreamResponse_Dial_ struct {
	Dial *StreamResponse_Dial `protobuf:"bytes,2,opt,name=dial,proto3,oneof"` // request
}

type StreamResponse_Tunnel struct {
	Tunnel *grpctunnel.ClientToServer `protobuf:"bytes,3,opt,name=tunnel,proto3,oneof"`
}

func (*StreamResponse_Connect_) isStreamResponse_Data() {}

func (*StreamResponse_Dial_) isStreamResponse_Data() {}

func (*StreamResponse_Tunnel) isStreamResponse_Data() {}

type TunnelRequest_Connect struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Remote string `protobuf:"bytes,1,opt,name=remote,proto3" json:"remote,omitempty"`
	// Maximum number of concurrent stream requests.
	StreamBuffer int32 `protobuf:"varint,2,opt,name=stream_buffer,json=streamBuffer,proto3" json:"stream_buffer,omitempty"`
}

func (x *TunnelRequest_Connect) Reset() {
	*x = TunnelRequest_Connect{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TunnelRequest_Connect) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TunnelRequest_Connect) ProtoMessage() {}

func (x *TunnelRequest_Connect) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TunnelRequest_Connect.ProtoReflect.Descriptor instead.
func (*TunnelRequest_Connect) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_tunnel_proto_rawDescGZIP(), []int{0, 0}
}

func (x *TunnelRequest_Connect) GetRemote() string {
	if x != nil {
		return x.Remote
	}
	return ""
}

func (x *TunnelRequest_Connect) GetStreamBuffer() int32 {
	if x != nil {
		return x.StreamBuffer
	}
	return 0
}

type TunnelRequest_Stream struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Error *status.Status `protobuf:"bytes,1,opt,name=error,proto3" json:"error,omitempty"`
}

func (x *TunnelRequest_Stream) Reset() {
	*x = TunnelRequest_Stream{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TunnelRequest_Stream) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TunnelRequest_Stream) ProtoMessage() {}

func (x *TunnelRequest_Stream) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TunnelRequest_Stream.ProtoReflect.Descriptor instead.
func (*TunnelRequest_Stream) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_tunnel_proto_rawDescGZIP(), []int{0, 1}
}

func (x *TunnelRequest_Stream) GetError() *status.Status {
	if x != nil {
		return x.Error
	}
	return nil
}

type TunnelResponse_Connect struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *TunnelResponse_Connect) Reset() {
	*x = TunnelResponse_Connect{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TunnelResponse_Connect) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TunnelResponse_Connect) ProtoMessage() {}

func (x *TunnelResponse_Connect) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TunnelResponse_Connect.ProtoReflect.Descriptor instead.
func (*TunnelResponse_Connect) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_tunnel_proto_rawDescGZIP(), []int{1, 0}
}

// Models a request to initialise a new stream.
type TunnelResponse_Stream struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The target endpoint.
	Endpoint string `protobuf:"bytes,1,opt,name=endpoint,proto3" json:"endpoint,omitempty"`
}

func (x *TunnelResponse_Stream) Reset() {
	*x = TunnelResponse_Stream{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TunnelResponse_Stream) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TunnelResponse_Stream) ProtoMessage() {}

func (x *TunnelResponse_Stream) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TunnelResponse_Stream.ProtoReflect.Descriptor instead.
func (*TunnelResponse_Stream) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_tunnel_proto_rawDescGZIP(), []int{1, 1}
}

func (x *TunnelResponse_Stream) GetEndpoint() string {
	if x != nil {
		return x.Endpoint
	}
	return ""
}

type StreamRequest_Connect struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Remote string `protobuf:"bytes,1,opt,name=remote,proto3" json:"remote,omitempty"`
}

func (x *StreamRequest_Connect) Reset() {
	*x = StreamRequest_Connect{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StreamRequest_Connect) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StreamRequest_Connect) ProtoMessage() {}

func (x *StreamRequest_Connect) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StreamRequest_Connect.ProtoReflect.Descriptor instead.
func (*StreamRequest_Connect) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_tunnel_proto_rawDescGZIP(), []int{2, 0}
}

func (x *StreamRequest_Connect) GetRemote() string {
	if x != nil {
		return x.Remote
	}
	return ""
}

type StreamRequest_Dial struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *StreamRequest_Dial) Reset() {
	*x = StreamRequest_Dial{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[9]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StreamRequest_Dial) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StreamRequest_Dial) ProtoMessage() {}

func (x *StreamRequest_Dial) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[9]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StreamRequest_Dial.ProtoReflect.Descriptor instead.
func (*StreamRequest_Dial) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_tunnel_proto_rawDescGZIP(), []int{2, 1}
}

type StreamResponse_Connect struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *StreamResponse_Connect) Reset() {
	*x = StreamResponse_Connect{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[10]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StreamResponse_Connect) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StreamResponse_Connect) ProtoMessage() {}

func (x *StreamResponse_Connect) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[10]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StreamResponse_Connect.ProtoReflect.Descriptor instead.
func (*StreamResponse_Connect) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_tunnel_proto_rawDescGZIP(), []int{3, 0}
}

type StreamResponse_Dial struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Endpoint string `protobuf:"bytes,1,opt,name=endpoint,proto3" json:"endpoint,omitempty"`
}

func (x *StreamResponse_Dial) Reset() {
	*x = StreamResponse_Dial{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[11]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StreamResponse_Dial) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StreamResponse_Dial) ProtoMessage() {}

func (x *StreamResponse_Dial) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_tunnel_proto_msgTypes[11]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StreamResponse_Dial.ProtoReflect.Descriptor instead.
func (*StreamResponse_Dial) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_tunnel_proto_rawDescGZIP(), []int{3, 1}
}

func (x *StreamResponse_Dial) GetEndpoint() string {
	if x != nil {
		return x.Endpoint
	}
	return ""
}

var File_sesame_v1alpha1_tunnel_proto protoreflect.FileDescriptor

var file_sesame_v1alpha1_tunnel_proto_rawDesc = []byte{
	0x0a, 0x1c, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2f, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61,
	0x31, 0x2f, 0x74, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0f,
	0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x1a,
	0x17, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x72, 0x70, 0x63, 0x2f, 0x73, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1c, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65,
	0x2f, 0x74, 0x79, 0x70, 0x65, 0x2f, 0x67, 0x72, 0x70, 0x63, 0x74, 0x75, 0x6e, 0x6e, 0x65, 0x6c,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xa8, 0x02, 0x0a, 0x0d, 0x54, 0x75, 0x6e, 0x6e, 0x65,
	0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x04, 0x52, 0x02, 0x69, 0x64, 0x12, 0x42, 0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x6e,
	0x65, 0x63, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x26, 0x2e, 0x73, 0x65, 0x73, 0x61,
	0x6d, 0x65, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x54, 0x75, 0x6e, 0x6e,
	0x65, 0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63,
	0x74, 0x48, 0x00, 0x52, 0x07, 0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x12, 0x3f, 0x0a, 0x06,
	0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x25, 0x2e, 0x73,
	0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x54,
	0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x53, 0x74, 0x72,
	0x65, 0x61, 0x6d, 0x48, 0x00, 0x52, 0x06, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x1a, 0x46, 0x0a,
	0x07, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x72, 0x65, 0x6d, 0x6f,
	0x74, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x72, 0x65, 0x6d, 0x6f, 0x74, 0x65,
	0x12, 0x23, 0x0a, 0x0d, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x5f, 0x62, 0x75, 0x66, 0x66, 0x65,
	0x72, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52, 0x0c, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x42,
	0x75, 0x66, 0x66, 0x65, 0x72, 0x1a, 0x32, 0x0a, 0x06, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x12,
	0x28, 0x0a, 0x05, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x12,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x72, 0x70, 0x63, 0x2e, 0x53, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x52, 0x05, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x42, 0x06, 0x0a, 0x04, 0x64, 0x61, 0x74,
	0x61, 0x22, 0xe0, 0x01, 0x0a, 0x0e, 0x54, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x04,
	0x52, 0x02, 0x69, 0x64, 0x12, 0x43, 0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x27, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x76,
	0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x54, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x2e, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x48, 0x00,
	0x52, 0x07, 0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x12, 0x40, 0x0a, 0x06, 0x73, 0x74, 0x72,
	0x65, 0x61, 0x6d, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x26, 0x2e, 0x73, 0x65, 0x73, 0x61,
	0x6d, 0x65, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x54, 0x75, 0x6e, 0x6e,
	0x65, 0x6c, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x2e, 0x53, 0x74, 0x72, 0x65, 0x61,
	0x6d, 0x48, 0x00, 0x52, 0x06, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x1a, 0x09, 0x0a, 0x07, 0x43,
	0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x1a, 0x24, 0x0a, 0x06, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d,
	0x12, 0x1a, 0x0a, 0x08, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x08, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x42, 0x06, 0x0a, 0x04,
	0x64, 0x61, 0x74, 0x61, 0x22, 0xf8, 0x01, 0x0a, 0x0d, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x42, 0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63,
	0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x26, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65,
	0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x48,
	0x00, 0x52, 0x07, 0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x12, 0x39, 0x0a, 0x04, 0x64, 0x69,
	0x61, 0x6c, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x23, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d,
	0x65, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x53, 0x74, 0x72, 0x65, 0x61,
	0x6d, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x44, 0x69, 0x61, 0x6c, 0x48, 0x00, 0x52,
	0x04, 0x64, 0x69, 0x61, 0x6c, 0x12, 0x35, 0x0a, 0x06, 0x74, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x74,
	0x79, 0x70, 0x65, 0x2e, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x54, 0x6f, 0x43, 0x6c, 0x69, 0x65,
	0x6e, 0x74, 0x48, 0x00, 0x52, 0x06, 0x74, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x1a, 0x21, 0x0a, 0x07,
	0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x72, 0x65, 0x6d, 0x6f, 0x74,
	0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x72, 0x65, 0x6d, 0x6f, 0x74, 0x65, 0x1a,
	0x06, 0x0a, 0x04, 0x44, 0x69, 0x61, 0x6c, 0x42, 0x06, 0x0a, 0x04, 0x64, 0x61, 0x74, 0x61, 0x22,
	0xff, 0x01, 0x0a, 0x0e, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x12, 0x43, 0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x27, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x76, 0x31, 0x61,
	0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x2e, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x48, 0x00, 0x52, 0x07,
	0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x12, 0x3a, 0x0a, 0x04, 0x64, 0x69, 0x61, 0x6c, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x24, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x76,
	0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x2e, 0x44, 0x69, 0x61, 0x6c, 0x48, 0x00, 0x52, 0x04, 0x64,
	0x69, 0x61, 0x6c, 0x12, 0x35, 0x0a, 0x06, 0x74, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x74, 0x79, 0x70,
	0x65, 0x2e, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x54, 0x6f, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72,
	0x48, 0x00, 0x52, 0x06, 0x74, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x1a, 0x09, 0x0a, 0x07, 0x43, 0x6f,
	0x6e, 0x6e, 0x65, 0x63, 0x74, 0x1a, 0x22, 0x0a, 0x04, 0x44, 0x69, 0x61, 0x6c, 0x12, 0x1a, 0x0a,
	0x08, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x08, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x42, 0x06, 0x0a, 0x04, 0x64, 0x61, 0x74,
	0x61, 0x32, 0xb1, 0x01, 0x0a, 0x0d, 0x54, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x53, 0x65, 0x72, 0x76,
	0x69, 0x63, 0x65, 0x12, 0x4f, 0x0a, 0x06, 0x54, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x12, 0x1e, 0x2e,
	0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e,
	0x54, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1f, 0x2e,
	0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e,
	0x54, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00,
	0x28, 0x01, 0x30, 0x01, 0x12, 0x4f, 0x0a, 0x06, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x12, 0x1e,
	0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31,
	0x2e, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1f,
	0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31,
	0x2e, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22,
	0x00, 0x28, 0x01, 0x30, 0x01, 0x42, 0x23, 0x5a, 0x21, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e,
	0x63, 0x6f, 0x6d, 0x2f, 0x6a, 0x6f, 0x65, 0x79, 0x63, 0x75, 0x6d, 0x69, 0x6e, 0x65, 0x73, 0x2f,
	0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2f, 0x74, 0x75, 0x6e, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
}

var (
	file_sesame_v1alpha1_tunnel_proto_rawDescOnce sync.Once
	file_sesame_v1alpha1_tunnel_proto_rawDescData = file_sesame_v1alpha1_tunnel_proto_rawDesc
)

func file_sesame_v1alpha1_tunnel_proto_rawDescGZIP() []byte {
	file_sesame_v1alpha1_tunnel_proto_rawDescOnce.Do(func() {
		file_sesame_v1alpha1_tunnel_proto_rawDescData = protoimpl.X.CompressGZIP(file_sesame_v1alpha1_tunnel_proto_rawDescData)
	})
	return file_sesame_v1alpha1_tunnel_proto_rawDescData
}

var file_sesame_v1alpha1_tunnel_proto_msgTypes = make([]protoimpl.MessageInfo, 12)
var file_sesame_v1alpha1_tunnel_proto_goTypes = []any{
	(*TunnelRequest)(nil),             // 0: sesame.v1alpha1.TunnelRequest
	(*TunnelResponse)(nil),            // 1: sesame.v1alpha1.TunnelResponse
	(*StreamRequest)(nil),             // 2: sesame.v1alpha1.StreamRequest
	(*StreamResponse)(nil),            // 3: sesame.v1alpha1.StreamResponse
	(*TunnelRequest_Connect)(nil),     // 4: sesame.v1alpha1.TunnelRequest.Connect
	(*TunnelRequest_Stream)(nil),      // 5: sesame.v1alpha1.TunnelRequest.Stream
	(*TunnelResponse_Connect)(nil),    // 6: sesame.v1alpha1.TunnelResponse.Connect
	(*TunnelResponse_Stream)(nil),     // 7: sesame.v1alpha1.TunnelResponse.Stream
	(*StreamRequest_Connect)(nil),     // 8: sesame.v1alpha1.StreamRequest.Connect
	(*StreamRequest_Dial)(nil),        // 9: sesame.v1alpha1.StreamRequest.Dial
	(*StreamResponse_Connect)(nil),    // 10: sesame.v1alpha1.StreamResponse.Connect
	(*StreamResponse_Dial)(nil),       // 11: sesame.v1alpha1.StreamResponse.Dial
	(*grpctunnel.ServerToClient)(nil), // 12: sesame.type.ServerToClient
	(*grpctunnel.ClientToServer)(nil), // 13: sesame.type.ClientToServer
	(*status.Status)(nil),             // 14: google.rpc.Status
}
var file_sesame_v1alpha1_tunnel_proto_depIdxs = []int32{
	4,  // 0: sesame.v1alpha1.TunnelRequest.connect:type_name -> sesame.v1alpha1.TunnelRequest.Connect
	5,  // 1: sesame.v1alpha1.TunnelRequest.stream:type_name -> sesame.v1alpha1.TunnelRequest.Stream
	6,  // 2: sesame.v1alpha1.TunnelResponse.connect:type_name -> sesame.v1alpha1.TunnelResponse.Connect
	7,  // 3: sesame.v1alpha1.TunnelResponse.stream:type_name -> sesame.v1alpha1.TunnelResponse.Stream
	8,  // 4: sesame.v1alpha1.StreamRequest.connect:type_name -> sesame.v1alpha1.StreamRequest.Connect
	9,  // 5: sesame.v1alpha1.StreamRequest.dial:type_name -> sesame.v1alpha1.StreamRequest.Dial
	12, // 6: sesame.v1alpha1.StreamRequest.tunnel:type_name -> sesame.type.ServerToClient
	10, // 7: sesame.v1alpha1.StreamResponse.connect:type_name -> sesame.v1alpha1.StreamResponse.Connect
	11, // 8: sesame.v1alpha1.StreamResponse.dial:type_name -> sesame.v1alpha1.StreamResponse.Dial
	13, // 9: sesame.v1alpha1.StreamResponse.tunnel:type_name -> sesame.type.ClientToServer
	14, // 10: sesame.v1alpha1.TunnelRequest.Stream.error:type_name -> google.rpc.Status
	0,  // 11: sesame.v1alpha1.TunnelService.Tunnel:input_type -> sesame.v1alpha1.TunnelRequest
	2,  // 12: sesame.v1alpha1.TunnelService.Stream:input_type -> sesame.v1alpha1.StreamRequest
	1,  // 13: sesame.v1alpha1.TunnelService.Tunnel:output_type -> sesame.v1alpha1.TunnelResponse
	3,  // 14: sesame.v1alpha1.TunnelService.Stream:output_type -> sesame.v1alpha1.StreamResponse
	13, // [13:15] is the sub-list for method output_type
	11, // [11:13] is the sub-list for method input_type
	11, // [11:11] is the sub-list for extension type_name
	11, // [11:11] is the sub-list for extension extendee
	0,  // [0:11] is the sub-list for field type_name
}

func init() { file_sesame_v1alpha1_tunnel_proto_init() }
func file_sesame_v1alpha1_tunnel_proto_init() {
	if File_sesame_v1alpha1_tunnel_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_sesame_v1alpha1_tunnel_proto_msgTypes[0].Exporter = func(v any, i int) any {
			switch v := v.(*TunnelRequest); i {
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
		file_sesame_v1alpha1_tunnel_proto_msgTypes[1].Exporter = func(v any, i int) any {
			switch v := v.(*TunnelResponse); i {
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
		file_sesame_v1alpha1_tunnel_proto_msgTypes[2].Exporter = func(v any, i int) any {
			switch v := v.(*StreamRequest); i {
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
		file_sesame_v1alpha1_tunnel_proto_msgTypes[3].Exporter = func(v any, i int) any {
			switch v := v.(*StreamResponse); i {
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
		file_sesame_v1alpha1_tunnel_proto_msgTypes[4].Exporter = func(v any, i int) any {
			switch v := v.(*TunnelRequest_Connect); i {
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
		file_sesame_v1alpha1_tunnel_proto_msgTypes[5].Exporter = func(v any, i int) any {
			switch v := v.(*TunnelRequest_Stream); i {
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
		file_sesame_v1alpha1_tunnel_proto_msgTypes[6].Exporter = func(v any, i int) any {
			switch v := v.(*TunnelResponse_Connect); i {
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
		file_sesame_v1alpha1_tunnel_proto_msgTypes[7].Exporter = func(v any, i int) any {
			switch v := v.(*TunnelResponse_Stream); i {
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
		file_sesame_v1alpha1_tunnel_proto_msgTypes[8].Exporter = func(v any, i int) any {
			switch v := v.(*StreamRequest_Connect); i {
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
		file_sesame_v1alpha1_tunnel_proto_msgTypes[9].Exporter = func(v any, i int) any {
			switch v := v.(*StreamRequest_Dial); i {
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
		file_sesame_v1alpha1_tunnel_proto_msgTypes[10].Exporter = func(v any, i int) any {
			switch v := v.(*StreamResponse_Connect); i {
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
		file_sesame_v1alpha1_tunnel_proto_msgTypes[11].Exporter = func(v any, i int) any {
			switch v := v.(*StreamResponse_Dial); i {
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
	file_sesame_v1alpha1_tunnel_proto_msgTypes[0].OneofWrappers = []any{
		(*TunnelRequest_Connect_)(nil),
		(*TunnelRequest_Stream_)(nil),
	}
	file_sesame_v1alpha1_tunnel_proto_msgTypes[1].OneofWrappers = []any{
		(*TunnelResponse_Connect_)(nil),
		(*TunnelResponse_Stream_)(nil),
	}
	file_sesame_v1alpha1_tunnel_proto_msgTypes[2].OneofWrappers = []any{
		(*StreamRequest_Connect_)(nil),
		(*StreamRequest_Dial_)(nil),
		(*StreamRequest_Tunnel)(nil),
	}
	file_sesame_v1alpha1_tunnel_proto_msgTypes[3].OneofWrappers = []any{
		(*StreamResponse_Connect_)(nil),
		(*StreamResponse_Dial_)(nil),
		(*StreamResponse_Tunnel)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_sesame_v1alpha1_tunnel_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   12,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_sesame_v1alpha1_tunnel_proto_goTypes,
		DependencyIndexes: file_sesame_v1alpha1_tunnel_proto_depIdxs,
		MessageInfos:      file_sesame_v1alpha1_tunnel_proto_msgTypes,
	}.Build()
	File_sesame_v1alpha1_tunnel_proto = out.File
	file_sesame_v1alpha1_tunnel_proto_rawDesc = nil
	file_sesame_v1alpha1_tunnel_proto_goTypes = nil
	file_sesame_v1alpha1_tunnel_proto_depIdxs = nil
}
