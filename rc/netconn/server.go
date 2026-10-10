package netconn

import (
	"context"
	"fmt"
	"io"
	"net"

	"google.golang.org/protobuf/proto"

	grpcstream "github.com/joeycumines/sesame/grpc"
	"github.com/joeycumines/sesame/rc"
	sesameproxy "github.com/joeycumines/sesame/rc/proxy"
	sesametls "github.com/joeycumines/sesame/rc/tls"
	streamutil "github.com/joeycumines/sesame/stream"
	"github.com/joeycumines/sesame/type/netaddr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type (
	// Server implements rc.RemoteControlServer's NetConn method.
	Server struct {
		Dialer       DialerFactory
		Capabilities *rc.NetConnResponse_Capabilities
		TLSProvider  TLSProvider
		//lint:ignore U1000 it is actually used
		unimplementedRemoteControlServer
	}

	// ServerAPI models a subset of rc.RemoteControlServer, as implemented by Server.
	ServerAPI interface {
		NetConn(stream rc.RemoteControl_NetConnServer) error
	}

	// DialerFactory prepares a Dialer based on a dial request.
	// See also DefaultDialer.
	DialerFactory func(req *rc.NetConnRequest_Dial) (Dialer, error)

	// Dialer models an implementation like net.Dialer.
	// See also DialerFactory.
	Dialer interface {
		DialContext(ctx context.Context, network, address string) (net.Conn, error)
	}

	//lint:ignore U1000 it is actually used
	unimplementedRemoteControlServer = rc.UnimplementedRemoteControlServer
)

var (
	// DefaultDialer will be used by Server.NetConn if Server.Dialer is nil.
	DefaultDialer DialerFactory = defaultDialer

	// compile time assertions

	_ rc.RemoteControlServer = (*Server)(nil)
	_ ServerAPI              = (*Server)(nil)
)

func (x *Server) NetConn(stream rc.RemoteControl_NetConnServer) error {
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

	// Validate advertised capabilities at the API boundary: negative
	// window/chunk values are malformed input, and rejecting them before
	// any dialing or stream output keeps the failure cheap and precise.
	if caps := dialReq.GetCapabilities(); caps != nil {
		if caps.GetInitialWindowSize() < 0 || caps.GetMaxChunkSize() < 0 {
			return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: negative capability value in dial request")
		}
	}

	// Validate the TLS version range at the API boundary, before any
	// dialing: a floor above its ceiling can never negotiate, and input
	// validation must precede side effects. ExecuteTLSHandshake re-checks
	// (covering the in-stream upgrade path and direct callers).
	if tlsOpts := dialReq.GetTls(); tlsOpts != nil {
		if minV, maxV := tlsOpts.GetMinVersion(), tlsOpts.GetMaxVersion(); minV != 0 && maxV != 0 && minV > maxV {
			return status.Error(codes.InvalidArgument, "sesame/rc/netconn: min_version exceeds max_version")
		}
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
		proxyResult *sesameproxy.ProxyResult
		tlsResult   *sesametls.TLSHandshakeResult
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
			return dialErr
		}
	}
	defer conn.Close()

	if dialReq.GetTls() != nil {
		var tlsErr error
		conn, tlsResult, tlsErr = ExecuteTLSHandshake(ctx, conn, dialReq.GetTls(), targetAddr, x.TLSProvider)
		if tlsErr != nil {
			return tlsErr
		}
		defer conn.Close()
	}

	serverCaps := x.Capabilities
	if serverCaps == nil {
		serverCaps = &rc.NetConnResponse_Capabilities{
			SupportsFlowControl:      true,
			SupportsOpportunisticTls: true,
			MaxChunkSize:             DefaultChunkSize,
			InitialWindowSize:        DefaultInitialWindowSize,
			ClientHelloCapabilities:  GoClientHelloCapabilities(),
		}
	}

	serverCaps = proto.Clone(serverCaps).(*rc.NetConnResponse_Capabilities)
	if serverCaps.GetInitialWindowSize() == 0 {
		serverCaps.InitialWindowSize = DefaultInitialWindowSize
	}
	if serverCaps.GetMaxChunkSize() == 0 {
		serverCaps.MaxChunkSize = DefaultChunkSize
	}

	// 2. NetConnResponse.conn
	sendErr := stream.Send(&rc.NetConnResponse{Data: &rc.NetConnResponse_Conn_{Conn: &rc.NetConnResponse_Conn{
		Local:        netaddr.New(conn.LocalAddr()),
		Remote:       netaddr.New(conn.RemoteAddr()),
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

func (x *Server) dial(req *rc.NetConnRequest_Dial) (Dialer, error) {
	if x.Dialer != nil {
		return x.Dialer(req)
	}
	return DefaultDialer(req)
}

func defaultDialer(req *rc.NetConnRequest_Dial) (Dialer, error) {
	// TODO smarter (grpc) errors for both this func and the returned dialer
	return &net.Dialer{
		Timeout: req.GetTimeout().AsDuration(),
	}, nil
}

func newStreamServerReader(stream rc.RemoteControl_NetConnServer) *grpcstream.Reader {
	return &grpcstream.Reader{
		Stream: stream,
		Factory: grpcstream.NewReaderMessageFactory(func() (value interface{}, chunk func() ([]byte, bool)) {
			var msg rc.NetConnRequest
			value = &msg
			chunk = func() ([]byte, bool) {
				if v, ok := msg.GetData().(*rc.NetConnRequest_Bytes); ok {
					return v.Bytes, true
				}
				return nil, false
			}
			return
		}),
	}
}

func newStreamServerWriter(stream rc.RemoteControl_NetConnServer) io.Writer {
	return streamutil.ChunkWriter(func(b []byte) (int, error) {
		if err := stream.Send(&rc.NetConnResponse{Data: &rc.NetConnResponse_Bytes{Bytes: b}}); err != nil {
			return 0, err
		}
		return len(b), nil
	})
}
