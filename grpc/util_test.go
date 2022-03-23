package grpc_test

import (
	"github.com/joeycumines/sesame/rc"
)

type (
	mockRemoteControlServer struct {
		rc.UnimplementedRemoteControlServer
		netConn func(stream rc.RemoteControl_NetConnServer) error
	}

	mockStreamRecvMsg struct{ recvMsg func(m interface{}) error }
)

func (x *mockRemoteControlServer) NetConn(stream rc.RemoteControl_NetConnServer) error {
	if x.netConn != nil {
		return x.netConn(stream)
	}
	return x.UnimplementedRemoteControlServer.NetConn(stream)
}

func (x *mockStreamRecvMsg) RecvMsg(m interface{}) error { return x.recvMsg(m) }
