package grpc

import (
	"context"
	"fmt"
	"github.com/fullstorydev/grpchan/grpchantesting"
	"github.com/joeycumines/sesame/genproto/type/grpctunnel"
	"github.com/joeycumines/sesame/internal/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	refl "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"math"
	"runtime"
	"sort"
	"testing"
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
		tunnelService = &mockServer{openTunnel: func(stream grpctunnel.TunnelService_OpenTunnelServer) error {
			return ServeTunnel(
				OptTunnel.ServerStream(stream),
				OptTunnel.Service(handlerMapConfig),
			)
		}}
	)

	conn := testutil.NewBufconnClient(0, func(_ *bufconn.Listener, srv *grpc.Server) {
		grpctunnel.RegisterTunnelServiceServer(srv, tunnelService)
	})
	defer conn.Close()

	tunnel, err := grpctunnel.NewTunnelServiceClient(conn).OpenTunnel(context.Background())
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

func Test_maxChunkSize(t *testing.T) {
	if maxChunkSize > maxChunkSizeBase {
		t.Fatal(maxChunkSize, maxChunkSizeBase)
	}

	encode := proto.Marshal

	var fattestInt32 int32
	{
		encodedSizes := map[int32]int{
			0:             0,
			1:             0,
			-1:            0,
			math.MaxInt32: 0,
			math.MinInt32: 0,
		}
		for k := range encodedSizes {
			b, err := proto.Marshal(&grpctunnel.EncodedMessage{Size: k})
			if err != nil {
				t.Fatal(err)
			}
			encodedSizes[k] = len(b)
		}
		//t.Log(encodedSizes)
		max := -1
		for k, v := range encodedSizes {
			if v > max || (v == max && k < fattestInt32) {
				fattestInt32 = k
				max = v
			}
		}
		//t.Log(fattestInt32)
	}
	if fattestInt32 != math.MinInt32 {
		t.Fatal(fattestInt32)
	}

	var fattestUint64 uint64
	{
		encodedSizes := map[uint64]int{
			0:              0,
			1:              0,
			math.MaxUint64: 0,
		}
		for k := range encodedSizes {
			b, err := proto.Marshal(&grpctunnel.ClientToServer{StreamId: k})
			if err != nil {
				t.Fatal(err)
			}
			encodedSizes[k] = len(b)
		}
		//t.Log(encodedSizes)
		var max int
		for k, v := range encodedSizes {
			if v >= max {
				fattestUint64 = k
				max = v
			}
		}
		//t.Log(fattestUint64)
	}
	if fattestUint64 != math.MaxUint64 {
		t.Fatal(fattestUint64)
	}

	for _, tc := range [...]struct {
		Name string
		Init func() proto.Message
	}{
		{
			Name: `client message`,
			Init: func() proto.Message {
				return &grpctunnel.ClientToServer{
					StreamId: fattestUint64,
					Frame: &grpctunnel.ClientToServer_Message{Message: &grpctunnel.EncodedMessage{
						Size: fattestInt32,
						Data: make([]byte, maxChunkSizeBase),
					}},
				}
			},
		},
		{
			Name: `client message data`,
			Init: func() proto.Message {
				return &grpctunnel.ClientToServer{
					StreamId: fattestUint64,
					Frame:    &grpctunnel.ClientToServer_MessageData{MessageData: make([]byte, maxChunkSizeBase)},
				}
			},
		},
		{
			Name: `server message`,
			Init: func() proto.Message {
				return &grpctunnel.ServerToClient{
					StreamId: fattestUint64,
					Frame: &grpctunnel.ServerToClient_Message{Message: &grpctunnel.EncodedMessage{
						Size: fattestInt32,
						Data: make([]byte, maxChunkSizeBase),
					}},
				}
			},
		},
		{
			Name: `server message data`,
			Init: func() proto.Message {
				return &grpctunnel.ServerToClient{
					StreamId: fattestUint64,
					Frame:    &grpctunnel.ServerToClient_MessageData{MessageData: make([]byte, maxChunkSizeBase)},
				}
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			msg := tc.Init()
			b, err := encode(msg)
			if err != nil {
				t.Fatal(err)
			}
			msg = nil
			if len(b) > grpcMaxFrameSize {
				t.Errorf(`expected max frame size %d exceeds grpc max frame size: delta %d`, len(b), len(b)-grpcMaxFrameSize)
			}
		})
	}
}
