package grpc

import "github.com/joeycumines/sesame/sesamepb"

type (
	mockServer struct {
		openTunnel        func(stream sesamepb.GrpcTunnelService_OpenTunnelServer) error
		openReverseTunnel func(stream sesamepb.GrpcTunnelService_OpenReverseTunnelServer) error
		unimplementedTunnelServiceServer
	}
)

func (x *mockServer) OpenTunnel(stream sesamepb.GrpcTunnelService_OpenTunnelServer) error {
	if x.openTunnel != nil {
		return x.openTunnel(stream)
	}
	return x.unimplementedTunnelServiceServer.OpenTunnel(stream)
}

func (x *mockServer) OpenReverseTunnel(stream sesamepb.GrpcTunnelService_OpenReverseTunnelServer) error {
	if x.openReverseTunnel != nil {
		return x.openReverseTunnel(stream)
	}
	return x.unimplementedTunnelServiceServer.OpenReverseTunnel(stream)
}
