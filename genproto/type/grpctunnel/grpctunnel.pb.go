// This file was originally based on
// https://github.com/jhump/grpctunnel/blob/525f1361e55b62188ee09dedceed5b12a6fdb0f3/tunnel.proto
// See the license below.

//
//Apache License
//Version 2.0, January 2004
//http://www.apache.org/licenses/
//
//TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION
//
//1. Definitions.
//
//"License" shall mean the terms and conditions for use, reproduction,
//and distribution as defined by Sections 1 through 9 of this document.
//
//"Licensor" shall mean the copyright owner or entity authorized by
//the copyright owner that is granting the License.
//
//"Legal Entity" shall mean the union of the acting entity and all
//other entities that control, are controlled by, or are under common
//control with that entity. For the purposes of this definition,
//"control" means (i) the power, direct or indirect, to cause the
//direction or management of such entity, whether by contract or
//otherwise, or (ii) ownership of fifty percent (50%) or more of the
//outstanding shares, or (iii) beneficial ownership of such entity.
//
//"You" (or "Your") shall mean an individual or Legal Entity
//exercising permissions granted by this License.
//
//"Source" form shall mean the preferred form for making modifications,
//including but not limited to software source code, documentation
//source, and configuration files.
//
//"Object" form shall mean any form resulting from mechanical
//transformation or translation of a Source form, including but
//not limited to compiled object code, generated documentation,
//and conversions to other media types.
//
//"Work" shall mean the work of authorship, whether in Source or
//Object form, made available under the License, as indicated by a
//copyright notice that is included in or attached to the work
//(an example is provided in the Appendix below).
//
//"Derivative Works" shall mean any work, whether in Source or Object
//form, that is based on (or derived from) the Work and for which the
//editorial revisions, annotations, elaborations, or other modifications
//represent, as a whole, an original work of authorship. For the purposes
//of this License, Derivative Works shall not include works that remain
//separable from, or merely link (or bind by name) to the interfaces of,
//the Work and Derivative Works thereof.
//
//"Contribution" shall mean any work of authorship, including
//the original version of the Work and any modifications or additions
//to that Work or Derivative Works thereof, that is intentionally
//submitted to Licensor for inclusion in the Work by the copyright owner
//or by an individual or Legal Entity authorized to submit on behalf of
//the copyright owner. For the purposes of this definition, "submitted"
//means any form of electronic, verbal, or written communication sent
//to the Licensor or its representatives, including but not limited to
//communication on electronic mailing lists, source code control systems,
//and issue tracking systems that are managed by, or on behalf of, the
//Licensor for the purpose of discussing and improving the Work, but
//excluding communication that is conspicuously marked or otherwise
//designated in writing by the copyright owner as "Not a Contribution."
//
//"Contributor" shall mean Licensor and any individual or Legal Entity
//on behalf of whom a Contribution has been received by Licensor and
//subsequently incorporated within the Work.
//
//2. Grant of Copyright License. Subject to the terms and conditions of
//this License, each Contributor hereby grants to You a perpetual,
//worldwide, non-exclusive, no-charge, royalty-free, irrevocable
//copyright license to reproduce, prepare Derivative Works of,
//publicly display, publicly perform, sublicense, and distribute the
//Work and such Derivative Works in Source or Object form.
//
//3. Grant of Patent License. Subject to the terms and conditions of
//this License, each Contributor hereby grants to You a perpetual,
//worldwide, non-exclusive, no-charge, royalty-free, irrevocable
//(except as stated in this section) patent license to make, have made,
//use, offer to sell, sell, import, and otherwise transfer the Work,
//where such license applies only to those patent claims licensable
//by such Contributor that are necessarily infringed by their
//Contribution(s) alone or by combination of their Contribution(s)
//with the Work to which such Contribution(s) was submitted. If You
//institute patent litigation against any entity (including a
//cross-claim or counterclaim in a lawsuit) alleging that the Work
//or a Contribution incorporated within the Work constitutes direct
//or contributory patent infringement, then any patent licenses
//granted to You under this License for that Work shall terminate
//as of the date such litigation is filed.
//
//4. Redistribution. You may reproduce and distribute copies of the
//Work or Derivative Works thereof in any medium, with or without
//modifications, and in Source or Object form, provided that You
//meet the following conditions:
//
//(a) You must give any other recipients of the Work or
//Derivative Works a copy of this License; and
//
//(b) You must cause any modified files to carry prominent notices
//stating that You changed the files; and
//
//(c) You must retain, in the Source form of any Derivative Works
//that You distribute, all copyright, patent, trademark, and
//attribution notices from the Source form of the Work,
//excluding those notices that do not pertain to any part of
//the Derivative Works; and
//
//(d) If the Work includes a "NOTICE" text file as part of its
//distribution, then any Derivative Works that You distribute must
//include a readable copy of the attribution notices contained
//within such NOTICE file, excluding those notices that do not
//pertain to any part of the Derivative Works, in at least one
//of the following places: within a NOTICE text file distributed
//as part of the Derivative Works; within the Source form or
//documentation, if provided along with the Derivative Works; or,
//within a display generated by the Derivative Works, if and
//wherever such third-party notices normally appear. The contents
//of the NOTICE file are for informational purposes only and
//do not modify the License. You may add Your own attribution
//notices within Derivative Works that You distribute, alongside
//or as an addendum to the NOTICE text from the Work, provided
//that such additional attribution notices cannot be construed
//as modifying the License.
//
//You may add Your own copyright statement to Your modifications and
//may provide additional or different license terms and conditions
//for use, reproduction, or distribution of Your modifications, or
//for any such Derivative Works as a whole, provided Your use,
//reproduction, and distribution of the Work otherwise complies with
//the conditions stated in this License.
//
//5. Submission of Contributions. Unless You explicitly state otherwise,
//any Contribution intentionally submitted for inclusion in the Work
//by You to the Licensor shall be under the terms and conditions of
//this License, without any additional terms or conditions.
//Notwithstanding the above, nothing herein shall supersede or modify
//the terms of any separate license agreement you may have executed
//with Licensor regarding such Contributions.
//
//6. Trademarks. This License does not grant permission to use the trade
//names, trademarks, service marks, or product names of the Licensor,
//except as required for reasonable and customary use in describing the
//origin of the Work and reproducing the content of the NOTICE file.
//
//7. Disclaimer of Warranty. Unless required by applicable law or
//agreed to in writing, Licensor provides the Work (and each
//Contributor provides its Contributions) on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
//implied, including, without limitation, any warranties or conditions
//of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A
//PARTICULAR PURPOSE. You are solely responsible for determining the
//appropriateness of using or redistributing the Work and assume any
//risks associated with Your exercise of permissions under this License.
//
//8. Limitation of Liability. In no event and under no legal theory,
//whether in tort (including negligence), contract, or otherwise,
//unless required by applicable law (such as deliberate and grossly
//negligent acts) or agreed to in writing, shall any Contributor be
//liable to You for damages, including any direct, indirect, special,
//incidental, or consequential damages of any character arising as a
//result of this License or out of the use or inability to use the
//Work (including but not limited to damages for loss of goodwill,
//work stoppage, computer failure or malfunction, or any and all
//other commercial damages or losses), even if such Contributor
//has been advised of the possibility of such damages.
//
//9. Accepting Warranty or Additional Liability. While redistributing
//the Work or Derivative Works thereof, You may choose to offer,
//and charge a fee for, acceptance of support, warranty, indemnity,
//or other liability obligations and/or rights consistent with this
//License. However, in accepting such obligations, You may act only
//on Your own behalf and on Your sole responsibility, not on behalf
//of any other Contributor, and only if You agree to indemnify,
//defend, and hold each Contributor harmless for any liability
//incurred by, or claims asserted against, such Contributor by reason
//of your accepting any such warranty or additional liability.
//
//END OF TERMS AND CONDITIONS
//
//APPENDIX: How to apply the Apache License to your work.
//
//To apply the Apache License to your work, attach the following
//boilerplate notice, with the fields enclosed by brackets "[]"
//replaced with your own identifying information. (Don't include
//the brackets!)  The text should be enclosed in the appropriate
//comment syntax for the file format. We also recommend that a
//file or class name and description of purpose be included on the
//same "printed page" as the copyright notice for easier
//identification within third-party archives.
//
//Copyright 2018 Joshua Humphries
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.0
// 	protoc        v3.14.0
// source: sesame/type/grpctunnel.proto

package grpctunnel

import (
	grpcmetadata "github.com/joeycumines/sesame/genproto/type/grpcmetadata"
	status "google.golang.org/genproto/googleapis/rpc/status"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// ClientToServer is the message a client sends to a server.
//
// For a single stream ID, the first such message must include the new_stream
// field. After that, there can be any number of requests sent, via the
// request_message field and additional messages thereafter that use the
// message_data field (for requests that are larger than 16kb). And
// finally, the RPC ends with either the half_close or cancel fields. If the
// half_close field is used, the RPC stream remains active so the server may
// continue to send response data. But, if the cancel field is used, the RPC
// stream is aborted and thus closed on both client and server ends. If a stream
// has been half-closed, the only allowed message from the client for that
// stream ID is one with the cancel field, to abort the remainder of the
// operation.
type ClientToServer struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The ID of the stream. Stream IDs must be used in increasing order and
	// cannot be re-used.
	StreamId uint64 `protobuf:"varint,1,opt,name=stream_id,json=streamId,proto3" json:"stream_id,omitempty"`
	// Types that are assignable to Frame:
	//	*ClientToServer_WindowUpdate
	//	*ClientToServer_NewStream_
	//	*ClientToServer_Message
	//	*ClientToServer_MessageData
	//	*ClientToServer_HalfClose
	//	*ClientToServer_Cancel
	Frame isClientToServer_Frame `protobuf_oneof:"frame"`
}

func (x *ClientToServer) Reset() {
	*x = ClientToServer{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_type_grpctunnel_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ClientToServer) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClientToServer) ProtoMessage() {}

func (x *ClientToServer) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_type_grpctunnel_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClientToServer.ProtoReflect.Descriptor instead.
func (*ClientToServer) Descriptor() ([]byte, []int) {
	return file_sesame_type_grpctunnel_proto_rawDescGZIP(), []int{0}
}

func (x *ClientToServer) GetStreamId() uint64 {
	if x != nil {
		return x.StreamId
	}
	return 0
}

func (m *ClientToServer) GetFrame() isClientToServer_Frame {
	if m != nil {
		return m.Frame
	}
	return nil
}

func (x *ClientToServer) GetWindowUpdate() uint32 {
	if x, ok := x.GetFrame().(*ClientToServer_WindowUpdate); ok {
		return x.WindowUpdate
	}
	return 0
}

func (x *ClientToServer) GetNewStream() *ClientToServer_NewStream {
	if x, ok := x.GetFrame().(*ClientToServer_NewStream_); ok {
		return x.NewStream
	}
	return nil
}

func (x *ClientToServer) GetMessage() *EncodedMessage {
	if x, ok := x.GetFrame().(*ClientToServer_Message); ok {
		return x.Message
	}
	return nil
}

func (x *ClientToServer) GetMessageData() []byte {
	if x, ok := x.GetFrame().(*ClientToServer_MessageData); ok {
		return x.MessageData
	}
	return nil
}

func (x *ClientToServer) GetHalfClose() *emptypb.Empty {
	if x, ok := x.GetFrame().(*ClientToServer_HalfClose); ok {
		return x.HalfClose
	}
	return nil
}

func (x *ClientToServer) GetCancel() *emptypb.Empty {
	if x, ok := x.GetFrame().(*ClientToServer_Cancel); ok {
		return x.Cancel
	}
	return nil
}

type isClientToServer_Frame interface {
	isClientToServer_Frame()
}

type ClientToServer_WindowUpdate struct {
	// Per-stream flow control message, based on HTTP/2.
	// Note that only message/message_data frames are subject to flow control.
	// https://datatracker.ietf.org/doc/html/rfc7540#section-6.9
	WindowUpdate uint32 `protobuf:"varint,2,opt,name=window_update,json=windowUpdate,proto3,oneof"`
}

type ClientToServer_NewStream_ struct {
	// Creates a new RPC stream, which includes request header metadata. The
	// stream ID must not be an already active stream.
	NewStream *ClientToServer_NewStream `protobuf:"bytes,3,opt,name=new_stream,json=newStream,proto3,oneof"`
}

type ClientToServer_Message struct {
	// Sends a message on the RPC stream. If the message is larger than 16k,
	// the rest of the message should be sent in chunks using the
	// message_data field (up to 16kb of data in each chunk).
	Message *EncodedMessage `protobuf:"bytes,4,opt,name=message,proto3,oneof"`
}

type ClientToServer_MessageData struct {
	// Sends a chunk of request data, for a request message that could not
	// wholly fit in a request_message field (e.g. > 16kb).
	MessageData []byte `protobuf:"bytes,5,opt,name=message_data,json=messageData,proto3,oneof"`
}

type ClientToServer_HalfClose struct {
	// Half-closes the stream, signaling that no more request messages will
	// be sent. No other messages, other than one with the cancel field set,
	// should be sent for this stream.
	HalfClose *emptypb.Empty `protobuf:"bytes,6,opt,name=half_close,json=halfClose,proto3,oneof"`
}

type ClientToServer_Cancel struct {
	// Aborts the stream. No other messages should be sent for this stream.
	Cancel *emptypb.Empty `protobuf:"bytes,7,opt,name=cancel,proto3,oneof"`
}

func (*ClientToServer_WindowUpdate) isClientToServer_Frame() {}

func (*ClientToServer_NewStream_) isClientToServer_Frame() {}

func (*ClientToServer_Message) isClientToServer_Frame() {}

func (*ClientToServer_MessageData) isClientToServer_Frame() {}

func (*ClientToServer_HalfClose) isClientToServer_Frame() {}

func (*ClientToServer_Cancel) isClientToServer_Frame() {}

// ServerToClient is the message a server sends to a client.
//
// For a single stream ID, the first such message should include the
// response_headers field unless no headers are to be sent. After the headers,
// the server can send any number of responses, via the response_message field
// and additional messages thereafter that use the message_data field (for
// responses that are larger than 16kb). A message with the close_stream field
// concludes the stream, whether it terminates successfully or with an error.
type ServerToClient struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The ID of the stream. Stream IDs are defined by the client and should be
	// used in monotonically increasing order. They cannot be re-used.
	StreamId uint64 `protobuf:"varint,1,opt,name=stream_id,json=streamId,proto3" json:"stream_id,omitempty"`
	// Types that are assignable to Frame:
	//	*ServerToClient_WindowUpdate
	//	*ServerToClient_Header
	//	*ServerToClient_Message
	//	*ServerToClient_MessageData
	//	*ServerToClient_CloseStream_
	Frame isServerToClient_Frame `protobuf_oneof:"frame"`
}

func (x *ServerToClient) Reset() {
	*x = ServerToClient{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_type_grpctunnel_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ServerToClient) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ServerToClient) ProtoMessage() {}

func (x *ServerToClient) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_type_grpctunnel_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ServerToClient.ProtoReflect.Descriptor instead.
func (*ServerToClient) Descriptor() ([]byte, []int) {
	return file_sesame_type_grpctunnel_proto_rawDescGZIP(), []int{1}
}

func (x *ServerToClient) GetStreamId() uint64 {
	if x != nil {
		return x.StreamId
	}
	return 0
}

func (m *ServerToClient) GetFrame() isServerToClient_Frame {
	if m != nil {
		return m.Frame
	}
	return nil
}

func (x *ServerToClient) GetWindowUpdate() uint32 {
	if x, ok := x.GetFrame().(*ServerToClient_WindowUpdate); ok {
		return x.WindowUpdate
	}
	return 0
}

func (x *ServerToClient) GetHeader() *grpcmetadata.GrpcMetadata {
	if x, ok := x.GetFrame().(*ServerToClient_Header); ok {
		return x.Header
	}
	return nil
}

func (x *ServerToClient) GetMessage() *EncodedMessage {
	if x, ok := x.GetFrame().(*ServerToClient_Message); ok {
		return x.Message
	}
	return nil
}

func (x *ServerToClient) GetMessageData() []byte {
	if x, ok := x.GetFrame().(*ServerToClient_MessageData); ok {
		return x.MessageData
	}
	return nil
}

func (x *ServerToClient) GetCloseStream() *ServerToClient_CloseStream {
	if x, ok := x.GetFrame().(*ServerToClient_CloseStream_); ok {
		return x.CloseStream
	}
	return nil
}

type isServerToClient_Frame interface {
	isServerToClient_Frame()
}

type ServerToClient_WindowUpdate struct {
	// Per-stream flow control message, based on HTTP/2.
	// Note that only message/message_data frames are subject to flow control.
	// https://datatracker.ietf.org/doc/html/rfc7540#section-6.9
	WindowUpdate uint32 `protobuf:"varint,2,opt,name=window_update,json=windowUpdate,proto3,oneof"`
}

type ServerToClient_Header struct {
	// Sends response headers for this stream. If headers are sent at all,
	// they must be sent before any response message data.
	Header *grpcmetadata.GrpcMetadata `protobuf:"bytes,3,opt,name=header,proto3,oneof"`
}

type ServerToClient_Message struct {
	// Sends a message on the RPC stream. If the message is larger than 16k,
	// the rest of the message should be sent in chunks using the
	// message_data field (up to 16kb of data in each chunk).
	Message *EncodedMessage `protobuf:"bytes,4,opt,name=message,proto3,oneof"`
}

type ServerToClient_MessageData struct {
	// Sends a chunk of response data, for a response message that could not
	// wholly fit in a response_message field (e.g. > 16kb).
	MessageData []byte `protobuf:"bytes,5,opt,name=message_data,json=messageData,proto3,oneof"`
}

type ServerToClient_CloseStream_ struct {
	// Terminates the stream and communicates the final disposition to the
	// client. After the stream is closed, no other messages should use the
	// given stream ID.
	CloseStream *ServerToClient_CloseStream `protobuf:"bytes,6,opt,name=close_stream,json=closeStream,proto3,oneof"`
}

func (*ServerToClient_WindowUpdate) isServerToClient_Frame() {}

func (*ServerToClient_Header) isServerToClient_Frame() {}

func (*ServerToClient_Message) isServerToClient_Frame() {}

func (*ServerToClient_MessageData) isServerToClient_Frame() {}

func (*ServerToClient_CloseStream_) isServerToClient_Frame() {}

// EncodedMessage models a binary gRPC message, and is used to frame tunneled messages.
type EncodedMessage struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The full size of the message.
	Size int32 `protobuf:"varint,1,opt,name=size,proto3" json:"size,omitempty"`
	// The message data. This field should not be longer than 16kb (16,384
	// bytes). If the full size of the message is larger then it should be
	// split into multiple chunks. The chunking is done to allow multiple
	// access to the underlying gRPC stream by concurrent tunneled streams.
	// If very large messages were sent via a single chunk, it could cause
	// head-of-line blocking and starvation when multiple streams need to send
	// data on the one underlying gRPC stream.
	Data []byte `protobuf:"bytes,2,opt,name=data,proto3" json:"data,omitempty"`
}

func (x *EncodedMessage) Reset() {
	*x = EncodedMessage{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_type_grpctunnel_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *EncodedMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*EncodedMessage) ProtoMessage() {}

func (x *EncodedMessage) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_type_grpctunnel_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use EncodedMessage.ProtoReflect.Descriptor instead.
func (*EncodedMessage) Descriptor() ([]byte, []int) {
	return file_sesame_type_grpctunnel_proto_rawDescGZIP(), []int{2}
}

func (x *EncodedMessage) GetSize() int32 {
	if x != nil {
		return x.Size
	}
	return 0
}

func (x *EncodedMessage) GetData() []byte {
	if x != nil {
		return x.Data
	}
	return nil
}

type ClientToServer_NewStream struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Method string                     `protobuf:"bytes,1,opt,name=method,proto3" json:"method,omitempty"`
	Header *grpcmetadata.GrpcMetadata `protobuf:"bytes,2,opt,name=header,proto3" json:"header,omitempty"`
}

func (x *ClientToServer_NewStream) Reset() {
	*x = ClientToServer_NewStream{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_type_grpctunnel_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ClientToServer_NewStream) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClientToServer_NewStream) ProtoMessage() {}

func (x *ClientToServer_NewStream) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_type_grpctunnel_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClientToServer_NewStream.ProtoReflect.Descriptor instead.
func (*ClientToServer_NewStream) Descriptor() ([]byte, []int) {
	return file_sesame_type_grpctunnel_proto_rawDescGZIP(), []int{0, 0}
}

func (x *ClientToServer_NewStream) GetMethod() string {
	if x != nil {
		return x.Method
	}
	return ""
}

func (x *ClientToServer_NewStream) GetHeader() *grpcmetadata.GrpcMetadata {
	if x != nil {
		return x.Header
	}
	return nil
}

type ServerToClient_CloseStream struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Trailer *grpcmetadata.GrpcMetadata `protobuf:"bytes,1,opt,name=trailer,proto3" json:"trailer,omitempty"`
	Status  *status.Status             `protobuf:"bytes,2,opt,name=status,proto3" json:"status,omitempty"`
}

func (x *ServerToClient_CloseStream) Reset() {
	*x = ServerToClient_CloseStream{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sesame_type_grpctunnel_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ServerToClient_CloseStream) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ServerToClient_CloseStream) ProtoMessage() {}

func (x *ServerToClient_CloseStream) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_type_grpctunnel_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ServerToClient_CloseStream.ProtoReflect.Descriptor instead.
func (*ServerToClient_CloseStream) Descriptor() ([]byte, []int) {
	return file_sesame_type_grpctunnel_proto_rawDescGZIP(), []int{1, 0}
}

func (x *ServerToClient_CloseStream) GetTrailer() *grpcmetadata.GrpcMetadata {
	if x != nil {
		return x.Trailer
	}
	return nil
}

func (x *ServerToClient_CloseStream) GetStatus() *status.Status {
	if x != nil {
		return x.Status
	}
	return nil
}

var File_sesame_type_grpctunnel_proto protoreflect.FileDescriptor

var file_sesame_type_grpctunnel_proto_rawDesc = []byte{
	0x0a, 0x1c, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2f, 0x74, 0x79, 0x70, 0x65, 0x2f, 0x67, 0x72,
	0x70, 0x63, 0x74, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0b,
	0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x74, 0x79, 0x70, 0x65, 0x1a, 0x1b, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70,
	0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x17, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2f, 0x72, 0x70, 0x63, 0x2f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x1a, 0x1e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2f, 0x74, 0x79, 0x70, 0x65, 0x2f, 0x67,
	0x72, 0x70, 0x63, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x22, 0xc6, 0x03, 0x0a, 0x0e, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x54, 0x6f, 0x53, 0x65,
	0x72, 0x76, 0x65, 0x72, 0x12, 0x1b, 0x0a, 0x09, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x5f, 0x69,
	0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x04, 0x52, 0x08, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x49,
	0x64, 0x12, 0x25, 0x0a, 0x0d, 0x77, 0x69, 0x6e, 0x64, 0x6f, 0x77, 0x5f, 0x75, 0x70, 0x64, 0x61,
	0x74, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x48, 0x00, 0x52, 0x0c, 0x77, 0x69, 0x6e, 0x64,
	0x6f, 0x77, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x12, 0x46, 0x0a, 0x0a, 0x6e, 0x65, 0x77, 0x5f,
	0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x25, 0x2e, 0x73,
	0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x74, 0x79, 0x70, 0x65, 0x2e, 0x43, 0x6c, 0x69, 0x65, 0x6e,
	0x74, 0x54, 0x6f, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x4e, 0x65, 0x77, 0x53, 0x74, 0x72,
	0x65, 0x61, 0x6d, 0x48, 0x00, 0x52, 0x09, 0x6e, 0x65, 0x77, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d,
	0x12, 0x37, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x1b, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x74, 0x79, 0x70, 0x65, 0x2e,
	0x45, 0x6e, 0x63, 0x6f, 0x64, 0x65, 0x64, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x48, 0x00,
	0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x23, 0x0a, 0x0c, 0x6d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x5f, 0x64, 0x61, 0x74, 0x61, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0c, 0x48,
	0x00, 0x52, 0x0b, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x44, 0x61, 0x74, 0x61, 0x12, 0x37,
	0x0a, 0x0a, 0x68, 0x61, 0x6c, 0x66, 0x5f, 0x63, 0x6c, 0x6f, 0x73, 0x65, 0x18, 0x06, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x48, 0x00, 0x52, 0x09, 0x68, 0x61,
	0x6c, 0x66, 0x43, 0x6c, 0x6f, 0x73, 0x65, 0x12, 0x30, 0x0a, 0x06, 0x63, 0x61, 0x6e, 0x63, 0x65,
	0x6c, 0x18, 0x07, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x48,
	0x00, 0x52, 0x06, 0x63, 0x61, 0x6e, 0x63, 0x65, 0x6c, 0x1a, 0x56, 0x0a, 0x09, 0x4e, 0x65, 0x77,
	0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x12, 0x16, 0x0a, 0x06, 0x6d, 0x65, 0x74, 0x68, 0x6f, 0x64,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x6d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x12, 0x31,
	0x0a, 0x06, 0x68, 0x65, 0x61, 0x64, 0x65, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19,
	0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x74, 0x79, 0x70, 0x65, 0x2e, 0x47, 0x72, 0x70,
	0x63, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x52, 0x06, 0x68, 0x65, 0x61, 0x64, 0x65,
	0x72, 0x42, 0x07, 0x0a, 0x05, 0x66, 0x72, 0x61, 0x6d, 0x65, 0x22, 0xae, 0x03, 0x0a, 0x0e, 0x53,
	0x65, 0x72, 0x76, 0x65, 0x72, 0x54, 0x6f, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x12, 0x1b, 0x0a,
	0x09, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x04,
	0x52, 0x08, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x49, 0x64, 0x12, 0x25, 0x0a, 0x0d, 0x77, 0x69,
	0x6e, 0x64, 0x6f, 0x77, 0x5f, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x0d, 0x48, 0x00, 0x52, 0x0c, 0x77, 0x69, 0x6e, 0x64, 0x6f, 0x77, 0x55, 0x70, 0x64, 0x61, 0x74,
	0x65, 0x12, 0x33, 0x0a, 0x06, 0x68, 0x65, 0x61, 0x64, 0x65, 0x72, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x19, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x74, 0x79, 0x70, 0x65, 0x2e,
	0x47, 0x72, 0x70, 0x63, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x48, 0x00, 0x52, 0x06,
	0x68, 0x65, 0x61, 0x64, 0x65, 0x72, 0x12, 0x37, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67,
	0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65,
	0x2e, 0x74, 0x79, 0x70, 0x65, 0x2e, 0x45, 0x6e, 0x63, 0x6f, 0x64, 0x65, 0x64, 0x4d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x48, 0x00, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12,
	0x23, 0x0a, 0x0c, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x5f, 0x64, 0x61, 0x74, 0x61, 0x18,
	0x05, 0x20, 0x01, 0x28, 0x0c, 0x48, 0x00, 0x52, 0x0b, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65,
	0x44, 0x61, 0x74, 0x61, 0x12, 0x4c, 0x0a, 0x0c, 0x63, 0x6c, 0x6f, 0x73, 0x65, 0x5f, 0x73, 0x74,
	0x72, 0x65, 0x61, 0x6d, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x27, 0x2e, 0x73, 0x65, 0x73,
	0x61, 0x6d, 0x65, 0x2e, 0x74, 0x79, 0x70, 0x65, 0x2e, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x54,
	0x6f, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x2e, 0x43, 0x6c, 0x6f, 0x73, 0x65, 0x53, 0x74, 0x72,
	0x65, 0x61, 0x6d, 0x48, 0x00, 0x52, 0x0b, 0x63, 0x6c, 0x6f, 0x73, 0x65, 0x53, 0x74, 0x72, 0x65,
	0x61, 0x6d, 0x1a, 0x6e, 0x0a, 0x0b, 0x43, 0x6c, 0x6f, 0x73, 0x65, 0x53, 0x74, 0x72, 0x65, 0x61,
	0x6d, 0x12, 0x33, 0x0a, 0x07, 0x74, 0x72, 0x61, 0x69, 0x6c, 0x65, 0x72, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x19, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x74, 0x79, 0x70, 0x65,
	0x2e, 0x47, 0x72, 0x70, 0x63, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x52, 0x07, 0x74,
	0x72, 0x61, 0x69, 0x6c, 0x65, 0x72, 0x12, 0x2a, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x72, 0x70, 0x63, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x42, 0x07, 0x0a, 0x05, 0x66, 0x72, 0x61, 0x6d, 0x65, 0x22, 0x38, 0x0a, 0x0e, 0x45,
	0x6e, 0x63, 0x6f, 0x64, 0x65, 0x64, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x12, 0x0a,
	0x04, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x04, 0x73, 0x69, 0x7a,
	0x65, 0x12, 0x12, 0x0a, 0x04, 0x64, 0x61, 0x74, 0x61, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x04, 0x64, 0x61, 0x74, 0x61, 0x32, 0xae, 0x01, 0x0a, 0x0d, 0x54, 0x75, 0x6e, 0x6e, 0x65, 0x6c,
	0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x4a, 0x0a, 0x0a, 0x4f, 0x70, 0x65, 0x6e, 0x54,
	0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x12, 0x1b, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x74,
	0x79, 0x70, 0x65, 0x2e, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x54, 0x6f, 0x53, 0x65, 0x72, 0x76,
	0x65, 0x72, 0x1a, 0x1b, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x74, 0x79, 0x70, 0x65,
	0x2e, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x54, 0x6f, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x28,
	0x01, 0x30, 0x01, 0x12, 0x51, 0x0a, 0x11, 0x4f, 0x70, 0x65, 0x6e, 0x52, 0x65, 0x76, 0x65, 0x72,
	0x73, 0x65, 0x54, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x12, 0x1b, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d,
	0x65, 0x2e, 0x74, 0x79, 0x70, 0x65, 0x2e, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x54, 0x6f, 0x43,
	0x6c, 0x69, 0x65, 0x6e, 0x74, 0x1a, 0x1b, 0x2e, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2e, 0x74,
	0x79, 0x70, 0x65, 0x2e, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x54, 0x6f, 0x53, 0x65, 0x72, 0x76,
	0x65, 0x72, 0x28, 0x01, 0x30, 0x01, 0x42, 0x38, 0x5a, 0x36, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6a, 0x6f, 0x65, 0x79, 0x63, 0x75, 0x6d, 0x69, 0x6e, 0x65, 0x73,
	0x2f, 0x73, 0x65, 0x73, 0x61, 0x6d, 0x65, 0x2f, 0x67, 0x65, 0x6e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2f, 0x74, 0x79, 0x70, 0x65, 0x2f, 0x67, 0x72, 0x70, 0x63, 0x74, 0x75, 0x6e, 0x6e, 0x65, 0x6c,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_sesame_type_grpctunnel_proto_rawDescOnce sync.Once
	file_sesame_type_grpctunnel_proto_rawDescData = file_sesame_type_grpctunnel_proto_rawDesc
)

func file_sesame_type_grpctunnel_proto_rawDescGZIP() []byte {
	file_sesame_type_grpctunnel_proto_rawDescOnce.Do(func() {
		file_sesame_type_grpctunnel_proto_rawDescData = protoimpl.X.CompressGZIP(file_sesame_type_grpctunnel_proto_rawDescData)
	})
	return file_sesame_type_grpctunnel_proto_rawDescData
}

var file_sesame_type_grpctunnel_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_sesame_type_grpctunnel_proto_goTypes = []interface{}{
	(*ClientToServer)(nil),             // 0: sesame.type.ClientToServer
	(*ServerToClient)(nil),             // 1: sesame.type.ServerToClient
	(*EncodedMessage)(nil),             // 2: sesame.type.EncodedMessage
	(*ClientToServer_NewStream)(nil),   // 3: sesame.type.ClientToServer.NewStream
	(*ServerToClient_CloseStream)(nil), // 4: sesame.type.ServerToClient.CloseStream
	(*emptypb.Empty)(nil),              // 5: google.protobuf.Empty
	(*grpcmetadata.GrpcMetadata)(nil),  // 6: sesame.type.GrpcMetadata
	(*status.Status)(nil),              // 7: google.rpc.Status
}
var file_sesame_type_grpctunnel_proto_depIdxs = []int32{
	3,  // 0: sesame.type.ClientToServer.new_stream:type_name -> sesame.type.ClientToServer.NewStream
	2,  // 1: sesame.type.ClientToServer.message:type_name -> sesame.type.EncodedMessage
	5,  // 2: sesame.type.ClientToServer.half_close:type_name -> google.protobuf.Empty
	5,  // 3: sesame.type.ClientToServer.cancel:type_name -> google.protobuf.Empty
	6,  // 4: sesame.type.ServerToClient.header:type_name -> sesame.type.GrpcMetadata
	2,  // 5: sesame.type.ServerToClient.message:type_name -> sesame.type.EncodedMessage
	4,  // 6: sesame.type.ServerToClient.close_stream:type_name -> sesame.type.ServerToClient.CloseStream
	6,  // 7: sesame.type.ClientToServer.NewStream.header:type_name -> sesame.type.GrpcMetadata
	6,  // 8: sesame.type.ServerToClient.CloseStream.trailer:type_name -> sesame.type.GrpcMetadata
	7,  // 9: sesame.type.ServerToClient.CloseStream.status:type_name -> google.rpc.Status
	0,  // 10: sesame.type.TunnelService.OpenTunnel:input_type -> sesame.type.ClientToServer
	1,  // 11: sesame.type.TunnelService.OpenReverseTunnel:input_type -> sesame.type.ServerToClient
	1,  // 12: sesame.type.TunnelService.OpenTunnel:output_type -> sesame.type.ServerToClient
	0,  // 13: sesame.type.TunnelService.OpenReverseTunnel:output_type -> sesame.type.ClientToServer
	12, // [12:14] is the sub-list for method output_type
	10, // [10:12] is the sub-list for method input_type
	10, // [10:10] is the sub-list for extension type_name
	10, // [10:10] is the sub-list for extension extendee
	0,  // [0:10] is the sub-list for field type_name
}

func init() { file_sesame_type_grpctunnel_proto_init() }
func file_sesame_type_grpctunnel_proto_init() {
	if File_sesame_type_grpctunnel_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_sesame_type_grpctunnel_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ClientToServer); i {
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
		file_sesame_type_grpctunnel_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ServerToClient); i {
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
		file_sesame_type_grpctunnel_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*EncodedMessage); i {
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
		file_sesame_type_grpctunnel_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ClientToServer_NewStream); i {
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
		file_sesame_type_grpctunnel_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ServerToClient_CloseStream); i {
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
	file_sesame_type_grpctunnel_proto_msgTypes[0].OneofWrappers = []interface{}{
		(*ClientToServer_WindowUpdate)(nil),
		(*ClientToServer_NewStream_)(nil),
		(*ClientToServer_Message)(nil),
		(*ClientToServer_MessageData)(nil),
		(*ClientToServer_HalfClose)(nil),
		(*ClientToServer_Cancel)(nil),
	}
	file_sesame_type_grpctunnel_proto_msgTypes[1].OneofWrappers = []interface{}{
		(*ServerToClient_WindowUpdate)(nil),
		(*ServerToClient_Header)(nil),
		(*ServerToClient_Message)(nil),
		(*ServerToClient_MessageData)(nil),
		(*ServerToClient_CloseStream_)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_sesame_type_grpctunnel_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_sesame_type_grpctunnel_proto_goTypes,
		DependencyIndexes: file_sesame_type_grpctunnel_proto_depIdxs,
		MessageInfos:      file_sesame_type_grpctunnel_proto_msgTypes,
	}.Build()
	File_sesame_type_grpctunnel_proto = out.File
	file_sesame_type_grpctunnel_proto_rawDesc = nil
	file_sesame_type_grpctunnel_proto_goTypes = nil
	file_sesame_type_grpctunnel_proto_depIdxs = nil
}
