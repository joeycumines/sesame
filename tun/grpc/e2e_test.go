package grpc_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/joeycumines/sesame/internal/grpctest"
	"github.com/joeycumines/sesame/internal/pipelistener"
	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/rc"
	"github.com/joeycumines/sesame/rc/netconn"
	"github.com/joeycumines/sesame/stream"
	grpctun "github.com/joeycumines/sesame/tun/grpc"
	"github.com/joeycumines/sesame/type/grpctunnel"
	"golang.org/x/exp/maps"
	"golang.org/x/net/nettest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"io"
	"math"
	"math/rand"
	"net"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type (
	mockDialer struct {
		dialContext func(ctx context.Context, network, address string) (net.Conn, error)
	}

	debugNetConnWrites struct {
		netConnI
		hook func(b []byte)
	}

	debugClientConnSends struct {
		clientConnCloserI
		hook func(msg any)
	}

	debugClientStreamSends struct {
		clientStreamI
		hook func(msg any)
	}

	netConnI = net.Conn

	clientStreamI = grpc.ClientStream

	clientConnCloserI = testutil.ClientConnCloser
)

var clientConnFactories = func() (m map[string]testutil.ClientConnFactory) {
	m = make(map[string]testutil.ClientConnFactory)
	for k, f := range testutil.ClientConnFactories {
		m[`tunnel_`+k] = tunnelCCFactory(f)
		m[`reverse_tunnel_`+k] = reverseTunnelCCFactory(f)
	}
	return
}()

func reverseTunnelCCFactory(ccFactory testutil.ClientConnFactory) testutil.ClientConnFactory {
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
		cc := ccFactory(func(h testutil.GRPCServer) { grpctunnel.RegisterTunnelServiceServer(h, &svc) })
		st, err := grpctunnel.NewTunnelServiceClient(cc).OpenTunnel(context.Background())
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
		if k != `tunnel_netpipe` {
			continue
		}
		t.Run(k, func(t *testing.T) { grpctest.RC_NetConn_TestClient_DialContext(t, clientConnFactories[k]) })
	}
}

func Test_external_RC_NetConn_nettest(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, time.Second*10)
	t.Run(`p`, func(t *testing.T) {
		wt := testutil.Wrap(t)
		wt = testutil.Parallel(wt)
		for _, k := range testutil.CallOn(maps.Keys(clientConnFactories), func(v []string) { sort.Strings(v) }) {
			wt.Run(k, func(t testutil.T) { grpctest.RC_NetConn_Test_nettest(t, clientConnFactories[k]) })
		}
	})
}

func Test_largeMessage(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, time.Second*10)

	const (
		maxGRPCFrame  = 1 << 14 // 16,384
		maxProxyFrame = maxGRPCFrame - 128
	)
	if maxGRPCFrame >= stream.ChunkSize {
		t.Fatal()
	}

	const (
		expectedSize     = 1 << 20
		expectedChunks   = expectedSize / stream.ChunkSize
		expectedWrites   = expectedChunks * (stream.ChunkSize/maxProxyFrame + 1)
		expectedWriteMin = 230
	)
	remainingWrites := int64(expectedWrites)
	writeCh := make(chan int, expectedWrites)
	defer func() {
		close(writeCh)
		if l := len(writeCh); l != 0 {
			t.Error(l)
		}
		if v := atomic.LoadInt64(&remainingWrites); v != 0 {
			t.Error(v)
		}
	}()

	var remainingMsg int32
	defer func() {
		if v := atomic.LoadInt32(&remainingMsg); v != 0 {
			t.Error(v)
		}
	}()

	bufferedWrites := make(chan int, 512)
	var numUnexpectedWrites int64
	defer func() {
		close(bufferedWrites)
		if l := len(bufferedWrites); l != 0 {
			t.Error(l)
		}
		if v := atomic.LoadInt64(&numUnexpectedWrites); v >= expectedChunks/3 || v >= 15 {
			t.Error(v)
		} else {
			t.Log(`num unexpected writes:`, v)
		}
	}()

	factory := reverseTunnelCCFactory(func(factory testutil.ClientConnFactory) testutil.ClientConnFactory {
		return func(fn func(h testutil.GRPCServer)) testutil.ClientConnCloser {
			conn := factory(fn)
			conn = &debugClientConnSends{
				clientConnCloserI: conn,
				hook: func(msg any) {
					var data []byte
					if msg, ok := msg.(proto.Message); ok {
						if b, err := proto.Marshal(msg); err == nil {
							data = b
						}
					}
					if len(data) >= expectedWriteMin {
						select {
						case writeCh <- len(data):
						default:
							t.Error(`unable to send to writeCh`, len(writeCh))
						}
					}
					var extra string
					if msg, ok := msg.(*grpctunnel.ServerToClient); ok {
						switch frame := msg.GetFrame().(type) {
						case *grpctunnel.ServerToClient_Message:
							extra = fmt.Sprintf(` s2c_message=%d/%d`, len(frame.Message.GetData()), frame.Message.GetSize())
							if remainingMsg != 0 {
								t.Error(`never finished previous message`, remainingMsg)
							}
							atomic.StoreInt32(&remainingMsg, frame.Message.GetSize()-int32(len(frame.Message.GetData())))
							if len(frame.Message.GetData()) > maxProxyFrame {
								t.Error(`unexpected message len`, len(frame.Message.GetData()))
							}
							if frame.Message.GetSize() > maxProxyFrame {
								if len(frame.Message.GetData()) != maxProxyFrame {
									t.Error(`expected sensible frame chunking 1`)
								}
							} else if int(frame.Message.GetSize()) != len(frame.Message.GetData()) {
								t.Error(`expected sensible frame chunking 2`)
							}
							if frame.Message.GetSize() > stream.ChunkSize+200 {
								t.Error(`unexpectedly large message`, frame.Message.GetSize())
							}

						case *grpctunnel.ServerToClient_MessageData:
							extra = fmt.Sprintf(` s2c_message_data=%d`, len(frame.MessageData))
							if v := atomic.AddInt32(&remainingMsg, -int32(len(frame.MessageData))); v < 0 {
								t.Error(`bad remainingMsg`, v)
							}
							if len(frame.MessageData) > maxProxyFrame {
								t.Error(`unexpected message data len`, len(frame.MessageData))
							}
						}
					}
					//fmt.Printf("proto message with len %d: %T%s\n", len(data), msg, extra)
					_ = extra
				},
			}
			return conn
		}
	}(func(fn func(h testutil.GRPCServer)) testutil.ClientConnCloser {
		return testutil.NewNetpipeClient(
			func() (c1, c2 net.Conn) {
				c1, c2 = net.Pipe()
				c2 = &debugNetConnWrites{
					netConnI: c2,
					hook: func(b []byte) {
						//fmt.Println(`c2 write len`, len(b))
						if len(b) >= expectedWriteMin {
							i := expectedWrites - atomic.LoadInt64(&remainingWrites) - int64(len(writeCh))
							var size int
							var ok bool
							select {
							case size, ok = <-bufferedWrites:
							default:
							}
							if !ok {
								select {
								case size, ok = <-writeCh:
								default:
								}
								if ok {
									atomic.AddInt64(&remainingWrites, -1)
								}
							}
							if !ok {
								t.Error(`write`, i, `no expected write`)
							} else if delta := math.Abs(float64(size) - float64(len(b))); delta > 200 {
								atomic.AddInt64(&numUnexpectedWrites, 1)
								if size > len(b) {
									select {
									case bufferedWrites <- size - len(b):
									default:
										t.Errorf(`write %d expected write %d actual %d: buffered writes full`, i, size, len(b))
									}
								} else {
									t.Errorf(`write %d expected write %d actual %d`, i, size, len(b))
								}
							}
						}
					},
				}
				return
			},
			func(_ *pipelistener.PipeListener, srv *grpc.Server) { fn(srv) },
			testutil.WithDialOptions(
				grpc.WithReadBufferSize(0),
				grpc.WithWriteBufferSize(0),
			),
			testutil.WithServerOptions(
				grpc.ReadBufferSize(0),
				grpc.WriteBufferSize(0),
			),
		)
	}))

	local, remote := net.Pipe()
	dialer := mockDialer{dialContext: func(ctx context.Context, network, address string) (net.Conn, error) { return local, nil }}
	rcServer := netconn.Server{Dialer: func(*rc.NetConnRequest_Dial) (netconn.Dialer, error) { return &dialer, nil }}
	rcConn := factory(func(h testutil.GRPCServer) { rc.RegisterRemoteControlServer(h, &rcServer) })
	rcClient := netconn.Client{API: rc.NewRemoteControlClient(rcConn)}
	c1, c2 := func() (net.Conn, net.Conn) {
		conn, err := rcClient.DialContext(context.Background(), ``, ``)
		if err != nil {
			t.Error(err)
			panic(err)
		}
		return remote, conn
	}()

	expected := make([]byte, expectedSize)
	rand.New(rand.NewSource(12356)).Read(expected)
	if len(expected) <= stream.ChunkSize {
		t.Fatal(len(expected))
	}

	go func() {
		if n, err := c1.Write(expected); err != nil || n != len(expected) {
			t.Error(n, err)
		}
		if err := c1.Close(); err != nil {
			t.Error(err)
		}
	}()

	ch := make(chan []byte)
	go func() {
		var (
			actual  = make([]byte, len(expected)+1)
			size    int
			exiting bool
		)
		for {
			n, err := c2.Read(actual[size:])
			if n != 0 && n != stream.ChunkSize {
				if exiting {
					t.Error(n)
				} else {
					exiting = true
				}
			}
			size += n
			if size >= len(actual) {
				t.Error(size)
				break
			}
			if err != nil {
				if err != io.EOF {
					t.Error(err)
				}
				break
			}
		}
		if err := c2.Close(); err != nil {
			t.Error(err)
		}
		ch <- actual[:size]
	}()

	if actual := <-ch; !bytes.Equal(expected, actual) {
		t.Error(len(expected), len(actual))
	}

	if err := rcConn.Close(); err != nil {
		t.Error(err)
	}
}

// Test_multiplex runs a variety of other tests in parallel, over mocked rc/netconn streams, backed by a single tunnel.
func Test_multiplex(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, time.Second*10)

	for _, k := range testutil.CallOn(maps.Keys(clientConnFactories), func(v []string) { sort.Strings(v) }) {
		factory := clientConnFactories[k]
		t.Run(k, func(t *testing.T) {
			defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, time.Second*10)

			var (
				mu      sync.Mutex
				index   int
				pipes   = make(map[string]net.Conn)
				newPipe = func() (string, net.Conn) {
					mu.Lock()
					defer mu.Unlock()
					index++
					address := strconv.Itoa(index)
					c1, c2 := net.Pipe()
					pipes[address] = c1
					return address, c2
				}
				dialer = mockDialer{dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
					mu.Lock()
					defer mu.Unlock()
					conn := pipes[address]
					if conn == nil {
						t.Error(`missing pipe for address`, address)
						panic(address)
					}
					delete(pipes, address)
					return conn, nil
				}}
			)

			rcServer := netconn.Server{Dialer: func(*rc.NetConnRequest_Dial) (netconn.Dialer, error) { return &dialer, nil }}
			rcConn := factory(func(h testutil.GRPCServer) { rc.RegisterRemoteControlServer(h, &rcServer) })
			defer rcConn.Close()
			rcClient := netconn.Client{API: rc.NewRemoteControlClient(rcConn)}
			pipeFactory := func() (net.Conn, net.Conn) {
				address, c2 := newPipe()
				c1, err := rcClient.DialContext(context.Background(), ``, address)
				if err != nil {
					t.Error(err)
					panic(err)
				}
				return c1, c2
			}

			var makePipe nettest.MakePipe = func() (c1, c2 net.Conn, stop func(), err error) {
				c1, c2 = pipeFactory()
				stop = func() {
					_ = c1.Close()
					_ = c2.Close()
				}
				return
			}

			var factory testutil.ClientConnFactory = func(fn func(h testutil.GRPCServer)) testutil.ClientConnCloser {
				return testutil.NewNetpipeClient(pipeFactory, func(_ *pipelistener.PipeListener, srv *grpc.Server) { fn(srv) })
			}

			t.Run(`p`, func(t *testing.T) {
				t.Run(`nettest1`, func(t *testing.T) {
					t.Parallel()
					nettest.TestConn(t, makePipe)
				})
				t.Run(`nettest2`, func(t *testing.T) {
					t.Parallel()
					nettest.TestConn(t, makePipe)
				})
				t.Run(`nettest3`, func(t *testing.T) {
					t.Parallel()
					nettest.TestConn(t, makePipe)
				})
				wt := testutil.Wrap(t)
				wt = testutil.DepthLimiter{T: testutil.Parallel(wt), Depth: 3}
				wt.Run(`RC_NetConn_Test_nettest`, func(t testutil.T) { grpctest.RC_NetConn_Test_nettest(t, factory) })
			})
		})
	}
}

func Test_clientCancel(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, time.Second*10)

	cin := make(chan struct{})
	cout := make(chan struct{})
	factory := tunnelCCFactory(func(factory testutil.ClientConnFactory) testutil.ClientConnFactory {
		return func(fn func(h testutil.GRPCServer)) testutil.ClientConnCloser {
			conn := factory(fn)
			conn = &debugClientConnSends{
				clientConnCloserI: conn,
				hook: func(msg any) {
					if _, ok := msg.(*grpctunnel.ClientToServer).GetFrame().(*grpctunnel.ClientToServer_Cancel); ok {
						cin <- struct{}{}
						<-cout
					}
				},
			}
			return conn
		}
	}(testutil.NetpipeClientConnFactory))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	local, remote := net.Pipe()
	remoteClosed := make(chan struct{})
	go func() {
		defer close(remoteClosed)
		_, _ = remote.Read(make([]byte, 1))
	}()
	dialerCtxCh := make(chan context.Context, 1)
	dialer := mockDialer{dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		dialerCtxCh <- ctx
		return local, nil
	}}
	rcServer := netconn.Server{Dialer: func(*rc.NetConnRequest_Dial) (netconn.Dialer, error) { return &dialer, nil }}
	rcConn := factory(func(h testutil.GRPCServer) { rc.RegisterRemoteControlServer(h, &rcServer) })
	rcStub := rc.NewRemoteControlClient(rcConn)
	rcStream, err := rcStub.NetConn(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := rcStream.Send(&rc.NetConnRequest{Data: &rc.NetConnRequest_Dial_{Dial: &rc.NetConnRequest_Dial{}}}); err != nil {
		t.Fatal(err)
	}

	dialerCtx := <-dialerCtxCh

	if res, err := rcStream.Recv(); err != nil {
		t.Fatal(err)
	} else if res.GetConn().GetLocal().GetAddress() != `pipe` {
		t.Fatal(res)
	}

	time.Sleep(time.Millisecond * 50)

	select {
	case <-remoteClosed:
		t.Fatal()
	case <-dialerCtx.Done():
		t.Fatal()
	default:
	}

	cancel()

	<-cin

	time.Sleep(time.Millisecond * 50)

	select {
	case <-remoteClosed:
		t.Fatal()
	case <-dialerCtx.Done():
		t.Fatal()
	default:
	}

	cout <- struct{}{}

	<-remoteClosed
	<-dialerCtx.Done()

	if err := rcConn.Close(); err != nil {
		t.Error(err)
	}
}

func (x *mockDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return x.dialContext(ctx, network, address)
}

func (x *debugNetConnWrites) Write(b []byte) (int, error) {
	x.hook(b)
	return x.netConnI.Write(b)
}

func (x *debugClientConnSends) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (s grpc.ClientStream, err error) {
	s, err = x.clientConnCloserI.NewStream(ctx, desc, method, opts...)
	if s != nil {
		s = &debugClientStreamSends{
			clientStreamI: s,
			hook:          x.hook,
		}
	}
	return
}

func (x *debugClientStreamSends) SendMsg(m interface{}) error {
	x.hook(m)
	return x.clientStreamI.SendMsg(m)
}
