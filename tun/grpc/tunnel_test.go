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
	"github.com/fullstorydev/grpchan"
	"github.com/fullstorydev/grpchan/grpchantesting"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"net"
	"runtime"
	"testing"
	"time"
)

func TestTunnelServer(t *testing.T) {
	// Basic tests of the tunnel service as a gRPC channel

	var svr grpchantesting.TestServer

	ready := make(chan struct{})
	ts := TunnelServer{
		OnReverseTunnelConnect: func(*ReverseTunnelChannel) {
			// don't block; just make sure there's something in the channel
			select {
			case ready <- struct{}{}:
			default:
			}
		},
	}
	grpchantesting.RegisterHandlerTestService(&ts, &svr)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	gs := grpc.NewServer()
	RegisterTunnelServiceServer(gs, &ts)
	go gs.Serve(l)
	defer gs.Stop()

	cc, err := grpc.Dial(l.Addr().String(), grpc.WithBlock(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer cc.Close()

	cli := NewTunnelServiceClient(cc)

	t.Run("forward", func(t *testing.T) {
		checkForGoroutineLeak(t, func() {
			tunnel, err := cli.OpenTunnel(context.Background())
			if err != nil {
				t.Fatalf("failed to open tunnel: %v", err)
			}

			ch := NewChannel(tunnel)
			defer ch.Close()

			grpchantesting.RunChannelTestCases(t, ch, true)
		})
	})

	t.Run("reverse", func(t *testing.T) {
		checkForGoroutineLeak(t, func() {
			tunnel, err := cli.OpenReverseTunnel(context.Background())
			if err != nil {
				t.Fatalf("failed to open reverse tunnel: %v", err)
			}

			// client now acts as the server
			handlerMap := grpchan.HandlerMap{}
			grpchantesting.RegisterHandlerTestService(handlerMap, &svr)
			errs := make(chan error)
			go func() {
				errs <- ServeReverseTunnel(tunnel, handlerMap)
			}()

			defer func() {
				tunnel.CloseSend()
				err := <-errs
				// note this test appears a little dodgy, as you shouldn't call CloseSend concurrently with SendMsg
				if err != nil && err.Error() != `rpc error: code = Internal desc = SendMsg called after CloseSend` {
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
	before := runtime.NumGoroutine()

	fn()

	// check for goroutine leaks
	deadline := time.Now().Add(time.Second * 5)
	after := 0
	for deadline.After(time.Now()) {
		after = runtime.NumGoroutine()
		if after <= before {
			// number of goroutines returned to previous level: no leak!
			return
		}
		time.Sleep(time.Millisecond * 50)
	}
	buf := make([]byte, 1024*1024)
	n := runtime.Stack(buf, true)
	t.Errorf("%d goroutines leaked:\n%s", after-before, string(buf[:n]))
}

// TODO: also need more tests around channel lifecycle, and ensuring it
// properly respects things like context cancellations, etc

// TODO: also need some concurrency checks, to make sure the channel works
// as expected, and race detector finds no bugs, when used from many
// goroutines at once
