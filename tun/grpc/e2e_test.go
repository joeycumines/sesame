package grpc_test

import (
	"context"
	"fmt"
	"github.com/joeycumines/sesame/internal/grpctest"
	"github.com/joeycumines/sesame/internal/pipelistener"
	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/rc"
	"github.com/joeycumines/sesame/rc/netconn"
	"github.com/joeycumines/sesame/stream"
	grpctun "github.com/joeycumines/sesame/tun/grpc"
	"golang.org/x/exp/maps"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"net"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

type (
	mockDialer struct {
		dialContext func(ctx context.Context, network, address string) (net.Conn, error)
	}
)

var clientConnFactories = func() (m map[string]testutil.ClientConnFactory) {
	m = make(map[string]testutil.ClientConnFactory)
	for k, f := range testutil.ClientConnFactories {
		m[`tunnel_`+k] = tunnelCCFactory(f)
		m[`reverse_tunnel_`+k] = reverseTunnelCCFactory(f)
	}
	return
}()

func reverseTunnelCCFactory(ccFactory testutil.ClientConnFactory) testutil.ClientConnFactory {
	return func(fn func(h testutil.GRPCServer)) testutil.ClientConnCloser {
		ready := make(chan struct{}, 1)
		svc := grpctun.TunnelServer{OnReverseTunnelConnect: func(*grpctun.Channel) {
			select {
			case ready <- struct{}{}:
			default:
			}
		}}
		cc := ccFactory(func(h testutil.GRPCServer) { grpctun.RegisterTunnelServiceServer(h, &svc) })
		st, err := grpctun.NewTunnelServiceClient(cc).OpenReverseTunnel(context.Background())
		if err != nil {
			panic(err)
		}
		done := make(chan struct{})
		var serveErr error
		go func() {
			defer close(done)
			serveErr = grpctun.ServeTunnel(
				grpctun.OptTunnel.ClientStream(st),
				grpctun.OptTunnel.Service(func(h *grpctun.HandlerMap) { fn(h) }),
			)
			stat, _ := status.FromError(serveErr)
			switch stat.Code() {
			case codes.Unavailable, codes.Canceled:
				serveErr = nil
			case codes.Unknown:
				if serveErr == context.Canceled {
					serveErr = nil
				}
			}
		}()
		timer := time.NewTimer(time.Second * 5)
		defer timer.Stop()
		select {
		case <-timer.C:
			panic(`reverseTunnelCCFactory: timed out waiting for ready`)
		case <-ready:
		}
		timer.Stop()
		type (
			cci = grpc.ClientConnInterface
			c   = io.Closer
		)
		return struct {
			cci
			c
		}{
			cci: svc.AsChannel(),
			c: stream.Closers(
				cc,
				stream.Closer(func() error {
					timer := time.NewTimer(time.Second * 30)
					defer timer.Stop()
					select {
					case <-done:
						return serveErr
					case <-timer.C:
						return fmt.Errorf(`reverseTunnelCCFactory: close timed out`)
					}
				}).Once(),
			),
		}
	}
}

func tunnelCCFactory(ccFactory testutil.ClientConnFactory) testutil.ClientConnFactory {
	return func(fn func(h testutil.GRPCServer)) testutil.ClientConnCloser {
		svc := grpctun.TunnelServer{NoReverseTunnels: true}
		fn(&svc)
		cc := ccFactory(func(h testutil.GRPCServer) { grpctun.RegisterTunnelServiceServer(h, &svc) })
		st, err := grpctun.NewTunnelServiceClient(cc).OpenTunnel(context.Background())
		if err != nil {
			panic(err)
		}
		ch, err := grpctun.NewChannel(grpctun.OptChannel.ClientStream(st))
		if err != nil {
			panic(err)
		}
		return replaceCCCClose(ch, stream.Closers(ch, cc))
	}
}

func replaceCCCClose(ccc testutil.ClientConnCloser, closer io.Closer) testutil.ClientConnCloser {
	type (
		ccci = testutil.ClientConnCloser
		nccc struct{ ccci }
		c    = io.Closer
	)
	return struct {
		nccc
		c
	}{
		nccc: nccc{ccci: ccc},
		c:    closer,
	}
}

func Test_external_RC_NetConn_TestClient_DialContext(t *testing.T) {
	for _, k := range testutil.CallOn(maps.Keys(clientConnFactories), func(v []string) { sort.Strings(v) }) {
		t.Run(k, func(t *testing.T) { grpctest.RC_NetConn_TestClient_DialContext(t, clientConnFactories[k]) })
	}
}

func Test_external_RC_NetConn_nettest(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, time.Second*20)
	t.Run(`p`, func(t *testing.T) {
		wt := testutil.Wrap(t)
		wt = testutil.Parallel(wt)
		for _, k := range testutil.CallOn(maps.Keys(clientConnFactories), func(v []string) { sort.Strings(v) }) {
			wt.Run(k, func(t testutil.T) { grpctest.RC_NetConn_Test_nettest(t, clientConnFactories[k]) })
		}
	})
}

// Test_multiplex runs a variety of other tests in parallel, over mocked rc/netconn streams, backed by a single tunnel.
func Test_multiplex(t *testing.T) {
	if true {
		t.SkipNow()
	}

	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, time.Second*10)

	type (
		TestConfig struct {
			testutil.T
			Factory testutil.ClientConnFactory
		}

		TestFunc func(t TestConfig)
	)

	for _, k := range testutil.CallOn(maps.Keys(clientConnFactories), func(v []string) { sort.Strings(v) }) {
		factory := clientConnFactories[k]
		t.Run(k, func(t *testing.T) {
			defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, time.Second*10)

			var (
				mu      sync.Mutex
				index   int
				pipes   = make(map[string]net.Conn)
				newPipe = func() (string, net.Conn) {
					mu.Lock()
					defer mu.Unlock()
					index++
					address := strconv.Itoa(index)
					c1, c2 := net.Pipe()
					pipes[address] = c1
					return address, c2
				}
				dialer = mockDialer{dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
					mu.Lock()
					defer mu.Unlock()
					conn := pipes[address]
					if conn == nil {
						t.Error(`missing pipe for address`, address)
						panic(address)
					}
					delete(pipes, address)
					return conn, nil
				}}
			)

			rcServer := netconn.Server{Dialer: func(*rc.NetConnRequest_Dial) (netconn.Dialer, error) { return &dialer, nil }}
			rcConn := factory(func(h testutil.GRPCServer) { rc.RegisterRemoteControlServer(h, &rcServer) })
			defer rcConn.Close()
			rcClient := netconn.Client{API: rc.NewRemoteControlClient(rcConn)}
			pipeFactory := func() (net.Conn, net.Conn) {
				address, c2 := newPipe()
				c1, err := rcClient.DialContext(context.Background(), ``, address)
				if err != nil {
					t.Error(err)
					panic(err)
				}
				return c1, c2
			}

			var factory testutil.ClientConnFactory = func(fn func(h testutil.GRPCServer)) testutil.ClientConnCloser {
				return testutil.NewNetpipeClient(pipeFactory, func(_ *pipelistener.PipeListener, srv *grpc.Server) { fn(srv) })
			}

			testutil.Wrap(t).Run(`p`, func(t testutil.T) {
				t = testutil.Parallel(t)
				t.Run(`RC_NetConn_Test_nettest`, func(t testutil.T) { grpctest.RC_NetConn_Test_nettest(t, factory) })
			})
		})
	}
}

func (x *mockDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return x.dialContext(ctx, network, address)
}
