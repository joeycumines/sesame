package grpctest

import (
	"context"
	"fmt"
	"github.com/joeycumines/sesame/genproto/type/netaddr"
	"github.com/joeycumines/sesame/internal/nettest"
	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/rc"
	"github.com/joeycumines/sesame/rc/netconn"
	streamutil "github.com/joeycumines/sesame/stream"
	"google.golang.org/protobuf/encoding/protojson"
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

func RC_NetConn_Test_nettest(
	t testutil.T,
	ccFactory testutil.ClientConnFactory,
) {
	makePipe := func(
		ctx context.Context,
		t testutil.T,
		timeout time.Duration,
		closePolicy streamutil.ClosePolicy,
	) (net.Conn, net.Conn, error) {
		const (
			networkValue       = `network`
			dialerValue        = `dialer`
			listenerValue      = `listener`
			clientNetworkValue = `client_network`
			clientAddressValue = `client_address`
		)
		dialerFactory, listenConn := mockDialFactory(networkValue, dialerValue, listenerValue)
		t.Cleanup(func() {
			if err := listenConn.Close(); err != nil {
				t.Error(err)
			}
			select {
			case <-listenConn.state.factory:
				if !proto.Equal(listenConn.state.request.GetAddress(), &netaddr.NetAddr{
					Network: clientNetworkValue,
					Address: clientAddressValue,
				}) {
					b, _ := protojson.Marshal(listenConn.state.request.GetAddress())
					t.Errorf(`unexpected request address: %s`, b)
				}
				if v := listenConn.state.request.GetTimeout().AsDuration(); v != timeout {
					t.Errorf(`expected timeout %s got %s`, timeout, v)
				}
			default:
				t.Error(`expected factory`)
			}
			select {
			case <-listenConn.state.dial:
				if listenConn.state.ctx == nil {
					t.Error(`expected non-nil ctx`)
				}
				if listenConn.state.network != clientNetworkValue {
					t.Errorf(`expected network %q got %q`, clientNetworkValue, listenConn.state.network)
				}
				if listenConn.state.address != clientAddressValue {
					t.Errorf(`expected address %q got %q`, clientNetworkValue, listenConn.state.address)
				}
			default:
				t.Error(`expected dial`)
			}
		})

		server := netconn.Server{Dialer: dialerFactory}

		client := ccFactory(func(h testutil.GRPCServer) { rc.RegisterRemoteControlServer(h, &server) })
		t.Cleanup(func() {
			if err := client.Close(); err != nil {
				t.Error(err)
			}
		})

		conn, err := (&netconn.Client{
			API:         rc.NewRemoteControlClient(client),
			Timeout:     timeout,
			ClosePolicy: closePolicy,
		}).DialContext(ctx, clientNetworkValue, clientAddressValue)
		if err != nil {
			return nil, nil, err
		}

		if v := conn.LocalAddr(); v == nil || v.Network() != networkValue || v.String() != dialerValue {
			t.Errorf(`unexpected local addr: %v`, v)
		}

		if v := conn.RemoteAddr(); v == nil || v.Network() != networkValue || v.String() != listenerValue {
			t.Errorf(`unexpected remote addr: %v`, v)
		}

		return conn, listenConn, nil
	}
	for _, tc := range [...]struct {
		Name  string
		Close bool
		Init  func(t testutil.T) nettest.MakePipe
	}{
		{
			// the wait remote timeout policy is necessary to handle one particular edge case on stop
			// specifically, in Test_nettest/wait_remote_timeout/d/RacyRead
			Name:  `wait remote timeout`,
			Close: true,
			Init: func(t testutil.T) nettest.MakePipe {
				return func() (c1, c2 net.Conn, stop func(), err error) {
					c1, c2, err = makePipe(context.Background(), t, 0, streamutil.WaitRemoteTimeout(time.Second*5))
					return
				}
			},
		},
		{
			Name:  `with cancel`,
			Close: true,
			Init: func(t testutil.T) nettest.MakePipe {
				return func() (c1, c2 net.Conn, stop func(), err error) {
					ctx, cancel := context.WithCancel(context.Background())
					c1, c2, err = makePipe(ctx, t, 0, nil)
					if err != nil {
						cancel()
					} else {
						stop = cancel
					}
					return
				}
			},
		},
		{
			Name:  `dial timeout`,
			Close: true,
			Init: func(t testutil.T) nettest.MakePipe {
				return func() (c1, c2 net.Conn, stop func(), err error) {
					ctx, cancel := context.WithCancel(context.Background())
					c1, c2, err = makePipe(ctx, t, 1234567898, nil)
					if err != nil {
						cancel()
					} else {
						stop = cancel
					}
					return
				}
			},
		},
		{
			Name: `tcp dialer`,
			Init: func(t testutil.T) nettest.MakePipe {
				t.Skip(`needs half closer support`)
				return func() (c1, c2 net.Conn, stop func(), err error) {
					ctx, cancel := context.WithCancel(context.Background())
					defer func() {
						if err != nil {
							cancel()
						}
					}()

					addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
					if err != nil {
						return
					}
					listener, err := net.ListenTCP("tcp", addr)
					if err != nil {
						return
					}
					defer func() {
						if err != nil {
							_ = listener.Close()
						}
					}()

					ch := make(chan *net.TCPConn)
					go func() {
						defer close(ch)
						if conn, err := listener.AcceptTCP(); err != nil {
							t.Error(err)
						} else {
							select {
							case <-ctx.Done():
								t.Error(ctx.Err())
							case ch <- conn:
							}
						}
					}()

					server := netconn.Server{}

					client := ccFactory(func(h testutil.GRPCServer) { rc.RegisterRemoteControlServer(h, &server) })
					defer func() {
						if err != nil {
							_ = client.Close()
						}
					}()

					c1, err = (&netconn.Client{
						API:     rc.NewRemoteControlClient(client),
						Timeout: time.Second * 30,
					}).DialContext(ctx, listener.Addr().Network(), listener.Addr().String())
					if err != nil {
						return
					}
					defer func() {
						if err != nil {
							_ = c1.Close()
						}
					}()

					timer := time.NewTimer(time.Second * 5)
					defer timer.Stop()
					select {
					case <-timer.C:
						t.Error(`timeout`)
					case c2 = <-ch:
					}
					if c2 == nil {
						err = fmt.Errorf(`failed to connect to tcp server`)
						return
					}

					stop = func() {
						cancel()
						_ = listener.Close()
						_ = c2.Close()
						_ = c1.Close()
						_ = client.Close()
					}

					return
				}
			},
		},
	} {
		t.Run(tc.Name, func(t testutil.T) {
			testutil.TestConn(t, nettest.TestConn, tc.Close, tc.Init)
		})
	}
}
