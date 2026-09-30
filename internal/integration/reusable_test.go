package integration_test

import (
	"context"
	"fmt"
	"io"
	"sort"
	"testing"
	"time"

	"github.com/joeycumines/sesame/internal/grpctest"
	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/stream"
	grpctun "github.com/joeycumines/sesame/tun/grpc"
	"github.com/joeycumines/sesame/type/grpctunnel"
	"golang.org/x/exp/maps"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestReusable_RC_NetConn_Client_DialContext exercises the reusable DialContext test suite across all ClientConnFactories.
func TestReusable_RC_NetConn_Client_DialContext(t *testing.T) {
	skipUnlessIntegration(t)
	for _, k := range testutil.CallOn(maps.Keys(testutil.ClientConnFactories), func(v []string) { sort.Strings(v) }) {
		factory := testutil.ClientConnFactories[k]
		t.Run(k, func(t *testing.T) {
			grpctest.RC_NetConn_TestClient_DialContext(t, factory)
		})
	}
}

// TestReusable_RC_NetConn_nettest exercises the reusable nettest test suite across all ClientConnFactories.
func TestReusable_RC_NetConn_nettest(t *testing.T) {
	skipUnlessIntegration(t)
	wt := testutil.Wrap(t)
	wt = testutil.DepthLimiter{T: testutil.GoroutineChecker{T: wt}, Depth: 3}
	for _, k := range testutil.CallOn(maps.Keys(testutil.ClientConnFactories), func(v []string) { sort.Strings(v) }) {
		factory := testutil.ClientConnFactories[k]
		wt.Run(k, func(t testutil.T) {
			grpctest.RC_NetConn_Test_nettest(t, factory)
		})
	}
}

// TestReusable_Tunnel_RC_NetConn exercises RemoteControl client dialing and data exchange over a grpctunnel.
func TestReusable_Tunnel_RC_NetConn(t *testing.T) {
	skipUnlessIntegration(t)

	tunnelFactories := make(map[string]testutil.ClientConnFactory)
	for k, f := range testutil.ClientConnFactories {
		tunnelFactories["tunnel_"+k] = makeTunnelCCFactory(f)
		tunnelFactories["reverse_tunnel_"+k] = makeReverseTunnelCCFactory(f)
	}

	for _, k := range testutil.CallOn(maps.Keys(tunnelFactories), func(v []string) { sort.Strings(v) }) {
		factory := tunnelFactories[k]
		t.Run(k, func(t *testing.T) {
			grpctest.RC_NetConn_TestClient_DialContext(t, factory)
		})
	}
}

func makeTunnelCCFactory(ccFactory testutil.ClientConnFactory) testutil.ClientConnFactory {
	return func(fn func(h testutil.GRPCServer)) testutil.ClientConnCloser {
		svc := grpctun.TunnelServer{NoReverseTunnels: true}
		fn(&svc)
		cc := ccFactory(func(h testutil.GRPCServer) { grpctunnel.RegisterTunnelServiceServer(h, &svc) })
		st, err := grpctunnel.NewTunnelServiceClient(cc).OpenTunnel(context.Background())
		if err != nil {
			panic(err)
		}
		ch, err := grpctun.NewChannel(grpctun.OptChannel.ClientStream(st))
		if err != nil {
			panic(err)
		}
		return wrapCloser(ch, stream.Closers(ch, cc))
	}
}

func makeReverseTunnelCCFactory(ccFactory testutil.ClientConnFactory) testutil.ClientConnFactory {
	return func(fn func(h testutil.GRPCServer)) testutil.ClientConnCloser {
		ready := make(chan struct{}, 1)
		svc := grpctun.TunnelServer{OnReverseTunnelConnect: func(*grpctun.Channel) {
			select {
			case ready <- struct{}{}:
			default:
			}
		}}
		cc := ccFactory(func(h testutil.GRPCServer) { grpctunnel.RegisterTunnelServiceServer(h, &svc) })
		st, err := grpctunnel.NewTunnelServiceClient(cc).OpenReverseTunnel(context.Background())
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
			panic("reverseTunnelCCFactory: timed out waiting for ready")
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
					t := time.NewTimer(time.Second * 30)
					defer t.Stop()
					select {
					case <-done:
						return serveErr
					case <-t.C:
						return fmt.Errorf("reverseTunnelCCFactory: close timed out")
					}
				}).Once(),
			),
		}
	}
}

func wrapCloser(ccc testutil.ClientConnCloser, closer io.Closer) testutil.ClientConnCloser {
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
