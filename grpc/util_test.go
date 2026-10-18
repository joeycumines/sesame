package grpc_test

import (
	sesamepb "github.com/joeycumines/sesame/sesamepb"
)

type (
	mockRemoteControlServer struct {
		sesamepb.UnimplementedRemoteControlServer
		netConn func(stream sesamepb.RemoteControl_NetConnServer) error
	}

	mockStreamRecvMsg struct{ recvMsg func(m interface{}) error }
)

func (x *mockRemoteControlServer) NetConn(stream sesamepb.RemoteControl_NetConnServer) error {
	if x.netConn != nil {
		return x.netConn(stream)
	}
	return x.UnimplementedRemoteControlServer.NetConn(stream)
}

func (x *mockStreamRecvMsg) RecvMsg(m interface{}) error { return x.recvMsg(m) }
