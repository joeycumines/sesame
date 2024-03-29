// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             v4.23.4
// source: sesame/v1alpha1/remotecontrol.proto

package rc

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
	RemoteControl_NetConn_FullMethodName = "/sesame.v1alpha1.RemoteControl/NetConn"
)

// RemoteControlClient is the client API for RemoteControl service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type RemoteControlClient interface {
	// NetConn implements a remote dialer, which may be used for a variety of purposes, including forwarding to other
	// gRPC services running in the same (server) process.
	//
	// If the server doesn't support this method, it returns `UNIMPLEMENTED`.
	//
	// The message flow is:
	//
	//  1. NetConnRequest.dial
	//  2. NetConnResponse.conn
	//  3. Any number of NetConnRequest.bytes and NetConnResponse.bytes
	//  4. Termination
	//     i. Graceful (client initiated)
	//     a. Initiated by grpc.ClientStream.CloseSend
	//     b. After processing all received messages the server initiates (full) connection close of the
	//     proxy target
	//     c. All data read from the proxy target is sent to the client
	//     d. The connection is closed by the server
	//     ii. Proxy target initiated
	//     a. The proxy target connection closes
	//     b. All buffered data received from the proxy target is sent to the client
	//     c. An error is propagated to the client
	//     iii. Server initiated
	//     a. The server encounters an error (e.g. due to context cancel)
	//     b. The proxy target connection is closed
	//     c. An error is propagated to the client (though there are common cases where it's already gone)
	NetConn(ctx context.Context, opts ...grpc.CallOption) (RemoteControl_NetConnClient, error)
}

type remoteControlClient struct {
	cc grpc.ClientConnInterface
}

func NewRemoteControlClient(cc grpc.ClientConnInterface) RemoteControlClient {
	return &remoteControlClient{cc}
}

func (c *remoteControlClient) NetConn(ctx context.Context, opts ...grpc.CallOption) (RemoteControl_NetConnClient, error) {
	stream, err := c.cc.NewStream(ctx, &RemoteControl_ServiceDesc.Streams[0], RemoteControl_NetConn_FullMethodName, opts...)
	if err != nil {
		return nil, err
	}
	x := &remoteControlNetConnClient{stream}
	return x, nil
}

type RemoteControl_NetConnClient interface {
	Send(*NetConnRequest) error
	Recv() (*NetConnResponse, error)
	grpc.ClientStream
}

type remoteControlNetConnClient struct {
	grpc.ClientStream
}

func (x *remoteControlNetConnClient) Send(m *NetConnRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *remoteControlNetConnClient) Recv() (*NetConnResponse, error) {
	m := new(NetConnResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// RemoteControlServer is the server API for RemoteControl service.
// All implementations must embed UnimplementedRemoteControlServer
// for forward compatibility
type RemoteControlServer interface {
	// NetConn implements a remote dialer, which may be used for a variety of purposes, including forwarding to other
	// gRPC services running in the same (server) process.
	//
	// If the server doesn't support this method, it returns `UNIMPLEMENTED`.
	//
	// The message flow is:
	//
	//  1. NetConnRequest.dial
	//  2. NetConnResponse.conn
	//  3. Any number of NetConnRequest.bytes and NetConnResponse.bytes
	//  4. Termination
	//     i. Graceful (client initiated)
	//     a. Initiated by grpc.ClientStream.CloseSend
	//     b. After processing all received messages the server initiates (full) connection close of the
	//     proxy target
	//     c. All data read from the proxy target is sent to the client
	//     d. The connection is closed by the server
	//     ii. Proxy target initiated
	//     a. The proxy target connection closes
	//     b. All buffered data received from the proxy target is sent to the client
	//     c. An error is propagated to the client
	//     iii. Server initiated
	//     a. The server encounters an error (e.g. due to context cancel)
	//     b. The proxy target connection is closed
	//     c. An error is propagated to the client (though there are common cases where it's already gone)
	NetConn(RemoteControl_NetConnServer) error
	mustEmbedUnimplementedRemoteControlServer()
}

// UnimplementedRemoteControlServer must be embedded to have forward compatible implementations.
type UnimplementedRemoteControlServer struct {
}

func (UnimplementedRemoteControlServer) NetConn(RemoteControl_NetConnServer) error {
	return status.Errorf(codes.Unimplemented, "method NetConn not implemented")
}
func (UnimplementedRemoteControlServer) mustEmbedUnimplementedRemoteControlServer() {}

// UnsafeRemoteControlServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to RemoteControlServer will
// result in compilation errors.
type UnsafeRemoteControlServer interface {
	mustEmbedUnimplementedRemoteControlServer()
}

func RegisterRemoteControlServer(s grpc.ServiceRegistrar, srv RemoteControlServer) {
	s.RegisterService(&RemoteControl_ServiceDesc, srv)
}

func _RemoteControl_NetConn_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(RemoteControlServer).NetConn(&remoteControlNetConnServer{stream})
}

type RemoteControl_NetConnServer interface {
	Send(*NetConnResponse) error
	Recv() (*NetConnRequest, error)
	grpc.ServerStream
}

type remoteControlNetConnServer struct {
	grpc.ServerStream
}

func (x *remoteControlNetConnServer) Send(m *NetConnResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *remoteControlNetConnServer) Recv() (*NetConnRequest, error) {
	m := new(NetConnRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// RemoteControl_ServiceDesc is the grpc.ServiceDesc for RemoteControl service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var RemoteControl_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "sesame.v1alpha1.RemoteControl",
	HandlerType: (*RemoteControlServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "NetConn",
			Handler:       _RemoteControl_NetConn_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "sesame/v1alpha1/remotecontrol.proto",
}
