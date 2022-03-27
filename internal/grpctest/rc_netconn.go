package grpctest

import (
	"context"
	"github.com/joeycumines/sesame/genproto/type/netaddr"
	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/rc"
	"github.com/joeycumines/sesame/rc/netconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"net"
	"runtime"
	"time"
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

func mockDialFactory(network, dialer, listener string) (netconn.DialerFactory, *mockDialFactoryWrapper) {
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

func (x *mockDialFactoryWrapper) DialFactory(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
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

func RC_NetConn_TestClient_DialContext(t testutil.TB, ccFactory testutil.ClientConnFactory) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)

	ctx := context.Background()

	const (
		connNetwork                = `tcp`
		connDialer                 = `1.2.3.4:60401`
		connListener               = `5.6.7.8:80`
		connAddress                = `example.com:80`
		connTimeout  time.Duration = 13586
	)

	dialerFactory, listenConn := mockDialFactory(connNetwork, connDialer, connListener)
	defer listenConn.Close()

	server := netconn.Server{Dialer: dialerFactory}

	gc := ccFactory(func(h testutil.GRPCServer) { rc.RegisterRemoteControlServer(h, &server) })
	defer gc.Close()

	client := netconn.Client{
		API:     rc.NewRemoteControlClient(gc),
		Timeout: connTimeout,
	}

	conn, err := client.DialContext(ctx, connNetwork, connAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if v := conn.LocalAddr(); v == nil {
		t.Error(v)
	} else if network, address := v.Network(), v.String(); network != connNetwork || address != connDialer {
		t.Error(network, address)
	}

	if v := conn.RemoteAddr(); v == nil {
		t.Error(v)
	} else if network, address := v.Network(), v.String(); network != connNetwork || address != connListener {
		t.Error(network, address)
	}

	select {
	case <-listenConn.state.factory:
		if !proto.Equal(listenConn.state.request, &rc.NetConnRequest_Dial{
			Address: &netaddr.NetAddr{
				Network: connNetwork,
				Address: connAddress,
			},
			Timeout: durationpb.New(connTimeout),
		}) {
			t.Error(listenConn.state.request)
		}
	default:
		t.Error()
	}

	select {
	case <-listenConn.state.dial:
		if listenConn.state.ctx == nil {
			t.Error(listenConn.state.ctx)
		}
		if listenConn.state.network != connNetwork {
			t.Error(listenConn.state.network)
		}
		if listenConn.state.address != connAddress {
			t.Error(listenConn.state.address)
		}
	default:
		t.Error()
	}

	{
		var (
			b    [4]byte
			done = make(chan struct{})
		)
		go func() {
			defer close(done)
			if n, err := listenConn.Write([]byte(`1234`)); err != nil || n != 4 {
				t.Error(n, err)
			}
		}()
		if n, err := conn.Read(b[:]); err != nil || n != 4 {
			t.Error(n, err)
		} else if s := string(b[:]); s != `1234` {
			t.Error(s)
		}
		<-done
	}

	{
		var (
			b    [4]byte
			done = make(chan struct{})
		)
		go func() {
			defer close(done)
			if n, err := conn.Write([]byte(`5678`)); err != nil || n != 4 {
				t.Error(n, err)
			}
		}()
		if n, err := listenConn.Read(b[:]); err != nil || n != 4 {
			t.Error(n, err)
		} else if s := string(b[:]); s != `5678` {
			t.Error(s)
		}
		<-done
	}

	if err := conn.Close(); err != nil {
		t.Error(err)
	}
}
