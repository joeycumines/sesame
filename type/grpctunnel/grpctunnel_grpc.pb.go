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

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             v4.23.4
// source: sesame/type/grpctunnel.proto

package grpctunnel

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

const (
	TunnelService_OpenTunnel_FullMethodName        = "/sesame.type.TunnelService/OpenTunnel"
	TunnelService_OpenReverseTunnel_FullMethodName = "/sesame.type.TunnelService/OpenReverseTunnel"
)

// TunnelServiceClient is the client API for TunnelService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type TunnelServiceClient interface {
	// OpenTunnel creates a channel to the server which can be used to send
	// additional RPCs, all of which will be sent to the same server via a
	// single underlying gRPC stream. This can provide affinity for a "chatty"
	// sequence of calls, where the gRPC connection is load balanced (so there
	// may be multiple backend servers), but a particular "conversation" (which
	// may consist of numerous RPCs) needs to all go to a single server, for
	// consistency.
	OpenTunnel(ctx context.Context, opts ...grpc.CallOption) (TunnelService_OpenTunnelClient, error)
	// OpenReverseTunnel creates a "reverse" channel, which allows the server to
	// act as a client and send RPCs to the client that creates the tunnel. It
	// is in most respects identical to OpenTunnel except that the roles are
	// reversed: the server initiates RPCs and sends requests and the client
	// replies to them and sends responses.
	OpenReverseTunnel(ctx context.Context, opts ...grpc.CallOption) (TunnelService_OpenReverseTunnelClient, error)
}

type tunnelServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewTunnelServiceClient(cc grpc.ClientConnInterface) TunnelServiceClient {
	return &tunnelServiceClient{cc}
}

func (c *tunnelServiceClient) OpenTunnel(ctx context.Context, opts ...grpc.CallOption) (TunnelService_OpenTunnelClient, error) {
	stream, err := c.cc.NewStream(ctx, &TunnelService_ServiceDesc.Streams[0], TunnelService_OpenTunnel_FullMethodName, opts...)
	if err != nil {
		return nil, err
	}
	x := &tunnelServiceOpenTunnelClient{stream}
	return x, nil
}

type TunnelService_OpenTunnelClient interface {
	Send(*ClientToServer) error
	Recv() (*ServerToClient, error)
	grpc.ClientStream
}

type tunnelServiceOpenTunnelClient struct {
	grpc.ClientStream
}

func (x *tunnelServiceOpenTunnelClient) Send(m *ClientToServer) error {
	return x.ClientStream.SendMsg(m)
}

func (x *tunnelServiceOpenTunnelClient) Recv() (*ServerToClient, error) {
	m := new(ServerToClient)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *tunnelServiceClient) OpenReverseTunnel(ctx context.Context, opts ...grpc.CallOption) (TunnelService_OpenReverseTunnelClient, error) {
	stream, err := c.cc.NewStream(ctx, &TunnelService_ServiceDesc.Streams[1], TunnelService_OpenReverseTunnel_FullMethodName, opts...)
	if err != nil {
		return nil, err
	}
	x := &tunnelServiceOpenReverseTunnelClient{stream}
	return x, nil
}

type TunnelService_OpenReverseTunnelClient interface {
	Send(*ServerToClient) error
	Recv() (*ClientToServer, error)
	grpc.ClientStream
}

type tunnelServiceOpenReverseTunnelClient struct {
	grpc.ClientStream
}

func (x *tunnelServiceOpenReverseTunnelClient) Send(m *ServerToClient) error {
	return x.ClientStream.SendMsg(m)
}

func (x *tunnelServiceOpenReverseTunnelClient) Recv() (*ClientToServer, error) {
	m := new(ClientToServer)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// TunnelServiceServer is the server API for TunnelService service.
// All implementations must embed UnimplementedTunnelServiceServer
// for forward compatibility
type TunnelServiceServer interface {
	// OpenTunnel creates a channel to the server which can be used to send
	// additional RPCs, all of which will be sent to the same server via a
	// single underlying gRPC stream. This can provide affinity for a "chatty"
	// sequence of calls, where the gRPC connection is load balanced (so there
	// may be multiple backend servers), but a particular "conversation" (which
	// may consist of numerous RPCs) needs to all go to a single server, for
	// consistency.
	OpenTunnel(TunnelService_OpenTunnelServer) error
	// OpenReverseTunnel creates a "reverse" channel, which allows the server to
	// act as a client and send RPCs to the client that creates the tunnel. It
	// is in most respects identical to OpenTunnel except that the roles are
	// reversed: the server initiates RPCs and sends requests and the client
	// replies to them and sends responses.
	OpenReverseTunnel(TunnelService_OpenReverseTunnelServer) error
	mustEmbedUnimplementedTunnelServiceServer()
}

// UnimplementedTunnelServiceServer must be embedded to have forward compatible implementations.
type UnimplementedTunnelServiceServer struct {
}

func (UnimplementedTunnelServiceServer) OpenTunnel(TunnelService_OpenTunnelServer) error {
	return status.Errorf(codes.Unimplemented, "method OpenTunnel not implemented")
}
func (UnimplementedTunnelServiceServer) OpenReverseTunnel(TunnelService_OpenReverseTunnelServer) error {
	return status.Errorf(codes.Unimplemented, "method OpenReverseTunnel not implemented")
}
func (UnimplementedTunnelServiceServer) mustEmbedUnimplementedTunnelServiceServer() {}

// UnsafeTunnelServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to TunnelServiceServer will
// result in compilation errors.
type UnsafeTunnelServiceServer interface {
	mustEmbedUnimplementedTunnelServiceServer()
}

func RegisterTunnelServiceServer(s grpc.ServiceRegistrar, srv TunnelServiceServer) {
	s.RegisterService(&TunnelService_ServiceDesc, srv)
}

func _TunnelService_OpenTunnel_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(TunnelServiceServer).OpenTunnel(&tunnelServiceOpenTunnelServer{stream})
}

type TunnelService_OpenTunnelServer interface {
	Send(*ServerToClient) error
	Recv() (*ClientToServer, error)
	grpc.ServerStream
}

type tunnelServiceOpenTunnelServer struct {
	grpc.ServerStream
}

func (x *tunnelServiceOpenTunnelServer) Send(m *ServerToClient) error {
	return x.ServerStream.SendMsg(m)
}

func (x *tunnelServiceOpenTunnelServer) Recv() (*ClientToServer, error) {
	m := new(ClientToServer)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _TunnelService_OpenReverseTunnel_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(TunnelServiceServer).OpenReverseTunnel(&tunnelServiceOpenReverseTunnelServer{stream})
}

type TunnelService_OpenReverseTunnelServer interface {
	Send(*ClientToServer) error
	Recv() (*ServerToClient, error)
	grpc.ServerStream
}

type tunnelServiceOpenReverseTunnelServer struct {
	grpc.ServerStream
}

func (x *tunnelServiceOpenReverseTunnelServer) Send(m *ClientToServer) error {
	return x.ServerStream.SendMsg(m)
}

func (x *tunnelServiceOpenReverseTunnelServer) Recv() (*ServerToClient, error) {
	m := new(ServerToClient)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// TunnelService_ServiceDesc is the grpc.ServiceDesc for TunnelService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var TunnelService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "sesame.type.TunnelService",
	HandlerType: (*TunnelServiceServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "OpenTunnel",
			Handler:       _TunnelService_OpenTunnel_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "OpenReverseTunnel",
			Handler:       _TunnelService_OpenReverseTunnel_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "sesame/type/grpctunnel.proto",
}
