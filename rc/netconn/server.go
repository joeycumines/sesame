package netconn

import (
	"context"
	"fmt"
	"io"
	"net"

	"google.golang.org/protobuf/proto"

	grpcstream "github.com/joeycumines/sesame/grpc"
	sesamepb "github.com/joeycumines/sesame/sesamepb"
	streamutil "github.com/joeycumines/sesame/stream"
	typepb "github.com/joeycumines/sesame/typepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type (
	// Server implements sesamepb.RemoteControlServer's NetConn method.
	Server struct {
		Dialer DialerFactory
		// Capabilities advertises the server's support matrix. When nil,
		// a default advertisement is used (flow control + opportunistic
		// TLS, DefaultChunkSize, DefaultInitialWindowSize, and the Go
		// engine's honest ClientHello capabilities).
		//
		// Per the wire contract (remotecontrol.proto), advertised chunk
		// and window sizes MUST NOT exceed the stream's gRPC
		// per-message receive limit (grpc-go default 4MiB): a peer
		// honoring an oversized advertisement fails at runtime on
		// messages its gRPC stack refuses to receive. The handler cannot
		// discover the host's configured limit, so correctness here is
		// the operator's responsibility.
		Capabilities *sesamepb.NetConnResponse_Capabilities
		TLSProvider  TLSProvider
		//lint:ignore U1000 it is actually used
		unimplementedRemoteControlServer
	}

	// ServerAPI models a subset of sesamepb.RemoteControlServer, as implemented by Server.
	ServerAPI interface {
		NetConn(stream sesamepb.RemoteControl_NetConnServer) error
	}

	// DialerFactory prepares a Dialer based on a dial request.
	// See also DefaultDialer.
	DialerFactory func(req *sesamepb.NetConnRequest_Dial) (Dialer, error)

	// Dialer models an implementation like net.Dialer.
	// See also DialerFactory.
	Dialer interface {
		DialContext(ctx context.Context, network, address string) (net.Conn, error)
	}

	//lint:ignore U1000 it is actually used
	unimplementedRemoteControlServer = sesamepb.UnimplementedRemoteControlServer
)

var (
	// DefaultDialer will be used by Server.NetConn if Server.Dialer is nil.
	DefaultDialer DialerFactory = defaultDialer

	// compile time assertions

	_ sesamepb.RemoteControlServer = (*Server)(nil)
	_ ServerAPI                    = (*Server)(nil)
)

func (x *Server) NetConn(stream sesamepb.RemoteControl_NetConnServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	// 1. NetConnRequest.dial
	msg, err := stream.Recv()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		// code unknown
		return err
	}

	if msg.GetDial() == nil {
		return status.Errorf(codes.InvalidArgument, `sesame/rc/netconn: unexpected request: %T`, msg.GetData())
	}

	dialReq := msg.GetDial()

	// Input validation precedes side effects: capabilities, TLS range,
	// timeout, and target address are all client-controlled dial fields
	// (see validateDialRequest).
	if err := validateDialRequest(dialReq); err != nil {
		return err
	}

	// dialer factory handles timeout etc
	dialer, err := x.dial(dialReq)
	if err != nil {
		// code unknown, or provided by the dialer factory
		return err
	}

	// factory-provided dialer handles the actual dial operation (with optional proxy chaining)
	var (
		conn        net.Conn
		proxyResult *sesamepb.ProxyResult
		tlsResult   *sesamepb.TlsHandshakeResult
	)

	targetNet := dialReq.GetAddress().GetNetwork()
	targetAddr := dialReq.GetAddress().GetAddress()

	if dialReq.GetProxy() != nil && len(dialReq.GetProxy().GetHops()) > 0 {
		var hopErr error
		conn, proxyResult, hopErr = ExecuteProxyHops(ctx, dialer, dialReq.GetProxy(), targetNet, targetAddr)
		if hopErr != nil {
			return hopErr
		}
	} else {
		var dialErr error
		conn, dialErr = dialer.DialContext(ctx, targetNet, targetAddr)
		if dialErr != nil {
			return wrapDialError(dialErr, "sesame/rc/netconn: dial %s %s failed", targetNet, targetAddr)
		}
	}
	defer conn.Close()

	if dialReq.GetTls() != nil {
		// The requested dial timeout bounds the whole establishment,
		// including TLS termination - the tunnel is not "dialed" until
		// the handshake completes, mirroring the TypeScript reference.
		// The derived context is scoped to the handshake only; the
		// steady-state demux below keeps the stream context.
		hctx := ctx
		if t := dialReq.GetTimeout().AsDuration(); t > 0 {
			var hcancel context.CancelFunc
			hctx, hcancel = context.WithTimeout(ctx, t)
			defer hcancel()
		}
		var tlsErr error
		conn, tlsResult, tlsErr = ExecuteTLSHandshake(hctx, conn, dialReq.GetTls(), targetAddr, x.TLSProvider)
		if tlsErr != nil {
			return tlsErr
		}
		defer conn.Close()
	}

	serverCaps := x.Capabilities
	if serverCaps == nil {
		serverCaps = &sesamepb.NetConnResponse_Capabilities{
			SupportsFlowControl:      true,
			SupportsOpportunisticTls: true,
			MaxChunkSize:             DefaultChunkSize,
			InitialWindowSize:        DefaultInitialWindowSize,
			ClientHelloCapabilities:  GoClientHelloCapabilities(),
		}
	}

	serverCaps = proto.Clone(serverCaps).(*sesamepb.NetConnResponse_Capabilities)
	if serverCaps.GetInitialWindowSize() == 0 {
		serverCaps.InitialWindowSize = DefaultInitialWindowSize
	}
	if serverCaps.GetMaxChunkSize() == 0 {
		serverCaps.MaxChunkSize = DefaultChunkSize
	}

	// 2. NetConnResponse.conn
	sendErr := stream.Send(&sesamepb.NetConnResponse{Data: &sesamepb.NetConnResponse_Conn_{Conn: &sesamepb.NetConnResponse_Conn{
		Local:        typepb.New(conn.LocalAddr()),
		Remote:       typepb.New(conn.RemoteAddr()),
		Tls:          tlsResult,
		Proxy:        proxyResult,
		Capabilities: serverCaps,
	}}})
	if sendErr != nil {
		// code unknown
		return sendErr
	}

	// 3. Any number of NetConnRequest.bytes and NetConnResponse.bytes
	// until (at least the start of)
	// 4. Termination
	if dialReq.GetCapabilities() != nil {
		if err := RunServerDemux(ctx, stream, conn, serverCaps, dialReq.GetCapabilities(), x.TLSProvider, targetAddr); err != nil {
			return fmt.Errorf(`sesame/rc/netconn: copy stream error: %w`, err)
		}
	} else {
		type streamIO struct {
			ioReader
			ioWriter
		}
		if err := streamutil.Proxy(ctx, streamIO{newStreamServerReader(stream), newStreamServerWriter(stream)}, conn); err != nil {
			// code unknown
			return fmt.Errorf(`sesame/rc/netconn: copy stream error: %w`, err)
		}
	}

	// ensure successful flush e.g. buffered conns
	if err := conn.Close(); err != nil {
		// code unknown
		return err
	}

	return nil
}

func (x *Server) dial(req *sesamepb.NetConnRequest_Dial) (Dialer, error) {
	if x.Dialer != nil {
		return x.Dialer(req)
	}
	return DefaultDialer(req)
}

// validateDialRequest rejects malformed client input at the API
// boundary, before any dialer is built or side effect runs: negative
// advertised capabilities, an impossible TLS version range, a negative
// dial timeout, and a target address that is not a parseable host:port.
// Per AIP-193 such requests fail with INVALID_ARGUMENT instead of
// surfacing later as opaque errors from the dial path. Mirrors the
// boundary checks of the TypeScript reference's netConn handler, which
// has always required a host:port target.
func validateDialRequest(req *sesamepb.NetConnRequest_Dial) error {
	// Negative window/chunk values are malformed input, and rejecting
	// them before any dialing or stream output keeps the failure cheap
	// and precise.
	if caps := req.GetCapabilities(); caps != nil {
		if caps.GetInitialWindowSize() < 0 || caps.GetMaxChunkSize() < 0 {
			return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: negative capability value in dial request")
		}
	}

	// A TLS version floor above its ceiling can never negotiate, and
	// input validation must precede side effects. ExecuteTLSHandshake
	// re-checks (covering the in-stream upgrade path and direct
	// callers).
	if tlsOpts := req.GetTls(); tlsOpts != nil {
		if minV, maxV := tlsOpts.GetMinVersion(), tlsOpts.GetMaxVersion(); minV != 0 && maxV != 0 && minV > maxV {
			return status.Error(codes.InvalidArgument, "sesame/rc/netconn: min_version exceeds max_version")
		}
	}

	// A negative timeout is malformed input, not "unset": honoring it
	// would fail the dial immediately for a confusing reason.
	if t := req.GetTimeout().AsDuration(); t < 0 {
		return status.Error(codes.InvalidArgument, "sesame/rc/netconn: negative dial timeout")
	}

	// The target address is client-controlled: both stacks dial
	// host:port authorities, so anything else is rejected before any
	// dialing.
	return validateHostPort(req.GetAddress().GetAddress(), "target address")
}

func defaultDialer(req *sesamepb.NetConnRequest_Dial) (Dialer, error) {
	return &net.Dialer{
		Timeout: req.GetTimeout().AsDuration(),
	}, nil
}

func newStreamServerReader(stream sesamepb.RemoteControl_NetConnServer) *grpcstream.Reader {
	return &grpcstream.Reader{
		Stream: stream,
		Factory: grpcstream.NewReaderMessageFactory(func() (value interface{}, chunk func() ([]byte, bool)) {
			var msg sesamepb.NetConnRequest
			value = &msg
			chunk = func() ([]byte, bool) {
				if v, ok := msg.GetData().(*sesamepb.NetConnRequest_Bytes); ok {
					return v.Bytes, true
				}
				return nil, false
			}
			return
		}),
	}
}

func newStreamServerWriter(stream sesamepb.RemoteControl_NetConnServer) io.Writer {
	return streamutil.ChunkWriter(func(b []byte) (int, error) {
		if err := stream.Send(&sesamepb.NetConnResponse{Data: &sesamepb.NetConnResponse_Bytes{Bytes: b}}); err != nil {
			return 0, err
		}
		return len(b), nil
	})
}
