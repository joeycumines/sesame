// Copyright 2018 Joshua Humphries
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grpc

import (
	"context"
	"github.com/fullstorydev/grpchan/grpchantesting"
	"github.com/joeycumines/sesame/internal/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"runtime"
	"testing"
	"time"
)

func TestTunnelServer(t *testing.T) {
	// Basic tests of the tunnel service as a gRPC channel

	var svr grpchantesting.TestServer

	ready := make(chan struct{})
	ts := TunnelServer{
		OnReverseTunnelConnect: func(*Channel) {
			// don't block; just make sure there's something in the channel
			select {
			case ready <- struct{}{}:
			default:
			}
		},
	}
	grpchantesting.RegisterTestServiceServer(&ts, &svr)

	t.Run("forward", func(t *testing.T) {
		checkForGoroutineLeak(t, func() {
			cc := testutil.NewBufconnClient(0, func(_ *bufconn.Listener, srv *grpc.Server) { RegisterTunnelServiceServer(srv, &ts) })
			defer cc.Close()

			tunnel, err := NewTunnelServiceClient(cc).OpenTunnel(context.Background())
			if err != nil {
				t.Fatalf("failed to open tunnel: %v", err)
			}

			ch, err := NewChannel(OptChannel.ClientStream(tunnel))
			defer ch.Close()

			grpchantesting.RunChannelTestCases(t, ch, true)
		})
	})

	t.Run("reverse", func(t *testing.T) {
		checkForGoroutineLeak(t, func() {
			cc := testutil.NewBufconnClient(0, func(_ *bufconn.Listener, srv *grpc.Server) { RegisterTunnelServiceServer(srv, &ts) })
			defer cc.Close()

			tunnel, err := NewTunnelServiceClient(cc).OpenReverseTunnel(context.Background())
			if err != nil {
				t.Fatalf("failed to open reverse tunnel: %v", err)
			}

			stop := make(chan struct{})

			errs := make(chan error)
			go func() {
				errs <- ServeTunnel(
					OptTunnel.ClientStream(tunnel),
					OptTunnel.Service(func(h *HandlerMap) { grpchantesting.RegisterTestServiceServer(h, &svr) }),
					OptTunnel.StopSignal(stop),
				)
			}()

			defer func() {
				close(stop)
				err := <-errs
				// note this test appears a little dodgy, as you shouldn't call CloseSend concurrently with SendMsg
				if err != nil {
					t.Errorf("ServeReverseTunnel returned error: %v", err)
				}
			}()

			// make sure server has registered client, so we can issue RPCs to it
			<-ready
			ch := ts.AsChannel()
			grpchantesting.RunChannelTestCases(t, ch, true)
		})
	})
}

func checkForGoroutineLeak(t *testing.T, fn func()) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, time.Second*5)
	fn()
}

// TODO: also need more tests around channel lifecycle, and ensuring it
// properly respects things like context cancellations, etc

// TODO: also need some concurrency checks, to make sure the channel works
// as expected, and race detector finds no bugs, when used from many
// goroutines at once