package testutil

import (
	"context"
	"fmt"
	"github.com/fullstorydev/grpchan/grpchantesting"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	refl "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"runtime"
	"testing"
	"time"
)

func ExampleNewBufconnClient() {
	defer CheckNumGoroutines(nil, runtime.NumGoroutine(), false, 0)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	conn := NewBufconnClient(0, func(lis *bufconn.Listener, srv *grpc.Server) {
		if lis == nil || srv == nil {
			panic(`unexpected args`)
		}
		reflection.Register(srv)
	})
	defer conn.Close()

	stream, err := refl.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	if err != nil {
		panic(err)
	}

	if err := stream.Send(&refl.ServerReflectionRequest{MessageRequest: &refl.ServerReflectionRequest_ListServices{}}); err != nil {
		panic(err)
	}

	if msg, err := stream.Recv(); err != nil {
		panic(err)
	} else {
		services := make([]string, 0, len(msg.GetListServicesResponse().GetService()))
		for _, v := range msg.GetListServicesResponse().GetService() {
			services = append(services, v.GetName())
		}
		fmt.Printf("there are %d available services: %q\n", len(services), services)
	}

	if err := conn.Close(); err != nil {
		panic(err)
	}

	// output:
	// there are 1 available services: ["grpc.reflection.v1alpha.ServerReflection"]
}

func TestGrpchanClientConnFactory(t *testing.T) {
	defer CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)

	conn := GrpchanClientConnFactory(func(h GRPCServer) {
		grpchantesting.RegisterTestServiceServer(h, &grpchantesting.TestServer{})
	})
	defer conn.Close()

	grpchantesting.RunChannelTestCases(t, conn, true)

	if err := conn.Close(); err != nil {
		t.Error(err)
	}

	_, errUnary := grpchantesting.NewTestServiceClient(conn).Unary(context.Background(), &grpchantesting.Message{})
	_, errStream := grpchantesting.NewTestServiceClient(conn).ClientStream(context.Background())
	for i, err := range [...]error{errUnary, errStream} {
		stat, _ := status.FromError(err)
		if stat.Code() != codes.Unavailable || stat.Message() != `transport is closing` {
			t.Error(i, err)
		}
	}
}
