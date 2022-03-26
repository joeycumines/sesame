package grpc

type (
	mockServer struct {
		openTunnel        func(stream TunnelService_OpenTunnelServer) error
		openReverseTunnel func(stream TunnelService_OpenReverseTunnelServer) error
		unimplementedTunnelServiceServer
	}
)

func (x *mockServer) OpenTunnel(stream TunnelService_OpenTunnelServer) error {
	if x.openTunnel != nil {
		return x.openTunnel(stream)
	}
	return x.unimplementedTunnelServiceServer.OpenTunnel(stream)
}

func (x *mockServer) OpenReverseTunnel(stream TunnelService_OpenReverseTunnelServer) error {
	if x.openReverseTunnel != nil {
		return x.openReverseTunnel(stream)
	}
	return x.unimplementedTunnelServiceServer.OpenReverseTunnel(stream)
}
