package testutil

import (
	"context"
	"github.com/fullstorydev/grpchan"
	"github.com/fullstorydev/grpchan/inprocgrpc"
	"github.com/joeycumines/sesame/internal/pipelistener"
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
	PipeClient struct {
		*grpc.ClientConn
		Listener ListenerDialer
		Server   *grpc.Server
		closer   io.Closer
	}

	PipeClientOption func(c *pipeClientConfig)

	pipeClientConfig struct {
		dialOptions   []grpc.DialOption
		serverOptions []grpc.ServerOption
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

	ListenerDialer interface {
		net.Listener
		DialContext(ctx context.Context) (net.Conn, error)
	}
)

var (
	ClientConnFactories = map[string]ClientConnFactory{
		`bufconn`: BufconnClientConnFactory,
		`grpchan`: GrpchanClientConnFactory,
		`netpipe`: NetpipeClientConnFactory,
	}

	// compile time assertions

	_ grpc.ClientConnInterface = (*PipeClient)(nil)
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

func NetpipeClientConnFactory(fn func(h GRPCServer)) ClientConnCloser {
	return NewNetpipeClient(net.Pipe, func(_ *pipelistener.PipeListener, srv *grpc.Server) { fn(srv) })
}

// NewBufconnClient initialises a new gRPC client for testing, using google.golang.org/grpc/test/bufconn.
// Size will default to DefaultBufconnSize if <= 0. The init func will be called prior to serving on the listener, which
// does not need to be implemented by the caller.
func NewBufconnClient(size int, init func(lis *bufconn.Listener, srv *grpc.Server), options ...PipeClientOption) *PipeClient {
	if size <= 0 {
		size = DefaultBufconnSize
	}
	listenerFactory := func() ListenerDialer {
		var lis *bufconn.Listener = bufconn.Listen(size)
		return lis
	}
	var initFunc func(lis net.Listener, srv *grpc.Server)
	if init != nil {
		initFunc = func(lis net.Listener, srv *grpc.Server) { init(lis.(*bufconn.Listener), srv) }
	}
	return newPipeClient(listenerFactory, initFunc, options...)
}

func NewNetpipeClient(factory func() (c1, c2 net.Conn), init func(lis *pipelistener.PipeListener, srv *grpc.Server), options ...PipeClientOption) *PipeClient {
	if factory == nil {
		panic(`nil factory`)
	}
	var f pipelistener.PipeFactory = func(context.Context) (listener, dialer net.Conn, _ error) {
		// note: ignores the input context, it appears to be only for dial (vs being for the whole stream)
		listener, dialer = factory()
		return
	}
	return NewPipeClient(f, init, options...)
}

// NewPipeClient initialises a new gRPC client for testing, using a net.Pipe style implementation.
// Size will default to DefaultBufconnSize if <= 0. The init func will be called prior to serving on the listener, which
// does not need to be implemented by the caller.
func NewPipeClient(factory pipelistener.PipeFactory, init func(lis *pipelistener.PipeListener, srv *grpc.Server), options ...PipeClientOption) *PipeClient {
	listenerFactory := func() ListenerDialer {
		var lis *pipelistener.PipeListener = pipelistener.NewPipeListener(factory)
		return lis
	}
	var initFunc func(lis net.Listener, srv *grpc.Server)
	if init != nil {
		initFunc = func(lis net.Listener, srv *grpc.Server) { init(lis.(*pipelistener.PipeListener), srv) }
	}
	return newPipeClient(listenerFactory, initFunc, options...)
}

func WithDialOptions(options ...grpc.DialOption) PipeClientOption {
	return func(c *pipeClientConfig) { c.dialOptions = append(c.dialOptions, options...) }
}

func WithServerOptions(options ...grpc.ServerOption) PipeClientOption {
	return func(c *pipeClientConfig) { c.serverOptions = append(c.serverOptions, options...) }
}

func newPipeClient(listenerFactory func() ListenerDialer, init func(lis net.Listener, srv *grpc.Server), options ...PipeClientOption) *PipeClient {
	var c pipeClientConfig
	for _, o := range options {
		o(&c)
	}

	listener := listenerFactory()
	server := grpc.NewServer(c.serverOptions...)

	if init != nil {
		init(listener, server)
	}

	out := make(chan error, 1)
	go func() { out <- server.Serve(listener) }()

	//lint:ignore SA1019 NewClient doesn't support empty address
	conn, err := grpc.Dial("", append([]grpc.DialOption{
		//lint:ignore SA1019 no real alternative offered
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
	}, c.dialOptions...)...)
	if err != nil {
		defer listener.Close()
		defer server.Stop()
		panic(err)
	}

	return &PipeClient{
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

func (x *PipeClient) Close() error { return x.closer.Close() }

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
