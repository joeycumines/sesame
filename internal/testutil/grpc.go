package testutil

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"io"
	"net"
)

const (
	// DefaultBufconnSize is an arbitrary default.
	DefaultBufconnSize = 1024 * 1024
)

type (
	// BufconnClient wraps a grpc.ClientConn to also close a bufconn.Listener and a grpc.Server, and is used by
	// NewBufconnClient.
	BufconnClient struct {
		*grpc.ClientConn
		Listener *bufconn.Listener
		Server   *grpc.Server
		closer   io.Closer
	}
)

var (
	// compile time assertions

	_ grpc.ClientConnInterface = (*BufconnClient)(nil)
)

// NewBufconnClient initialises a new gRPC client for testing, using google.golang.org/grpc/test/bufconn.
// Size will default to DefaultBufconnSize if <= 0. The init func will be called prior to serving on the listener, which
// does not need to be implemented by the caller.
func NewBufconnClient(size int, init func(lis *bufconn.Listener, srv *grpc.Server)) *BufconnClient {
	if size <= 0 {
		size = DefaultBufconnSize
	}

	listener := bufconn.Listen(size)
	server := grpc.NewServer()

	if init != nil {
		init(listener, server)
	}

	out := make(chan error, 1)
	go func() { out <- server.Serve(listener) }()

	conn, err := grpc.Dial(
		"",
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
	)
	if err != nil {
		defer listener.Close()
		defer server.Stop()
		panic(err)
	}

	return &BufconnClient{
		ClientConn: conn,
		Listener:   listener,
		Server:     server,
		closer: Closers(
			conn,
			Closer(func() error {
				server.Stop()
				return nil
			}),
			listener,
			Closer(func() error { return <-out }).Once(),
		),
	}
}

func (x *BufconnClient) Close() error { return x.closer.Close() }
