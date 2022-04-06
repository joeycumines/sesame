package grpc

import (
	"github.com/joeycumines/sesame/genproto/type/grpctunnel"
)

type (
	mockServer struct {
		openTunnel        func(stream grpctunnel.TunnelService_OpenTunnelServer) error
		openReverseTunnel func(stream grpctunnel.TunnelService_OpenReverseTunnelServer) error
		unimplementedTunnelServiceServer
	}
)

func (x *mockServer) OpenTunnel(stream grpctunnel.TunnelService_OpenTunnelServer) error {
	if x.openTunnel != nil {
		return x.openTunnel(stream)
	}
	return x.unimplementedTunnelServiceServer.OpenTunnel(stream)
}

func (x *mockServer) OpenReverseTunnel(stream grpctunnel.TunnelService_OpenReverseTunnelServer) error {
	if x.openReverseTunnel != nil {
		return x.openReverseTunnel(stream)
	}
	return x.unimplementedTunnelServiceServer.OpenReverseTunnel(stream)
}
