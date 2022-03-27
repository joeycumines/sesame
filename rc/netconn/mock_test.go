package netconn

import (
	"context"
	"github.com/joeycumines/sesame/genproto/type/netaddr"
	"github.com/joeycumines/sesame/rc"
	"net"
)

type (
	mockDialFactoryWrapper struct {
		netConnI
		local  net.Addr
		remote net.Addr
		state  *mockDialFactoryWrapperState
	}

	mockDialFactoryWrapperState struct {
		request          *rc.NetConnRequest_Dial
		factory          chan struct{}
		ctx              context.Context
		network, address string
		dial             chan struct{}
	}
)

func mockDialFactory(network, dialer, listener string) (DialerFactory, *mockDialFactoryWrapper) {
	av, bv := net.Pipe()
	state := &mockDialFactoryWrapperState{
		factory: make(chan struct{}),
		dial:    make(chan struct{}),
	}
	a := &mockDialFactoryWrapper{
		av,
		(&netaddr.NetAddr{
			Network: network,
			Address: dialer,
		}).AsGoNetAddr(),
		(&netaddr.NetAddr{
			Network: network,
			Address: listener,
		}).AsGoNetAddr(),
		state,
	}
	b := &mockDialFactoryWrapper{
		bv,
		(&netaddr.NetAddr{
			Network: network,
			Address: listener,
		}).AsGoNetAddr(),
		(&netaddr.NetAddr{
			Network: network,
			Address: dialer,
		}).AsGoNetAddr(),
		state,
	}
	return a.DialFactory, b
}

func (x *mockDialFactoryWrapper) LocalAddr() net.Addr { return x.local }

func (x *mockDialFactoryWrapper) RemoteAddr() net.Addr { return x.remote }

func (x *mockDialFactoryWrapper) DialFactory(req *rc.NetConnRequest_Dial) (Dialer, error) {
	x.state.request = req
	close(x.state.factory)
	return x, nil
}

func (x *mockDialFactoryWrapper) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	x.state.ctx = ctx
	x.state.network = network
	x.state.address = address
	close(x.state.dial)
	return x, nil
}
