// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.21.6
// source: sesame/v1alpha1/sesame.proto

package api

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// SesameClient is the client API for Sesame service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type SesameClient interface {
	CreateRemote(ctx context.Context, in *CreateRemoteRequest, opts ...grpc.CallOption) (*Remote, error)
	GetRemote(ctx context.Context, in *GetRemoteRequest, opts ...grpc.CallOption) (*Remote, error)
	DeleteRemote(ctx context.Context, in *DeleteRemoteRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	UpdateRemote(ctx context.Context, in *UpdateRemoteRequest, opts ...grpc.CallOption) (*Remote, error)
	ListRemotes(ctx context.Context, in *ListRemotesRequest, opts ...grpc.CallOption) (*ListRemotesResponse, error)
	CreateEndpoint(ctx context.Context, in *CreateEndpointRequest, opts ...grpc.CallOption) (*Endpoint, error)
	GetEndpoint(ctx context.Context, in *GetEndpointRequest, opts ...grpc.CallOption) (*Endpoint, error)
	DeleteEndpoint(ctx context.Context, in *DeleteEndpointRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	UpdateEndpoint(ctx context.Context, in *UpdateEndpointRequest, opts ...grpc.CallOption) (*Endpoint, error)
	ListEndpoints(ctx context.Context, in *ListEndpointsRequest, opts ...grpc.CallOption) (*ListEndpointsResponse, error)
	Proxy(ctx context.Context, opts ...grpc.CallOption) (Sesame_ProxyClient, error)
}

type sesameClient struct {
	cc grpc.ClientConnInterface
}

func NewSesameClient(cc grpc.ClientConnInterface) SesameClient {
	return &sesameClient{cc}
}

func (c *sesameClient) CreateRemote(ctx context.Context, in *CreateRemoteRequest, opts ...grpc.CallOption) (*Remote, error) {
	out := new(Remote)
	err := c.cc.Invoke(ctx, "/sesame.v1alpha1.Sesame/CreateRemote", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sesameClient) GetRemote(ctx context.Context, in *GetRemoteRequest, opts ...grpc.CallOption) (*Remote, error) {
	out := new(Remote)
	err := c.cc.Invoke(ctx, "/sesame.v1alpha1.Sesame/GetRemote", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sesameClient) DeleteRemote(ctx context.Context, in *DeleteRemoteRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/sesame.v1alpha1.Sesame/DeleteRemote", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sesameClient) UpdateRemote(ctx context.Context, in *UpdateRemoteRequest, opts ...grpc.CallOption) (*Remote, error) {
	out := new(Remote)
	err := c.cc.Invoke(ctx, "/sesame.v1alpha1.Sesame/UpdateRemote", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sesameClient) ListRemotes(ctx context.Context, in *ListRemotesRequest, opts ...grpc.CallOption) (*ListRemotesResponse, error) {
	out := new(ListRemotesResponse)
	err := c.cc.Invoke(ctx, "/sesame.v1alpha1.Sesame/ListRemotes", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sesameClient) CreateEndpoint(ctx context.Context, in *CreateEndpointRequest, opts ...grpc.CallOption) (*Endpoint, error) {
	out := new(Endpoint)
	err := c.cc.Invoke(ctx, "/sesame.v1alpha1.Sesame/CreateEndpoint", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sesameClient) GetEndpoint(ctx context.Context, in *GetEndpointRequest, opts ...grpc.CallOption) (*Endpoint, error) {
	out := new(Endpoint)
	err := c.cc.Invoke(ctx, "/sesame.v1alpha1.Sesame/GetEndpoint", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sesameClient) DeleteEndpoint(ctx context.Context, in *DeleteEndpointRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/sesame.v1alpha1.Sesame/DeleteEndpoint", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sesameClient) UpdateEndpoint(ctx context.Context, in *UpdateEndpointRequest, opts ...grpc.CallOption) (*Endpoint, error) {
	out := new(Endpoint)
	err := c.cc.Invoke(ctx, "/sesame.v1alpha1.Sesame/UpdateEndpoint", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sesameClient) ListEndpoints(ctx context.Context, in *ListEndpointsRequest, opts ...grpc.CallOption) (*ListEndpointsResponse, error) {
	out := new(ListEndpointsResponse)
	err := c.cc.Invoke(ctx, "/sesame.v1alpha1.Sesame/ListEndpoints", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sesameClient) Proxy(ctx context.Context, opts ...grpc.CallOption) (Sesame_ProxyClient, error) {
	stream, err := c.cc.NewStream(ctx, &Sesame_ServiceDesc.Streams[0], "/sesame.v1alpha1.Sesame/Proxy", opts...)
	if err != nil {
		return nil, err
	}
	x := &sesameProxyClient{stream}
	return x, nil
}

type Sesame_ProxyClient interface {
	Send(*ProxyRequest) error
	Recv() (*ProxyResponse, error)
	grpc.ClientStream
}

type sesameProxyClient struct {
	grpc.ClientStream
}

func (x *sesameProxyClient) Send(m *ProxyRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *sesameProxyClient) Recv() (*ProxyResponse, error) {
	m := new(ProxyResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// SesameServer is the server API for Sesame service.
// All implementations must embed UnimplementedSesameServer
// for forward compatibility
type SesameServer interface {
	CreateRemote(context.Context, *CreateRemoteRequest) (*Remote, error)
	GetRemote(context.Context, *GetRemoteRequest) (*Remote, error)
	DeleteRemote(context.Context, *DeleteRemoteRequest) (*emptypb.Empty, error)
	UpdateRemote(context.Context, *UpdateRemoteRequest) (*Remote, error)
	ListRemotes(context.Context, *ListRemotesRequest) (*ListRemotesResponse, error)
	CreateEndpoint(context.Context, *CreateEndpointRequest) (*Endpoint, error)
	GetEndpoint(context.Context, *GetEndpointRequest) (*Endpoint, error)
	DeleteEndpoint(context.Context, *DeleteEndpointRequest) (*emptypb.Empty, error)
	UpdateEndpoint(context.Context, *UpdateEndpointRequest) (*Endpoint, error)
	ListEndpoints(context.Context, *ListEndpointsRequest) (*ListEndpointsResponse, error)
	Proxy(Sesame_ProxyServer) error
	mustEmbedUnimplementedSesameServer()
}

// UnimplementedSesameServer must be embedded to have forward compatible implementations.
type UnimplementedSesameServer struct {
}

func (UnimplementedSesameServer) CreateRemote(context.Context, *CreateRemoteRequest) (*Remote, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateRemote not implemented")
}
func (UnimplementedSesameServer) GetRemote(context.Context, *GetRemoteRequest) (*Remote, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetRemote not implemented")
}
func (UnimplementedSesameServer) DeleteRemote(context.Context, *DeleteRemoteRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteRemote not implemented")
}
func (UnimplementedSesameServer) UpdateRemote(context.Context, *UpdateRemoteRequest) (*Remote, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateRemote not implemented")
}
func (UnimplementedSesameServer) ListRemotes(context.Context, *ListRemotesRequest) (*ListRemotesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListRemotes not implemented")
}
func (UnimplementedSesameServer) CreateEndpoint(context.Context, *CreateEndpointRequest) (*Endpoint, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateEndpoint not implemented")
}
func (UnimplementedSesameServer) GetEndpoint(context.Context, *GetEndpointRequest) (*Endpoint, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetEndpoint not implemented")
}
func (UnimplementedSesameServer) DeleteEndpoint(context.Context, *DeleteEndpointRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteEndpoint not implemented")
}
func (UnimplementedSesameServer) UpdateEndpoint(context.Context, *UpdateEndpointRequest) (*Endpoint, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateEndpoint not implemented")
}
func (UnimplementedSesameServer) ListEndpoints(context.Context, *ListEndpointsRequest) (*ListEndpointsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListEndpoints not implemented")
}
func (UnimplementedSesameServer) Proxy(Sesame_ProxyServer) error {
	return status.Errorf(codes.Unimplemented, "method Proxy not implemented")
}
func (UnimplementedSesameServer) mustEmbedUnimplementedSesameServer() {}

// UnsafeSesameServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to SesameServer will
// result in compilation errors.
type UnsafeSesameServer interface {
	mustEmbedUnimplementedSesameServer()
}

func RegisterSesameServer(s grpc.ServiceRegistrar, srv SesameServer) {
	s.RegisterService(&Sesame_ServiceDesc, srv)
}

func _Sesame_CreateRemote_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateRemoteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SesameServer).CreateRemote(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/sesame.v1alpha1.Sesame/CreateRemote",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SesameServer).CreateRemote(ctx, req.(*CreateRemoteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Sesame_GetRemote_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetRemoteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SesameServer).GetRemote(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/sesame.v1alpha1.Sesame/GetRemote",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SesameServer).GetRemote(ctx, req.(*GetRemoteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Sesame_DeleteRemote_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteRemoteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SesameServer).DeleteRemote(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/sesame.v1alpha1.Sesame/DeleteRemote",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SesameServer).DeleteRemote(ctx, req.(*DeleteRemoteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Sesame_UpdateRemote_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateRemoteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SesameServer).UpdateRemote(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/sesame.v1alpha1.Sesame/UpdateRemote",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SesameServer).UpdateRemote(ctx, req.(*UpdateRemoteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Sesame_ListRemotes_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListRemotesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SesameServer).ListRemotes(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/sesame.v1alpha1.Sesame/ListRemotes",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SesameServer).ListRemotes(ctx, req.(*ListRemotesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Sesame_CreateEndpoint_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateEndpointRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SesameServer).CreateEndpoint(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/sesame.v1alpha1.Sesame/CreateEndpoint",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SesameServer).CreateEndpoint(ctx, req.(*CreateEndpointRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Sesame_GetEndpoint_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetEndpointRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SesameServer).GetEndpoint(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/sesame.v1alpha1.Sesame/GetEndpoint",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SesameServer).GetEndpoint(ctx, req.(*GetEndpointRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Sesame_DeleteEndpoint_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteEndpointRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SesameServer).DeleteEndpoint(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/sesame.v1alpha1.Sesame/DeleteEndpoint",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SesameServer).DeleteEndpoint(ctx, req.(*DeleteEndpointRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Sesame_UpdateEndpoint_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateEndpointRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SesameServer).UpdateEndpoint(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/sesame.v1alpha1.Sesame/UpdateEndpoint",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SesameServer).UpdateEndpoint(ctx, req.(*UpdateEndpointRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Sesame_ListEndpoints_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListEndpointsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SesameServer).ListEndpoints(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/sesame.v1alpha1.Sesame/ListEndpoints",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SesameServer).ListEndpoints(ctx, req.(*ListEndpointsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Sesame_Proxy_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(SesameServer).Proxy(&sesameProxyServer{stream})
}

type Sesame_ProxyServer interface {
	Send(*ProxyResponse) error
	Recv() (*ProxyRequest, error)
	grpc.ServerStream
}

type sesameProxyServer struct {
	grpc.ServerStream
}

func (x *sesameProxyServer) Send(m *ProxyResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *sesameProxyServer) Recv() (*ProxyRequest, error) {
	m := new(ProxyRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// Sesame_ServiceDesc is the grpc.ServiceDesc for Sesame service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Sesame_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "sesame.v1alpha1.Sesame",
	HandlerType: (*SesameServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CreateRemote",
			Handler:    _Sesame_CreateRemote_Handler,
		},
		{
			MethodName: "GetRemote",
			Handler:    _Sesame_GetRemote_Handler,
		},
		{
			MethodName: "DeleteRemote",
			Handler:    _Sesame_DeleteRemote_Handler,
		},
		{
			MethodName: "UpdateRemote",
			Handler:    _Sesame_UpdateRemote_Handler,
		},
		{
			MethodName: "ListRemotes",
			Handler:    _Sesame_ListRemotes_Handler,
		},
		{
			MethodName: "CreateEndpoint",
			Handler:    _Sesame_CreateEndpoint_Handler,
		},
		{
			MethodName: "GetEndpoint",
			Handler:    _Sesame_GetEndpoint_Handler,
		},
		{
			MethodName: "DeleteEndpoint",
			Handler:    _Sesame_DeleteEndpoint_Handler,
		},
		{
			MethodName: "UpdateEndpoint",
			Handler:    _Sesame_UpdateEndpoint_Handler,
		},
		{
			MethodName: "ListEndpoints",
			Handler:    _Sesame_ListEndpoints_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Proxy",
			Handler:       _Sesame_Proxy_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "sesame/v1alpha1/sesame.proto",
}
