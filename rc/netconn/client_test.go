package netconn

import (
	"context"
	"fmt"
	"github.com/joeycumines/sesame/genproto/type/netaddr"
	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/rc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"runtime"
	"testing"
	"time"
)

func TestNetConn_String(t *testing.T) {
	const (
		networkValue       = `network`
		dialerValue        = `dialer`
		listenerValue      = `listener`
		clientNetworkValue = `client_network`
		clientAddressValue = `client_address`
		clientTimeout      = time.Duration(1234567898)
	)
	for _, tc := range [...]struct {
		Name   string
		Conn   *netConn
		String string
	}{
		{
			Name:   `unset`,
			Conn:   &netConn{},
			String: "sesame.v1alpha1.RemoteControl.NetConn()",
		},
		{
			Name: `set`,
			Conn: &netConn{
				req: &rc.NetConnRequest_Dial{
					Address: &netaddr.NetAddr{
						Network: clientNetworkValue,
						Address: clientAddressValue,
					},
					Timeout: durationpb.New(clientTimeout),
				},
				res: &rc.NetConnResponse_Conn{
					Local: &netaddr.NetAddr{
						Network: networkValue,
						Address: dialerValue,
					},
					Remote: &netaddr.NetAddr{
						Network: networkValue,
						Address: listenerValue,
					},
				},
			},
			String: "sesame.v1alpha1.RemoteControl.NetConn(req_net=\"client_network\" req_addr=\"client_address\" req_timeout=1.234567898s local_net=\"network\" local_addr=\"dialer\" remote_net=\"network\" remote_addr=\"listener\")",
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			if s := tc.Conn.String(); s != fmt.Sprint(tc.Conn) || s != tc.String {
				t.Errorf("unexpected stringer: %q\n%s", s, s)
			}
		})
	}
}

func TestClient_DialContext(t *testing.T) {
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

	server := Server{Dialer: dialerFactory}
	gc := testutil.NewBufconnClient(0, func(_ *bufconn.Listener, srv *grpc.Server) { rc.RegisterRemoteControlServer(srv, &server) })
	defer gc.Close()
	client := Client{
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
