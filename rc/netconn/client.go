package netconn

import (
	"context"
	"fmt"
	grpcstream "github.com/joeycumines/sesame/grpc"
	"github.com/joeycumines/sesame/ionet"
	sesamepb "github.com/joeycumines/sesame/sesamepb"
	streamutil "github.com/joeycumines/sesame/stream"
	typepb "github.com/joeycumines/sesame/typepb"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type (
	// Client implements Dialer using sesamepb.RemoteControlClient's NetConn method.
	//
	// ClosePolicy configures the legacy (capability-less) conn's close
	// behavior. Unset, it defaults to a graceful close bounded by
	// DefaultCloseDrainTimeout: drain buffered response data, then
	// cancel. Control-mode conns (Capabilities set) cancel immediately
	// on Close regardless of this field.
	Client struct {
		API          ClientAPI
		Timeout      time.Duration
		ClosePolicy  streamutil.ClosePolicy
		TLS          *sesamepb.TlsOptions
		Proxy        *sesamepb.ProxyOptions
		Capabilities *sesamepb.NetConnRequest_Capabilities
	}
	// ClientAPI models a subset of sesamepb.RemoteControlClient, as used by Client.
	ClientAPI interface {
		NetConn(ctx context.Context, opts ...grpc.CallOption) (sesamepb.RemoteControl_NetConnClient, error)
	}

	netConn struct {
		netConnI
		req *sesamepb.NetConnRequest_Dial
		res *sesamepb.NetConnResponse_Conn
	}

	clientReader struct{ ioReader }

	clientWriter struct {
		ioWriter
		stream sesamepb.RemoteControl_NetConnClient
	}

	netConnI net.Conn

	ioReader io.Reader

	ioWriter io.Writer
)

var (
	// compile time assertions

	_ net.Conn            = (*netConn)(nil)
	_ Dialer              = (*Client)(nil)
	_ ConnTransformResult = (*netConn)(nil)
	_                     = Client{API: sesamepb.RemoteControlClient(nil)}
)

// DefaultCloseDrainTimeout bounds the legacy (capability-less) conn's
// graceful close when Client.ClosePolicy is unset. The drain normally
// completes as soon as the server's relay finishes; the bound only
// fires when the relay cannot finish - e.g. the peer's socket is
// unread and is closed only after this conn's Close returns - a state
// in which an unbounded drain would hang Close forever.
const DefaultCloseDrainTimeout = 5 * time.Second

func (x *Client) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var success bool
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if !success {
			cancel()
		}
	}()

	stream, err := x.API.NetConn(ctx)
	if err != nil {
		return nil, err
	}

	conn := netConn{req: &sesamepb.NetConnRequest_Dial{
		Address: &typepb.NetAddr{
			Network: network,
			Address: address,
		},
		Tls:          x.TLS,
		Proxy:        x.Proxy,
		Capabilities: x.Capabilities,
	}}
	if x.Timeout > 0 {
		conn.req.Timeout = durationpb.New(x.Timeout)
	}

	// 1. NetConnRequest.dial
	if err := stream.Send(&sesamepb.NetConnRequest{Data: &sesamepb.NetConnRequest_Dial_{Dial: conn.req}}); err != nil {
		return nil, err
	}

	// 2. NetConnResponse.conn
	if res, err := stream.Recv(); err != nil {
		return nil, err
	} else if conn.res = res.GetConn(); conn.res == nil {
		return nil, fmt.Errorf(`sesame/rc/netconn: unexpected response: %T`, res.GetData())
	}

	// Fail-closed security invariants
	if x.TLS != nil {
		if conn.res.GetTls() == nil {
			return nil, fmt.Errorf("sesame/rc/netconn: security violation: server returned cleartext connection when TLS was requested")
		}
		// Verify applied_client_hello matches the requested spec.
		if err := VerifyAppliedClientHello(x.TLS.GetClientHello(), conn.res.GetTls().GetAppliedClientHello()); err != nil {
			return nil, err
		}
	}

	// Negative server-advertised window/chunk values are protocol
	// violations: fail the dial rather than clamping them into a zero
	// window that would stall writes indefinitely.
	if caps := conn.res.GetCapabilities(); caps != nil {
		if caps.GetInitialWindowSize() < 0 || caps.GetMaxChunkSize() < 0 {
			return nil, fmt.Errorf("sesame/rc/netconn: protocol violation: negative capability value in conn response")
		}
	}

	if x.Proxy != nil && len(x.Proxy.GetHops()) > 0 {
		if conn.res.GetProxy() == nil {
			return nil, fmt.Errorf("sesame/rc/netconn: security violation: server returned unproxied connection when proxy was requested")
		}
		if len(conn.res.GetProxy().GetTraversedHops()) != len(x.Proxy.GetHops()) {
			return nil, fmt.Errorf("sesame/rc/netconn: proxy hop count mismatch: requested %d hops, server traversed %d",
				len(x.Proxy.GetHops()), len(conn.res.GetProxy().GetTraversedHops()))
		}
	}

	// If in-stream capabilities were requested, return full control wrapper
	if x.Capabilities != nil {
		success = true
		return NewClientControlConn(ctx, cancel, stream, conn.res, x.Capabilities), nil
	}

	// encapsulates the remaining stages for legacy connections:
	// 3. Any number of NetConnRequest.bytes and NetConnResponse.bytes
	// 4. Termination
	closePolicy := x.ClosePolicy
	if closePolicy == nil {
		closePolicy = streamutil.WaitRemoteTimeout(DefaultCloseDrainTimeout)
	}
	wp, err := ionet.WrapPipeGraceful(
		streamutil.OptHalfCloser.Pipe(streamutil.Pipe{
			Reader: &clientReader{ioReader: newStreamClientReader(stream)},
			Writer: &clientWriter{
				ioWriter: newStreamClientWriter(stream),
				stream:   stream,
			},
			Closer: streamutil.Closer(func() error {
				cancel()
				return nil
			}),
		}),
		streamutil.OptHalfCloser.CloseGuard(func() bool { return ctx.Err() == nil }),
		streamutil.OptHalfCloser.ClosePolicy(closePolicy),
	)
	if err != nil {
		return nil, err
	}

	conn.netConnI = wp

	success = true

	return &conn, nil
}

func (x *netConn) LocalAddr() net.Addr { return x.res.GetLocal().AsGoNetAddr() }

func (x *netConn) RemoteAddr() net.Addr { return x.res.GetRemote().AsGoNetAddr() }

func (x *netConn) TlsHandshakeResult() *sesamepb.TlsHandshakeResult { return x.res.GetTls() }

func (x *netConn) ProxyResult() *sesamepb.ProxyResult { return x.res.GetProxy() }

func (x *netConn) ServerCapabilities() *sesamepb.NetConnResponse_Capabilities {
	return x.res.GetCapabilities()
}

func (x *netConn) String() string {
	var s strings.Builder
	s.WriteString(sesamepb.RemoteControl_ServiceDesc.ServiceName)
	s.WriteString(`.NetConn(`)
	w := func() func(k string, v bool) {
		var ok bool
		return func(k string, v bool) {
			if ok {
				s.WriteByte(' ')
			} else {
				ok = true
			}
			s.WriteString(k)
			if v {
				s.WriteByte('=')
			}
		}
	}()
	if v := x.req.GetAddress().GetNetwork(); v != `` {
		w(`req_net`, true)
		s.WriteString(strconv.Quote(v))
	}
	if v := x.req.GetAddress().GetAddress(); v != `` {
		w(`req_addr`, true)
		s.WriteString(strconv.Quote(v))
	}
	if v := x.req.GetTimeout().AsDuration(); v > 0 {
		w(`req_timeout`, true)
		s.WriteString(v.String())
	}
	if v := x.res.GetLocal().GetNetwork(); v != `` {
		w(`local_net`, true)
		s.WriteString(strconv.Quote(v))
	}
	if v := x.res.GetLocal().GetAddress(); v != `` {
		w(`local_addr`, true)
		s.WriteString(strconv.Quote(v))
	}
	if v := x.res.GetRemote().GetNetwork(); v != `` {
		w(`remote_net`, true)
		s.WriteString(strconv.Quote(v))
	}
	if v := x.res.GetRemote().GetAddress(); v != `` {
		w(`remote_addr`, true)
		s.WriteString(strconv.Quote(v))
	}
	s.WriteByte(')')
	return s.String()
}

func (x *clientWriter) Close() error { return x.CloseWithError(nil) }

func (x *clientWriter) CloseWithError(err error) error {
	if err != nil {
		return fmt.Errorf(`sesame/rc/netconn: clientWriter close with non-nil error not supported: %s`, err)
	}
	return x.stream.CloseSend()
}

func (x *clientReader) Close() error { return x.CloseWithError(nil) }

func (x *clientReader) CloseWithError(error) error { return nil }

func newStreamClientReader(stream sesamepb.RemoteControl_NetConnClient) *grpcstream.Reader {
	return &grpcstream.Reader{
		Stream: stream,
		Factory: grpcstream.NewReaderMessageFactory(func() (value interface{}, chunk func() ([]byte, bool)) {
			var msg sesamepb.NetConnResponse
			value = &msg
			chunk = func() ([]byte, bool) {
				if v, ok := msg.GetData().(*sesamepb.NetConnResponse_Bytes); ok {
					return v.Bytes, true
				}
				return nil, false
			}
			return
		}),
	}
}

func newStreamClientWriter(stream sesamepb.RemoteControl_NetConnClient) io.Writer {
	return streamutil.ChunkWriter(func(b []byte) (int, error) {
		if err := stream.Send(&sesamepb.NetConnRequest{Data: &sesamepb.NetConnRequest_Bytes{Bytes: b}}); err != nil {
			return 0, err
		}
		return len(b), nil
	})
}
