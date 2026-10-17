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
// 	protoc-gen-go v1.36.12
// 	protoc        v7.36.2
// source: sesame/v1alpha1/grpctunnel.proto

package sesamepb

import (
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"

	typepb "github.com/joeycumines/sesame/typepb"
	_ "google.golang.org/genproto/googleapis/api/annotations"
	status "google.golang.org/genproto/googleapis/rpc/status"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// OpenTunnelRequest is the frame message a client sends on the
// GrpcTunnelService.OpenTunnel stream.
//
// For a single stream ID, the first such message must include the new_stream
// field. After that, there can be any number of requests sent, via the
// message field and additional messages thereafter that use the
// message_data field (for requests that are larger than 16kb). And
// finally, the RPC ends with either the half_close or cancel fields. If the
// half_close field is used, the RPC stream remains active so the server may
// continue to send response data. But, if the cancel field is used, the RPC
// stream is aborted and thus closed on both client and server ends. If a stream
// has been half-closed, the only allowed message from the client for that
// stream ID is one with the cancel field, to abort the remainder of the
// operation.
type OpenTunnelRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The ID of the stream. Stream IDs must be used in increasing order and
	// cannot be re-used.
	StreamId int64 `protobuf:"varint,1,opt,name=stream_id,json=streamId,proto3" json:"stream_id,omitempty"`
	// Types that are valid to be assigned to Frame:
	//
	//	*OpenTunnelRequest_WindowUpdate
	//	*OpenTunnelRequest_NewStream
	//	*OpenTunnelRequest_Message
	//	*OpenTunnelRequest_MessageData
	//	*OpenTunnelRequest_HalfClose
	//	*OpenTunnelRequest_Cancel
	Frame         isOpenTunnelRequest_Frame `protobuf_oneof:"frame"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenTunnelRequest) Reset() {
	*x = OpenTunnelRequest{}
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenTunnelRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenTunnelRequest) ProtoMessage() {}

func (x *OpenTunnelRequest) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenTunnelRequest.ProtoReflect.Descriptor instead.
func (*OpenTunnelRequest) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_grpctunnel_proto_rawDescGZIP(), []int{0}
}

func (x *OpenTunnelRequest) GetStreamId() int64 {
	if x != nil {
		return x.StreamId
	}
	return 0
}

func (x *OpenTunnelRequest) GetFrame() isOpenTunnelRequest_Frame {
	if x != nil {
		return x.Frame
	}
	return nil
}

func (x *OpenTunnelRequest) GetWindowUpdate() int32 {
	if x != nil {
		if x, ok := x.Frame.(*OpenTunnelRequest_WindowUpdate); ok {
			return x.WindowUpdate
		}
	}
	return 0
}

func (x *OpenTunnelRequest) GetNewStream() *NewStream {
	if x != nil {
		if x, ok := x.Frame.(*OpenTunnelRequest_NewStream); ok {
			return x.NewStream
		}
	}
	return nil
}

func (x *OpenTunnelRequest) GetMessage() *EncodedMessage {
	if x != nil {
		if x, ok := x.Frame.(*OpenTunnelRequest_Message); ok {
			return x.Message
		}
	}
	return nil
}

func (x *OpenTunnelRequest) GetMessageData() []byte {
	if x != nil {
		if x, ok := x.Frame.(*OpenTunnelRequest_MessageData); ok {
			return x.MessageData
		}
	}
	return nil
}

func (x *OpenTunnelRequest) GetHalfClose() *emptypb.Empty {
	if x != nil {
		if x, ok := x.Frame.(*OpenTunnelRequest_HalfClose); ok {
			return x.HalfClose
		}
	}
	return nil
}

func (x *OpenTunnelRequest) GetCancel() *emptypb.Empty {
	if x != nil {
		if x, ok := x.Frame.(*OpenTunnelRequest_Cancel); ok {
			return x.Cancel
		}
	}
	return nil
}

type isOpenTunnelRequest_Frame interface {
	isOpenTunnelRequest_Frame()
}

type OpenTunnelRequest_WindowUpdate struct {
	// Per-stream flow control message, based on HTTP/2.
	// Note that only message/message_data frames are subject to flow control.
	// https://datatracker.ietf.org/doc/html/rfc7540#section-6.9
	WindowUpdate int32 `protobuf:"varint,2,opt,name=window_update,json=windowUpdate,proto3,oneof"`
}

type OpenTunnelRequest_NewStream struct {
	// Creates a new RPC stream, which includes request header metadata. The
	// stream ID must not be an already active stream.
	NewStream *NewStream `protobuf:"bytes,3,opt,name=new_stream,json=newStream,proto3,oneof"`
}

type OpenTunnelRequest_Message struct {
	// Sends a message on the RPC stream. If the message is larger than 16k,
	// the rest of the message should be sent in chunks using the
	// message_data field (up to 16kb of data in each chunk).
	Message *EncodedMessage `protobuf:"bytes,4,opt,name=message,proto3,oneof"`
}

type OpenTunnelRequest_MessageData struct {
	// Sends a chunk of request data, for a request message that could not
	// wholly fit in a message field (e.g. > 16kb).
	MessageData []byte `protobuf:"bytes,5,opt,name=message_data,json=messageData,proto3,oneof"`
}

type OpenTunnelRequest_HalfClose struct {
	// Half-closes the stream, signaling that no more request messages will
	// be sent. No other messages, other than one with the cancel field set,
	// should be sent for this stream.
	HalfClose *emptypb.Empty `protobuf:"bytes,6,opt,name=half_close,json=halfClose,proto3,oneof"`
}

type OpenTunnelRequest_Cancel struct {
	// Aborts the stream. No other messages should be sent for this stream.
	Cancel *emptypb.Empty `protobuf:"bytes,7,opt,name=cancel,proto3,oneof"`
}

func (*OpenTunnelRequest_WindowUpdate) isOpenTunnelRequest_Frame() {}

func (*OpenTunnelRequest_NewStream) isOpenTunnelRequest_Frame() {}

func (*OpenTunnelRequest_Message) isOpenTunnelRequest_Frame() {}

func (*OpenTunnelRequest_MessageData) isOpenTunnelRequest_Frame() {}

func (*OpenTunnelRequest_HalfClose) isOpenTunnelRequest_Frame() {}

func (*OpenTunnelRequest_Cancel) isOpenTunnelRequest_Frame() {}

// OpenTunnelResponse is the frame message a server sends on the
// GrpcTunnelService.OpenTunnel stream.
//
// For a single stream ID, the first such message should include the
// header field unless no headers are to be sent. After the headers,
// the server can send any number of responses, via the message field
// and additional messages thereafter that use the message_data field (for
// responses that are larger than 16kb). A message with the close_stream field
// concludes the stream, whether it terminates successfully or with an error.
type OpenTunnelResponse struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The ID of the stream. Stream IDs are defined by the client and should be
	// used in monotonically increasing order. They cannot be re-used.
	StreamId int64 `protobuf:"varint,1,opt,name=stream_id,json=streamId,proto3" json:"stream_id,omitempty"`
	// Types that are valid to be assigned to Frame:
	//
	//	*OpenTunnelResponse_WindowUpdate
	//	*OpenTunnelResponse_Header
	//	*OpenTunnelResponse_Message
	//	*OpenTunnelResponse_MessageData
	//	*OpenTunnelResponse_CloseStream
	Frame         isOpenTunnelResponse_Frame `protobuf_oneof:"frame"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenTunnelResponse) Reset() {
	*x = OpenTunnelResponse{}
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenTunnelResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenTunnelResponse) ProtoMessage() {}

func (x *OpenTunnelResponse) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenTunnelResponse.ProtoReflect.Descriptor instead.
func (*OpenTunnelResponse) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_grpctunnel_proto_rawDescGZIP(), []int{1}
}

func (x *OpenTunnelResponse) GetStreamId() int64 {
	if x != nil {
		return x.StreamId
	}
	return 0
}

func (x *OpenTunnelResponse) GetFrame() isOpenTunnelResponse_Frame {
	if x != nil {
		return x.Frame
	}
	return nil
}

func (x *OpenTunnelResponse) GetWindowUpdate() int32 {
	if x != nil {
		if x, ok := x.Frame.(*OpenTunnelResponse_WindowUpdate); ok {
			return x.WindowUpdate
		}
	}
	return 0
}

func (x *OpenTunnelResponse) GetHeader() *typepb.GrpcMetadata {
	if x != nil {
		if x, ok := x.Frame.(*OpenTunnelResponse_Header); ok {
			return x.Header
		}
	}
	return nil
}

func (x *OpenTunnelResponse) GetMessage() *EncodedMessage {
	if x != nil {
		if x, ok := x.Frame.(*OpenTunnelResponse_Message); ok {
			return x.Message
		}
	}
	return nil
}

func (x *OpenTunnelResponse) GetMessageData() []byte {
	if x != nil {
		if x, ok := x.Frame.(*OpenTunnelResponse_MessageData); ok {
			return x.MessageData
		}
	}
	return nil
}

func (x *OpenTunnelResponse) GetCloseStream() *CloseStream {
	if x != nil {
		if x, ok := x.Frame.(*OpenTunnelResponse_CloseStream); ok {
			return x.CloseStream
		}
	}
	return nil
}

type isOpenTunnelResponse_Frame interface {
	isOpenTunnelResponse_Frame()
}

type OpenTunnelResponse_WindowUpdate struct {
	// Per-stream flow control message, based on HTTP/2.
	// Note that only message/message_data frames are subject to flow control.
	// https://datatracker.ietf.org/doc/html/rfc7540#section-6.9
	WindowUpdate int32 `protobuf:"varint,2,opt,name=window_update,json=windowUpdate,proto3,oneof"`
}

type OpenTunnelResponse_Header struct {
	// Sends response headers for this stream. If headers are sent at all,
	// they must be sent before any response message data.
	Header *typepb.GrpcMetadata `protobuf:"bytes,3,opt,name=header,proto3,oneof"`
}

type OpenTunnelResponse_Message struct {
	// Sends a message on the RPC stream. If the message is larger than 16k,
	// the rest of the message should be sent in chunks using the
	// message_data field (up to 16kb of data in each chunk).
	Message *EncodedMessage `protobuf:"bytes,4,opt,name=message,proto3,oneof"`
}

type OpenTunnelResponse_MessageData struct {
	// Sends a chunk of response data, for a response message that could not
	// wholly fit in a message field (e.g. > 16kb).
	MessageData []byte `protobuf:"bytes,5,opt,name=message_data,json=messageData,proto3,oneof"`
}

type OpenTunnelResponse_CloseStream struct {
	// Terminates the stream and communicates the final disposition to the
	// client. After the stream is closed, no other messages should use the
	// given stream ID.
	CloseStream *CloseStream `protobuf:"bytes,6,opt,name=close_stream,json=closeStream,proto3,oneof"`
}

func (*OpenTunnelResponse_WindowUpdate) isOpenTunnelResponse_Frame() {}

func (*OpenTunnelResponse_Header) isOpenTunnelResponse_Frame() {}

func (*OpenTunnelResponse_Message) isOpenTunnelResponse_Frame() {}

func (*OpenTunnelResponse_MessageData) isOpenTunnelResponse_Frame() {}

func (*OpenTunnelResponse_CloseStream) isOpenTunnelResponse_Frame() {}

// OpenReverseTunnelRequest is the frame message a client sends on the
// GrpcTunnelService.OpenReverseTunnel stream, carrying the responses to the
// RPCs the server initiates.
//
// The frames are identical in shape to OpenTunnelResponse: for a single
// stream ID, the first such message should include the header field
// unless no headers are to be sent, then any number of responses via the
// message and message_data fields, and a close_stream frame concludes the
// stream. The stream IDs are chosen by the server, in monotonically
// increasing order, and cannot be re-used.
type OpenReverseTunnelRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The ID of the stream. Stream IDs are defined by the server and should be
	// used in monotonically increasing order. They cannot be re-used.
	StreamId int64 `protobuf:"varint,1,opt,name=stream_id,json=streamId,proto3" json:"stream_id,omitempty"`
	// Types that are valid to be assigned to Frame:
	//
	//	*OpenReverseTunnelRequest_WindowUpdate
	//	*OpenReverseTunnelRequest_Header
	//	*OpenReverseTunnelRequest_Message
	//	*OpenReverseTunnelRequest_MessageData
	//	*OpenReverseTunnelRequest_CloseStream
	Frame         isOpenReverseTunnelRequest_Frame `protobuf_oneof:"frame"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenReverseTunnelRequest) Reset() {
	*x = OpenReverseTunnelRequest{}
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenReverseTunnelRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenReverseTunnelRequest) ProtoMessage() {}

func (x *OpenReverseTunnelRequest) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenReverseTunnelRequest.ProtoReflect.Descriptor instead.
func (*OpenReverseTunnelRequest) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_grpctunnel_proto_rawDescGZIP(), []int{2}
}

func (x *OpenReverseTunnelRequest) GetStreamId() int64 {
	if x != nil {
		return x.StreamId
	}
	return 0
}

func (x *OpenReverseTunnelRequest) GetFrame() isOpenReverseTunnelRequest_Frame {
	if x != nil {
		return x.Frame
	}
	return nil
}

func (x *OpenReverseTunnelRequest) GetWindowUpdate() int32 {
	if x != nil {
		if x, ok := x.Frame.(*OpenReverseTunnelRequest_WindowUpdate); ok {
			return x.WindowUpdate
		}
	}
	return 0
}

func (x *OpenReverseTunnelRequest) GetHeader() *typepb.GrpcMetadata {
	if x != nil {
		if x, ok := x.Frame.(*OpenReverseTunnelRequest_Header); ok {
			return x.Header
		}
	}
	return nil
}

func (x *OpenReverseTunnelRequest) GetMessage() *EncodedMessage {
	if x != nil {
		if x, ok := x.Frame.(*OpenReverseTunnelRequest_Message); ok {
			return x.Message
		}
	}
	return nil
}

func (x *OpenReverseTunnelRequest) GetMessageData() []byte {
	if x != nil {
		if x, ok := x.Frame.(*OpenReverseTunnelRequest_MessageData); ok {
			return x.MessageData
		}
	}
	return nil
}

func (x *OpenReverseTunnelRequest) GetCloseStream() *CloseStream {
	if x != nil {
		if x, ok := x.Frame.(*OpenReverseTunnelRequest_CloseStream); ok {
			return x.CloseStream
		}
	}
	return nil
}

type isOpenReverseTunnelRequest_Frame interface {
	isOpenReverseTunnelRequest_Frame()
}

type OpenReverseTunnelRequest_WindowUpdate struct {
	// Per-stream flow control message, based on HTTP/2.
	// Note that only message/message_data frames are subject to flow control.
	// https://datatracker.ietf.org/doc/html/rfc7540#section-6.9
	WindowUpdate int32 `protobuf:"varint,2,opt,name=window_update,json=windowUpdate,proto3,oneof"`
}

type OpenReverseTunnelRequest_Header struct {
	// Sends response headers for this stream. If headers are sent at all,
	// they must be sent before any response message data.
	Header *typepb.GrpcMetadata `protobuf:"bytes,3,opt,name=header,proto3,oneof"`
}

type OpenReverseTunnelRequest_Message struct {
	// Sends a message on the RPC stream. If the message is larger than 16k,
	// the rest of the message should be sent in chunks using the
	// message_data field (up to 16kb of data in each chunk).
	Message *EncodedMessage `protobuf:"bytes,4,opt,name=message,proto3,oneof"`
}

type OpenReverseTunnelRequest_MessageData struct {
	// Sends a chunk of response data, for a response message that could not
	// wholly fit in a message field (e.g. > 16kb).
	MessageData []byte `protobuf:"bytes,5,opt,name=message_data,json=messageData,proto3,oneof"`
}

type OpenReverseTunnelRequest_CloseStream struct {
	// Terminates the stream and communicates the final disposition to the
	// server. After the stream is closed, no other messages should use the
	// given stream ID.
	CloseStream *CloseStream `protobuf:"bytes,6,opt,name=close_stream,json=closeStream,proto3,oneof"`
}

func (*OpenReverseTunnelRequest_WindowUpdate) isOpenReverseTunnelRequest_Frame() {}

func (*OpenReverseTunnelRequest_Header) isOpenReverseTunnelRequest_Frame() {}

func (*OpenReverseTunnelRequest_Message) isOpenReverseTunnelRequest_Frame() {}

func (*OpenReverseTunnelRequest_MessageData) isOpenReverseTunnelRequest_Frame() {}

func (*OpenReverseTunnelRequest_CloseStream) isOpenReverseTunnelRequest_Frame() {}

// OpenReverseTunnelResponse is the frame message a server sends on the
// GrpcTunnelService.OpenReverseTunnel stream, carrying the requests for the
// RPCs the server initiates.
//
// The frames are identical in shape to OpenTunnelRequest: for a single
// stream ID, the first such message must include the new_stream field.
// After that, any number of requests may be sent via the message and
// message_data fields, and the RPC ends with either the half_close or
// cancel fields. If the half_close field is used, the RPC stream remains
// active so the client may continue to send response data. But, if the
// cancel field is used, the RPC stream is aborted and thus closed on both
// server and client ends. If a stream has been half-closed, the only
// allowed message from the server for that stream ID is one with the
// cancel field, to abort the remainder of the operation.
type OpenReverseTunnelResponse struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The ID of the stream. Stream IDs must be used in increasing order and
	// cannot be re-used.
	StreamId int64 `protobuf:"varint,1,opt,name=stream_id,json=streamId,proto3" json:"stream_id,omitempty"`
	// Types that are valid to be assigned to Frame:
	//
	//	*OpenReverseTunnelResponse_WindowUpdate
	//	*OpenReverseTunnelResponse_NewStream
	//	*OpenReverseTunnelResponse_Message
	//	*OpenReverseTunnelResponse_MessageData
	//	*OpenReverseTunnelResponse_HalfClose
	//	*OpenReverseTunnelResponse_Cancel
	Frame         isOpenReverseTunnelResponse_Frame `protobuf_oneof:"frame"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenReverseTunnelResponse) Reset() {
	*x = OpenReverseTunnelResponse{}
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenReverseTunnelResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenReverseTunnelResponse) ProtoMessage() {}

func (x *OpenReverseTunnelResponse) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenReverseTunnelResponse.ProtoReflect.Descriptor instead.
func (*OpenReverseTunnelResponse) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_grpctunnel_proto_rawDescGZIP(), []int{3}
}

func (x *OpenReverseTunnelResponse) GetStreamId() int64 {
	if x != nil {
		return x.StreamId
	}
	return 0
}

func (x *OpenReverseTunnelResponse) GetFrame() isOpenReverseTunnelResponse_Frame {
	if x != nil {
		return x.Frame
	}
	return nil
}

func (x *OpenReverseTunnelResponse) GetWindowUpdate() int32 {
	if x != nil {
		if x, ok := x.Frame.(*OpenReverseTunnelResponse_WindowUpdate); ok {
			return x.WindowUpdate
		}
	}
	return 0
}

func (x *OpenReverseTunnelResponse) GetNewStream() *NewStream {
	if x != nil {
		if x, ok := x.Frame.(*OpenReverseTunnelResponse_NewStream); ok {
			return x.NewStream
		}
	}
	return nil
}

func (x *OpenReverseTunnelResponse) GetMessage() *EncodedMessage {
	if x != nil {
		if x, ok := x.Frame.(*OpenReverseTunnelResponse_Message); ok {
			return x.Message
		}
	}
	return nil
}

func (x *OpenReverseTunnelResponse) GetMessageData() []byte {
	if x != nil {
		if x, ok := x.Frame.(*OpenReverseTunnelResponse_MessageData); ok {
			return x.MessageData
		}
	}
	return nil
}

func (x *OpenReverseTunnelResponse) GetHalfClose() *emptypb.Empty {
	if x != nil {
		if x, ok := x.Frame.(*OpenReverseTunnelResponse_HalfClose); ok {
			return x.HalfClose
		}
	}
	return nil
}

func (x *OpenReverseTunnelResponse) GetCancel() *emptypb.Empty {
	if x != nil {
		if x, ok := x.Frame.(*OpenReverseTunnelResponse_Cancel); ok {
			return x.Cancel
		}
	}
	return nil
}

type isOpenReverseTunnelResponse_Frame interface {
	isOpenReverseTunnelResponse_Frame()
}

type OpenReverseTunnelResponse_WindowUpdate struct {
	// Per-stream flow control message, based on HTTP/2.
	// Note that only message/message_data frames are subject to flow control.
	// https://datatracker.ietf.org/doc/html/rfc7540#section-6.9
	WindowUpdate int32 `protobuf:"varint,2,opt,name=window_update,json=windowUpdate,proto3,oneof"`
}

type OpenReverseTunnelResponse_NewStream struct {
	// Creates a new RPC stream, which includes request header metadata. The
	// stream ID must not be an already active stream.
	NewStream *NewStream `protobuf:"bytes,3,opt,name=new_stream,json=newStream,proto3,oneof"`
}

type OpenReverseTunnelResponse_Message struct {
	// Sends a message on the RPC stream. If the message is larger than 16k,
	// the rest of the message should be sent in chunks using the
	// message_data field (up to 16kb of data in each chunk).
	Message *EncodedMessage `protobuf:"bytes,4,opt,name=message,proto3,oneof"`
}

type OpenReverseTunnelResponse_MessageData struct {
	// Sends a chunk of request data, for a request message that could not
	// wholly fit in a message field (e.g. > 16kb).
	MessageData []byte `protobuf:"bytes,5,opt,name=message_data,json=messageData,proto3,oneof"`
}

type OpenReverseTunnelResponse_HalfClose struct {
	// Half-closes the stream, signaling that no more request messages will
	// be sent. No other messages, other than one with the cancel field set,
	// should be sent for this stream.
	HalfClose *emptypb.Empty `protobuf:"bytes,6,opt,name=half_close,json=halfClose,proto3,oneof"`
}

type OpenReverseTunnelResponse_Cancel struct {
	// Aborts the stream. No other messages should be sent for this stream.
	Cancel *emptypb.Empty `protobuf:"bytes,7,opt,name=cancel,proto3,oneof"`
}

func (*OpenReverseTunnelResponse_WindowUpdate) isOpenReverseTunnelResponse_Frame() {}

func (*OpenReverseTunnelResponse_NewStream) isOpenReverseTunnelResponse_Frame() {}

func (*OpenReverseTunnelResponse_Message) isOpenReverseTunnelResponse_Frame() {}

func (*OpenReverseTunnelResponse_MessageData) isOpenReverseTunnelResponse_Frame() {}

func (*OpenReverseTunnelResponse_HalfClose) isOpenReverseTunnelResponse_Frame() {}

func (*OpenReverseTunnelResponse_Cancel) isOpenReverseTunnelResponse_Frame() {}

// NewStream creates a new RPC stream in a tunnel, which includes request
// header metadata.
type NewStream struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The full method name of the RPC being initiated, e.g.
	// "sesame.v1alpha1.RemoteControl/NetConn".
	Method string `protobuf:"bytes,1,opt,name=method,proto3" json:"method,omitempty"`
	// Request header metadata for the RPC being initiated.
	Header        *typepb.GrpcMetadata `protobuf:"bytes,2,opt,name=header,proto3" json:"header,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *NewStream) Reset() {
	*x = NewStream{}
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *NewStream) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NewStream) ProtoMessage() {}

func (x *NewStream) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use NewStream.ProtoReflect.Descriptor instead.
func (*NewStream) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_grpctunnel_proto_rawDescGZIP(), []int{4}
}

func (x *NewStream) GetMethod() string {
	if x != nil {
		return x.Method
	}
	return ""
}

func (x *NewStream) GetHeader() *typepb.GrpcMetadata {
	if x != nil {
		return x.Header
	}
	return nil
}

// CloseStream terminates a tunneled RPC stream and communicates its final
// disposition.
type CloseStream struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Response trailer metadata for the RPC being concluded.
	Trailer *typepb.GrpcMetadata `protobuf:"bytes,1,opt,name=trailer,proto3" json:"trailer,omitempty"`
	// Terminal status of the RPC being concluded.
	Status        *status.Status `protobuf:"bytes,2,opt,name=status,proto3" json:"status,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *CloseStream) Reset() {
	*x = CloseStream{}
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CloseStream) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CloseStream) ProtoMessage() {}

func (x *CloseStream) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CloseStream.ProtoReflect.Descriptor instead.
func (*CloseStream) Descriptor() ([]byte, []int) {
	return file_sesame_v1alpha1_grpctunnel_proto_rawDescGZIP(), []int{5}
}

func (x *CloseStream) GetTrailer() *typepb.GrpcMetadata {
	if x != nil {
		return x.Trailer
	}
	return nil
}

func (x *CloseStream) GetStatus() *status.Status {
	if x != nil {
		return x.Status
	}
	return nil
}

// EncodedMessage models a binary gRPC message, and is used to frame tunneled messages.
type EncodedMessage struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The full size of the message.
	Size int32 `protobuf:"varint,1,opt,name=size,proto3" json:"size,omitempty"`
	// The message data. This field should not be longer than 16kb (16,384
	// bytes). If the full size of the message is larger then it should be
	// split into multiple chunks. The chunking is done to allow multiple
	// access to the underlying gRPC stream by concurrent tunneled streams.
	// If very large messages were sent via a single chunk, it could cause
	// head-of-line blocking and starvation when multiple streams need to send
	// data on the one underlying gRPC stream.
	Data          []byte `protobuf:"bytes,2,opt,name=data,proto3" json:"data,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *EncodedMessage) Reset() {
	*x = EncodedMessage{}
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *EncodedMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*EncodedMessage) ProtoMessage() {}

func (x *EncodedMessage) ProtoReflect() protoreflect.Message {
	mi := &file_sesame_v1alpha1_grpctunnel_proto_msgTypes[6]
	if x != nil {
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
	return file_sesame_v1alpha1_grpctunnel_proto_rawDescGZIP(), []int{6}
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

var File_sesame_v1alpha1_grpctunnel_proto protoreflect.FileDescriptor

const file_sesame_v1alpha1_grpctunnel_proto_rawDesc = "" +
	"\n" +
	" sesame/v1alpha1/grpctunnel.proto\x12\x0fsesame.v1alpha1\x1a\x1fgoogle/api/field_behavior.proto\x1a\x1bgoogle/protobuf/empty.proto\x1a\x17google/rpc/status.proto\x1a\x1esesame/type/grpcmetadata.proto\"\xef\x02\n" +
	"\x11OpenTunnelRequest\x12 \n" +
	"\tstream_id\x18\x01 \x01(\x03B\x03\xe0A\x01R\bstreamId\x12%\n" +
	"\rwindow_update\x18\x02 \x01(\x05H\x00R\fwindowUpdate\x12;\n" +
	"\n" +
	"new_stream\x18\x03 \x01(\v2\x1a.sesame.v1alpha1.NewStreamH\x00R\tnewStream\x12;\n" +
	"\amessage\x18\x04 \x01(\v2\x1f.sesame.v1alpha1.EncodedMessageH\x00R\amessage\x12#\n" +
	"\fmessage_data\x18\x05 \x01(\fH\x00R\vmessageData\x127\n" +
	"\n" +
	"half_close\x18\x06 \x01(\v2\x16.google.protobuf.EmptyH\x00R\thalfClose\x120\n" +
	"\x06cancel\x18\a \x01(\v2\x16.google.protobuf.EmptyH\x00R\x06cancelB\a\n" +
	"\x05frame\"\xbb\x02\n" +
	"\x12OpenTunnelResponse\x12\x1b\n" +
	"\tstream_id\x18\x01 \x01(\x03R\bstreamId\x12%\n" +
	"\rwindow_update\x18\x02 \x01(\x05H\x00R\fwindowUpdate\x123\n" +
	"\x06header\x18\x03 \x01(\v2\x19.sesame.type.GrpcMetadataH\x00R\x06header\x12;\n" +
	"\amessage\x18\x04 \x01(\v2\x1f.sesame.v1alpha1.EncodedMessageH\x00R\amessage\x12#\n" +
	"\fmessage_data\x18\x05 \x01(\fH\x00R\vmessageData\x12A\n" +
	"\fclose_stream\x18\x06 \x01(\v2\x1c.sesame.v1alpha1.CloseStreamH\x00R\vcloseStreamB\a\n" +
	"\x05frame\"\xc6\x02\n" +
	"\x18OpenReverseTunnelRequest\x12 \n" +
	"\tstream_id\x18\x01 \x01(\x03B\x03\xe0A\x01R\bstreamId\x12%\n" +
	"\rwindow_update\x18\x02 \x01(\x05H\x00R\fwindowUpdate\x123\n" +
	"\x06header\x18\x03 \x01(\v2\x19.sesame.type.GrpcMetadataH\x00R\x06header\x12;\n" +
	"\amessage\x18\x04 \x01(\v2\x1f.sesame.v1alpha1.EncodedMessageH\x00R\amessage\x12#\n" +
	"\fmessage_data\x18\x05 \x01(\fH\x00R\vmessageData\x12A\n" +
	"\fclose_stream\x18\x06 \x01(\v2\x1c.sesame.v1alpha1.CloseStreamH\x00R\vcloseStreamB\a\n" +
	"\x05frame\"\xf2\x02\n" +
	"\x19OpenReverseTunnelResponse\x12\x1b\n" +
	"\tstream_id\x18\x01 \x01(\x03R\bstreamId\x12%\n" +
	"\rwindow_update\x18\x02 \x01(\x05H\x00R\fwindowUpdate\x12;\n" +
	"\n" +
	"new_stream\x18\x03 \x01(\v2\x1a.sesame.v1alpha1.NewStreamH\x00R\tnewStream\x12;\n" +
	"\amessage\x18\x04 \x01(\v2\x1f.sesame.v1alpha1.EncodedMessageH\x00R\amessage\x12#\n" +
	"\fmessage_data\x18\x05 \x01(\fH\x00R\vmessageData\x127\n" +
	"\n" +
	"half_close\x18\x06 \x01(\v2\x16.google.protobuf.EmptyH\x00R\thalfClose\x120\n" +
	"\x06cancel\x18\a \x01(\v2\x16.google.protobuf.EmptyH\x00R\x06cancelB\a\n" +
	"\x05frame\"`\n" +
	"\tNewStream\x12\x1b\n" +
	"\x06method\x18\x01 \x01(\tB\x03\xe0A\x01R\x06method\x126\n" +
	"\x06header\x18\x02 \x01(\v2\x19.sesame.type.GrpcMetadataB\x03\xe0A\x01R\x06header\"x\n" +
	"\vCloseStream\x128\n" +
	"\atrailer\x18\x01 \x01(\v2\x19.sesame.type.GrpcMetadataB\x03\xe0A\x01R\atrailer\x12/\n" +
	"\x06status\x18\x02 \x01(\v2\x12.google.rpc.StatusB\x03\xe0A\x01R\x06status\"B\n" +
	"\x0eEncodedMessage\x12\x17\n" +
	"\x04size\x18\x01 \x01(\x05B\x03\xe0A\x01R\x04size\x12\x17\n" +
	"\x04data\x18\x02 \x01(\fB\x03\xe0A\x01R\x04data2\xde\x01\n" +
	"\x11GrpcTunnelService\x12Y\n" +
	"\n" +
	"OpenTunnel\x12\".sesame.v1alpha1.OpenTunnelRequest\x1a#.sesame.v1alpha1.OpenTunnelResponse(\x010\x01\x12n\n" +
	"\x11OpenReverseTunnel\x12).sesame.v1alpha1.OpenReverseTunnelRequest\x1a*.sesame.v1alpha1.OpenReverseTunnelResponse(\x010\x01Bb\n" +
	"%io.github.joeycumines.sesame.v1alpha1B\x0fGrpctunnelProtoP\x01Z&github.com/joeycumines/sesame/sesamepbb\x06proto3"

var (
	file_sesame_v1alpha1_grpctunnel_proto_rawDescOnce sync.Once
	file_sesame_v1alpha1_grpctunnel_proto_rawDescData []byte
)

func file_sesame_v1alpha1_grpctunnel_proto_rawDescGZIP() []byte {
	file_sesame_v1alpha1_grpctunnel_proto_rawDescOnce.Do(func() {
		file_sesame_v1alpha1_grpctunnel_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_sesame_v1alpha1_grpctunnel_proto_rawDesc), len(file_sesame_v1alpha1_grpctunnel_proto_rawDesc)))
	})
	return file_sesame_v1alpha1_grpctunnel_proto_rawDescData
}

var file_sesame_v1alpha1_grpctunnel_proto_msgTypes = make([]protoimpl.MessageInfo, 7)
var file_sesame_v1alpha1_grpctunnel_proto_goTypes = []any{
	(*OpenTunnelRequest)(nil),         // 0: sesame.v1alpha1.OpenTunnelRequest
	(*OpenTunnelResponse)(nil),        // 1: sesame.v1alpha1.OpenTunnelResponse
	(*OpenReverseTunnelRequest)(nil),  // 2: sesame.v1alpha1.OpenReverseTunnelRequest
	(*OpenReverseTunnelResponse)(nil), // 3: sesame.v1alpha1.OpenReverseTunnelResponse
	(*NewStream)(nil),                 // 4: sesame.v1alpha1.NewStream
	(*CloseStream)(nil),               // 5: sesame.v1alpha1.CloseStream
	(*EncodedMessage)(nil),            // 6: sesame.v1alpha1.EncodedMessage
	(*emptypb.Empty)(nil),             // 7: google.protobuf.Empty
	(*typepb.GrpcMetadata)(nil),       // 8: sesame.type.GrpcMetadata
	(*status.Status)(nil),             // 9: google.rpc.Status
}
var file_sesame_v1alpha1_grpctunnel_proto_depIdxs = []int32{
	4,  // 0: sesame.v1alpha1.OpenTunnelRequest.new_stream:type_name -> sesame.v1alpha1.NewStream
	6,  // 1: sesame.v1alpha1.OpenTunnelRequest.message:type_name -> sesame.v1alpha1.EncodedMessage
	7,  // 2: sesame.v1alpha1.OpenTunnelRequest.half_close:type_name -> google.protobuf.Empty
	7,  // 3: sesame.v1alpha1.OpenTunnelRequest.cancel:type_name -> google.protobuf.Empty
	8,  // 4: sesame.v1alpha1.OpenTunnelResponse.header:type_name -> sesame.type.GrpcMetadata
	6,  // 5: sesame.v1alpha1.OpenTunnelResponse.message:type_name -> sesame.v1alpha1.EncodedMessage
	5,  // 6: sesame.v1alpha1.OpenTunnelResponse.close_stream:type_name -> sesame.v1alpha1.CloseStream
	8,  // 7: sesame.v1alpha1.OpenReverseTunnelRequest.header:type_name -> sesame.type.GrpcMetadata
	6,  // 8: sesame.v1alpha1.OpenReverseTunnelRequest.message:type_name -> sesame.v1alpha1.EncodedMessage
	5,  // 9: sesame.v1alpha1.OpenReverseTunnelRequest.close_stream:type_name -> sesame.v1alpha1.CloseStream
	4,  // 10: sesame.v1alpha1.OpenReverseTunnelResponse.new_stream:type_name -> sesame.v1alpha1.NewStream
	6,  // 11: sesame.v1alpha1.OpenReverseTunnelResponse.message:type_name -> sesame.v1alpha1.EncodedMessage
	7,  // 12: sesame.v1alpha1.OpenReverseTunnelResponse.half_close:type_name -> google.protobuf.Empty
	7,  // 13: sesame.v1alpha1.OpenReverseTunnelResponse.cancel:type_name -> google.protobuf.Empty
	8,  // 14: sesame.v1alpha1.NewStream.header:type_name -> sesame.type.GrpcMetadata
	8,  // 15: sesame.v1alpha1.CloseStream.trailer:type_name -> sesame.type.GrpcMetadata
	9,  // 16: sesame.v1alpha1.CloseStream.status:type_name -> google.rpc.Status
	0,  // 17: sesame.v1alpha1.GrpcTunnelService.OpenTunnel:input_type -> sesame.v1alpha1.OpenTunnelRequest
	2,  // 18: sesame.v1alpha1.GrpcTunnelService.OpenReverseTunnel:input_type -> sesame.v1alpha1.OpenReverseTunnelRequest
	1,  // 19: sesame.v1alpha1.GrpcTunnelService.OpenTunnel:output_type -> sesame.v1alpha1.OpenTunnelResponse
	3,  // 20: sesame.v1alpha1.GrpcTunnelService.OpenReverseTunnel:output_type -> sesame.v1alpha1.OpenReverseTunnelResponse
	19, // [19:21] is the sub-list for method output_type
	17, // [17:19] is the sub-list for method input_type
	17, // [17:17] is the sub-list for extension type_name
	17, // [17:17] is the sub-list for extension extendee
	0,  // [0:17] is the sub-list for field type_name
}

func init() { file_sesame_v1alpha1_grpctunnel_proto_init() }
func file_sesame_v1alpha1_grpctunnel_proto_init() {
	if File_sesame_v1alpha1_grpctunnel_proto != nil {
		return
	}
	file_sesame_v1alpha1_grpctunnel_proto_msgTypes[0].OneofWrappers = []any{
		(*OpenTunnelRequest_WindowUpdate)(nil),
		(*OpenTunnelRequest_NewStream)(nil),
		(*OpenTunnelRequest_Message)(nil),
		(*OpenTunnelRequest_MessageData)(nil),
		(*OpenTunnelRequest_HalfClose)(nil),
		(*OpenTunnelRequest_Cancel)(nil),
	}
	file_sesame_v1alpha1_grpctunnel_proto_msgTypes[1].OneofWrappers = []any{
		(*OpenTunnelResponse_WindowUpdate)(nil),
		(*OpenTunnelResponse_Header)(nil),
		(*OpenTunnelResponse_Message)(nil),
		(*OpenTunnelResponse_MessageData)(nil),
		(*OpenTunnelResponse_CloseStream)(nil),
	}
	file_sesame_v1alpha1_grpctunnel_proto_msgTypes[2].OneofWrappers = []any{
		(*OpenReverseTunnelRequest_WindowUpdate)(nil),
		(*OpenReverseTunnelRequest_Header)(nil),
		(*OpenReverseTunnelRequest_Message)(nil),
		(*OpenReverseTunnelRequest_MessageData)(nil),
		(*OpenReverseTunnelRequest_CloseStream)(nil),
	}
	file_sesame_v1alpha1_grpctunnel_proto_msgTypes[3].OneofWrappers = []any{
		(*OpenReverseTunnelResponse_WindowUpdate)(nil),
		(*OpenReverseTunnelResponse_NewStream)(nil),
		(*OpenReverseTunnelResponse_Message)(nil),
		(*OpenReverseTunnelResponse_MessageData)(nil),
		(*OpenReverseTunnelResponse_HalfClose)(nil),
		(*OpenReverseTunnelResponse_Cancel)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_sesame_v1alpha1_grpctunnel_proto_rawDesc), len(file_sesame_v1alpha1_grpctunnel_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   7,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_sesame_v1alpha1_grpctunnel_proto_goTypes,
		DependencyIndexes: file_sesame_v1alpha1_grpctunnel_proto_depIdxs,
		MessageInfos:      file_sesame_v1alpha1_grpctunnel_proto_msgTypes,
	}.Build()
	File_sesame_v1alpha1_grpctunnel_proto = out.File
	file_sesame_v1alpha1_grpctunnel_proto_goTypes = nil
	file_sesame_v1alpha1_grpctunnel_proto_depIdxs = nil
}
