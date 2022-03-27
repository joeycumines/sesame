package netconn

import (
	"context"
	"fmt"
	"github.com/joeycumines/sesame/genproto/type/netaddr"
	grpcstream "github.com/joeycumines/sesame/grpc"
	"github.com/joeycumines/sesame/ionet"
	"github.com/joeycumines/sesame/rc"
	streamutil "github.com/joeycumines/sesame/stream"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type (
	// Client implements Dialer using rc.RemoteControlClient's NetConn method.
	Client struct {
		API         ClientAPI
		Timeout     time.Duration
		ClosePolicy streamutil.ClosePolicy
	}

	// ClientAPI models a subset of rc.RemoteControlClient, as used by Client.
	ClientAPI interface {
		NetConn(ctx context.Context, opts ...grpc.CallOption) (rc.RemoteControl_NetConnClient, error)
	}

	netConn struct {
		netConnI
		req *rc.NetConnRequest_Dial
		res *rc.NetConnResponse_Conn
	}

	clientReader struct{ ioReader }

	clientWriter struct {
		ioWriter
		stream rc.RemoteControl_NetConnClient
	}

	netConnI net.Conn

	ioReader io.Reader

	ioWriter io.Writer
)

var (
	// compile time assertions

	_ net.Conn = (*netConn)(nil)
	_ Dialer   = (*Client)(nil)
	_          = Client{API: rc.RemoteControlClient(nil)}
)

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

	conn := netConn{req: &rc.NetConnRequest_Dial{Address: &netaddr.NetAddr{
		Network: network,
		Address: address,
	}}}
	if x.Timeout > 0 {
		conn.req.Timeout = durationpb.New(x.Timeout)
	}

	// 1. NetConnRequest.dial
	if err := stream.Send(&rc.NetConnRequest{Data: &rc.NetConnRequest_Dial_{Dial: conn.req}}); err != nil {
		return nil, err
	}

	// 2. NetConnResponse.conn
	if res, err := stream.Recv(); err != nil {
		return nil, err
	} else if conn.res = res.GetConn(); conn.res == nil {
		return nil, fmt.Errorf(`sesame/rc/netconn: unexpected response: %T`, res.GetData())
	}

	// encapsulates the remaining stages:
	// 3. Any number of NetConnRequest.bytes and NetConnResponse.bytes
	// 4. Termination
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
		streamutil.OptHalfCloser.ClosePolicy(x.ClosePolicy),
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

func (x *netConn) String() string {
	var s strings.Builder
	s.WriteString(rc.RemoteControl_ServiceDesc.ServiceName)
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

func newStreamClientReader(stream rc.RemoteControl_NetConnClient) *grpcstream.Reader {
	return &grpcstream.Reader{
		Stream: stream,
		Factory: grpcstream.NewReaderMessageFactory(func() (value interface{}, chunk func() ([]byte, bool)) {
			var msg rc.NetConnResponse
			value = &msg
			chunk = func() ([]byte, bool) {
				if v, ok := msg.GetData().(*rc.NetConnResponse_Bytes); ok {
					return v.Bytes, true
				}
				return nil, false
			}
			return
		}),
	}
}

func newStreamClientWriter(stream rc.RemoteControl_NetConnClient) io.Writer {
	return streamutil.ChunkWriter(func(b []byte) (int, error) {
		if err := stream.Send(&rc.NetConnRequest{Data: &rc.NetConnRequest_Bytes{Bytes: b}}); err != nil {
			return 0, err
		}
		return len(b), nil
	})
}
