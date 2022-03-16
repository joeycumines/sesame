package netconn

import (
	"context"
	"fmt"
	"github.com/joeycumines/sesame/genproto/type/netaddr"
	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/rc"
	streamutil "github.com/joeycumines/sesame/stream"
	"golang.org/x/net/nettest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"net"
	"testing"
	"time"
)

func Test_nettest(t *testing.T) {
	makePipe := func(
		ctx context.Context,
		t *testing.T,
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

		server := Server{Dialer: dialerFactory}

		client := testutil.NewBufconnClient(0, func(_ *bufconn.Listener, srv *grpc.Server) { rc.RegisterRemoteControlServer(srv, &server) })
		t.Cleanup(func() {
			if err := client.Close(); err != nil {
				t.Error(err)
			}
		})

		conn, err := (&Client{
			API:         rc.NewRemoteControlClient(client),
			Timeout:     timeout,
			ClosePolicy: closePolicy,
		}).DialContext(ctx, clientNetworkValue, clientAddressValue)
		if err != nil {
			return nil, nil, err
		}

		{
			nc := conn.(*netConn)
			if !proto.Equal(nc.req.GetAddress(), &netaddr.NetAddr{
				Network: clientNetworkValue,
				Address: clientAddressValue,
			}) {
				b, _ := protojson.Marshal(nc.req.GetAddress())
				t.Errorf(`unexpected conn request address: %s`, b)
			}
			if !proto.Equal(nc.res, &rc.NetConnResponse_Conn{
				Local: &netaddr.NetAddr{
					Network: networkValue,
					Address: dialerValue,
				},
				Remote: &netaddr.NetAddr{
					Network: networkValue,
					Address: listenerValue,
				},
			}) {
				b, _ := protojson.Marshal(nc.res)
				t.Errorf(`unexpected conn msg: %s`, b)
			}
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
		Init  func(t *testing.T) nettest.MakePipe
	}{
		{
			// the wait remote timeout policy is necessary to handle one particular edge case on stop
			// specifically, in Test_nettest/wait_remote_timeout/d/RacyRead
			Name:  `wait remote timeout`,
			Close: true,
			Init: func(t *testing.T) nettest.MakePipe {
				return func() (c1, c2 net.Conn, stop func(), err error) {
					c1, c2, err = makePipe(context.Background(), t, 0, streamutil.WaitRemoteTimeout(time.Second*5))
					return
				}
			},
		},
		{
			Name:  `with cancel`,
			Close: true,
			Init: func(t *testing.T) nettest.MakePipe {
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
			Init: func(t *testing.T) nettest.MakePipe {
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
			Init: func(t *testing.T) nettest.MakePipe {
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

					server := Server{}

					client := testutil.NewBufconnClient(0, func(_ *bufconn.Listener, srv *grpc.Server) { rc.RegisterRemoteControlServer(srv, &server) })
					defer func() {
						if err != nil {
							_ = client.Close()
						}
					}()

					c1, err = (&Client{
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
		t.Run(tc.Name, func(t *testing.T) {
			testutil.TestConn(t, tc.Close, tc.Init)
		})
	}
}
