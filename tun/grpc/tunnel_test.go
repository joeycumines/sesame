package grpc

import (
	"context"
	"fmt"
	"github.com/fullstorydev/grpchan/grpchantesting"
	"github.com/joeycumines/sesame/internal/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	refl "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/test/bufconn"
	"runtime"
	"sort"
	"time"
)

// ExampleServeTunnel_reflection demonstrates how to enable the reflection API on the tunnel.
func ExampleServeTunnel_reflection() {
	defer testutil.CheckNumGoroutines(nil, runtime.NumGoroutine(), false, time.Second*5)

	var (
		tunneledService  grpchantesting.TestServer
		handlerMapConfig HandlerMapConfig = func(h *HandlerMap) {
			// once it's serving, h will be registered with the combined set of services from all
			// OptTunnel.Service provided HandlerMapConfig values
			reflection.Register(h)
			grpchantesting.RegisterTestServiceServer(h, &tunneledService)
		}
		tunnelService = &mockServer{openTunnel: func(stream TunnelService_OpenTunnelServer) error {
			return ServeTunnel(
				OptTunnel.ServerStream(stream),
				OptTunnel.Service(handlerMapConfig),
			)
		}}
	)

	conn := testutil.NewBufconnClient(0, func(_ *bufconn.Listener, srv *grpc.Server) { RegisterTunnelServiceServer(srv, tunnelService) })
	defer conn.Close()

	tunnel, err := NewTunnelServiceClient(conn).OpenTunnel(context.Background())
	if err != nil {
		panic(err)
	}

	channel, err := NewChannel(OptChannel.ClientStream(tunnel))
	if err != nil {
		panic(err)
	}

	stream, err := refl.NewServerReflectionClient(channel).ServerReflectionInfo(context.Background())
	if err != nil {
		panic(err)
	}

	if err := stream.Send(&refl.ServerReflectionRequest{MessageRequest: &refl.ServerReflectionRequest_ListServices{}}); err != nil {
		panic(err)
	}

	res, err := stream.Recv()
	if err != nil {
		panic(err)
	}

	var names []string
	for _, svc := range res.GetListServicesResponse().GetService() {
		names = append(names, svc.GetName())
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Println(name)
	}

	// output:
	// grpc.reflection.v1alpha.ServerReflection
	// grpchantesting.TestService
}
