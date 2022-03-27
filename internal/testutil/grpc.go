package testutil

import (
	"context"
	"github.com/fullstorydev/grpchan"
	"github.com/fullstorydev/grpchan/inprocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"io"
	"net"
	"sync"
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

	ClientConnFactory func(fn func(h GRPCServer)) ClientConnCloser

	GRPCServer = reflection.GRPCServer

	ClientConnCloser interface {
		grpc.ClientConnInterface
		io.Closer
	}

	clientConnCancelCloser struct {
		conn grpc.ClientConnInterface
		stop chan struct{}
		mu   sync.Mutex
		wg   sync.WaitGroup
	}
)

var (
	ClientConnFactories = map[string]ClientConnFactory{
		`bufconn`: BufconnClientConnFactory,
		`grpchan`: GrpchanClientConnFactory,
	}

	// compile time assertions

	_ grpc.ClientConnInterface = (*BufconnClient)(nil)
)

func BufconnClientConnFactory(fn func(h GRPCServer)) ClientConnCloser {
	return NewBufconnClient(0, func(_ *bufconn.Listener, srv *grpc.Server) { fn(srv) })
}

func GrpchanClientConnFactory(fn func(h GRPCServer)) ClientConnCloser {
	h := make(grpchan.HandlerMap)
	fn(h)
	var conn inprocgrpc.Channel
	h.ForEach(conn.RegisterService)
	r := clientConnCancelCloser{
		conn: &conn,
		stop: make(chan struct{}),
	}
	r.wg.Add(1)
	return &r
}

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

func (x *clientConnCancelCloser) Close() error {
	x.mu.Lock()
	select {
	case <-x.stop:
	default:
		close(x.stop)
		x.wg.Done()
	}
	x.mu.Unlock()
	x.wg.Wait()
	return nil
}

func (x *clientConnCancelCloser) wrap(ctx context.Context) (context.Context, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	select {
	case <-x.stop:
		return nil, status.Error(codes.Unavailable, `transport is closing`)
	default:
	}
	ctx, cancel := context.WithCancel(ctx)
	x.wg.Add(1)
	go func() {
		<-x.stop
		cancel()
		x.wg.Done()
	}()
	return ctx, nil
}

func (x *clientConnCancelCloser) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	ctx, err := x.wrap(ctx)
	if err != nil {
		return err
	}
	return x.conn.Invoke(ctx, method, args, reply, opts...)
}

func (x *clientConnCancelCloser) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	ctx, err := x.wrap(ctx)
	if err != nil {
		return nil, err
	}
	return x.conn.NewStream(ctx, desc, method, opts...)
}
