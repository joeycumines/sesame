package grpc_test

import (
	"context"
	"fmt"
	"github.com/joeycumines/sesame/internal/grpctest"
	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/stream"
	grpctun "github.com/joeycumines/sesame/tun/grpc"
	"golang.org/x/exp/maps"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"runtime"
	"sort"
	"testing"
	"time"
)

var clientConnFactories = map[string]testutil.ClientConnFactory{
	`tunnel_netpipe`:         tunnelCCFactory(testutil.NetpipeClientConnFactory),
	`reverse_tunnel_netpipe`: reverseTunnelCCFactory(testutil.NetpipeClientConnFactory),
}

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

// Test_e2e pulls in other test cases to aid in testing the tunnel implementations (in both directions) under
// significant load, and in more "realistic" scenarios.
//func Test_e2e(t *testing.T) {
//	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, time.Second*5)
//
//	// Note that all tests must support running concurrently.
//	for _, tc := range [...]struct {
//		Name string
//		Test func(t testutil.T)
//	}{
//		{
//			Name: ``,
//		},
//	} {
//		t.Run(tc.Name, func(t *testing.T) {
//			defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, time.Second*5)
//
//			r, err := testutil.NewRunner(
//				testutil.OptRunner.T(testutil.Wrap(t)),
//			)
//			if err != nil {
//				t.Fatal(err)
//			}
//
//			tc.Test(r)
//		})
//	}
//}
