package netconn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joeycumines/sesame/rc"
	sesameproxy "github.com/joeycumines/sesame/type/proxy"
	sesametls "github.com/joeycumines/sesame/type/tls"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

type (
	// InStreamConn extends net.Conn with in-stream transformation and control methods.
	InStreamConn interface {
		net.Conn
		TLSResult() *sesametls.TLSHandshakeResult
		ProxyResult() *sesameproxy.ProxyResult
		ServerCapabilities() *rc.NetConnResponse_Capabilities
		UpgradeTLS(ctx context.Context, opts *sesametls.TLSOptions) (*sesametls.TLSHandshakeResult, error)
		Ping(ctx context.Context) (time.Duration, error)
		CloseWrite() error
	}

	clientControlConn struct {
		stream      rc.RemoteControl_NetConnClient
		cancel      context.CancelFunc
		tlsResult   *sesametls.TLSHandshakeResult
		proxyResult *sesameproxy.ProxyResult
		serverCaps  *rc.NetConnResponse_Capabilities
		localAddr   net.Addr
		remoteAddr  net.Addr

		pipeReader *io.PipeReader
		pipeWriter *io.PipeWriter

		sendMu     sync.Mutex
		outboundFC *FlowController
		inboundFC  *FlowController

		pingCounter  atomic.Uint64
		pingsMu      sync.Mutex
		pendingPings map[uint64]chan int64

		upgradeMu      sync.Mutex
		pendingUpgrade chan upgradeResult

		closed atomic.Bool
	}

	upgradeResult struct {
		result *sesametls.TLSHandshakeResult
		err    error
	}
)

var _ InStreamConn = (*clientControlConn)(nil)

// NewClientControlConn constructs a client-side connection wrapper with full in-stream control demultiplexing.
func NewClientControlConn(
	ctx context.Context,
	cancel context.CancelFunc,
	stream rc.RemoteControl_NetConnClient,
	connRes *rc.NetConnResponse_Conn,
	clientCaps *rc.NetConnRequest_Capabilities,
) InStreamConn {
	pr, pw := io.Pipe()

	cc := &clientControlConn{
		stream:       stream,
		cancel:       cancel,
		tlsResult:    connRes.GetTls(),
		proxyResult:  connRes.GetProxy(),
		serverCaps:   connRes.GetCapabilities(),
		localAddr:    connRes.GetLocal().AsGoNetAddr(),
		remoteAddr:   connRes.GetRemote().AsGoNetAddr(),
		pipeReader:   pr,
		pipeWriter:   pw,
		pendingPings: make(map[uint64]chan int64),
	}

	if clientCaps.GetSupportsFlowControl() && connRes.GetCapabilities().GetSupportsFlowControl() {
		serverInitWin := connRes.GetCapabilities().GetInitialWindowSize()
		if serverInitWin == 0 {
			serverInitWin = DefaultInitialWindowSize
		}
		cc.outboundFC = NewFlowController(serverInitWin)

		clientInitWin := clientCaps.GetInitialWindowSize()
		if clientInitWin == 0 {
			clientInitWin = DefaultInitialWindowSize
		}
		cc.inboundFC = NewFlowController(clientInitWin)
	}

	go cc.readLoop(ctx)

	return cc
}

func (c *clientControlConn) readLoop(ctx context.Context) {
	for {
		res, err := c.stream.Recv()
		if err != nil {
			_ = c.pipeWriter.CloseWithError(err)
			c.abortPending(err)
			return
		}

		switch data := res.GetData().(type) {
		case *rc.NetConnResponse_Bytes:
			if len(data.Bytes) > 0 {
				rem := data.Bytes
				for len(rem) > 0 && !c.closed.Load() {
					writeLen := len(rem)
					if c.inboundFC != nil {
						var err error
						// Use the loop's context, not context.Background():
						// this blocks until the application consumes the
						// previous Read, and only Close (which cancels this
						// context) can be relied on to release it.
						writeLen, err = c.inboundFC.AcquirePartial(ctx, writeLen)
						if err != nil {
							_ = c.pipeWriter.CloseWithError(err)
							c.abortPending(err)
							return
						}
						if writeLen <= 0 {
							// Zero credit would leave rem unchanged and spin.
							err = io.ErrNoProgress
							_ = c.pipeWriter.CloseWithError(err)
							c.abortPending(err)
							return
						}
					}
					n, writeErr := c.pipeWriter.Write(rem[:writeLen])
					rem = rem[n:]
					if writeErr != nil {
						_ = c.pipeWriter.CloseWithError(writeErr)
						c.abortPending(writeErr)
						return
					}
					if c.inboundFC != nil && n > 0 {
						c.sendMu.Lock()
						sendErr := c.stream.Send(&rc.NetConnRequest{
							Data: &rc.NetConnRequest_Control_{
								Control: &rc.NetConnRequest_Control{
									Action: &rc.NetConnRequest_Control_WindowUpdate_{
										WindowUpdate: &rc.NetConnRequest_Control_WindowUpdate{
											CreditBytes: uint32(n),
										},
									},
								},
							},
						})
						c.sendMu.Unlock()
						if sendErr != nil {
							_ = c.pipeWriter.CloseWithError(sendErr)
							c.abortPending(sendErr)
							return
						}
					}
				}
			}

		case *rc.NetConnResponse_Control_:
			ctl := data.Control
			if ctl == nil {
				continue
			}

			switch event := ctl.GetEvent().(type) {
			case *rc.NetConnResponse_Control_TlsUpgraded:
				c.upgradeMu.Lock()
				ch := c.pendingUpgrade
				c.pendingUpgrade = nil
				c.upgradeMu.Unlock()
				if ch != nil {
					c.tlsResult = event.TlsUpgraded.GetResult()
					ch <- upgradeResult{result: event.TlsUpgraded.GetResult()}
				}

			case *rc.NetConnResponse_Control_TlsUpgradeFailed:
				c.upgradeMu.Lock()
				ch := c.pendingUpgrade
				c.pendingUpgrade = nil
				c.upgradeMu.Unlock()
				if ch != nil {
					errMsg := "sesame/rc/netconn: TLS upgrade failed"
					if st := event.TlsUpgradeFailed.GetError(); st != nil {
						errMsg = fmt.Sprintf("sesame/rc/netconn: TLS upgrade failed (code %d): %s", st.GetCode(), st.GetMessage())
					}
					ch <- upgradeResult{err: errors.New(errMsg)}
				}

			case *rc.NetConnResponse_Control_Pong_:
				c.pingsMu.Lock()
				ch := c.pendingPings[event.Pong.GetId()]
				delete(c.pendingPings, event.Pong.GetId())
				c.pingsMu.Unlock()
				if ch != nil {
					ch <- event.Pong.GetTimestampNs()
				}

			case *rc.NetConnResponse_Control_HalfClose_:
				_ = c.pipeWriter.Close()

			case *rc.NetConnResponse_Control_WindowUpdate_:
				if c.outboundFC != nil {
					c.outboundFC.AddCredit(event.WindowUpdate.GetCreditBytes())
				}
			}
		}
	}
}

func (c *clientControlConn) abortPending(err error) {
	c.pingsMu.Lock()
	for id, ch := range c.pendingPings {
		close(ch)
		delete(c.pendingPings, id)
	}
	c.pingsMu.Unlock()

	c.upgradeMu.Lock()
	if c.pendingUpgrade != nil {
		c.pendingUpgrade <- upgradeResult{err: err}
		c.pendingUpgrade = nil
	}
	c.upgradeMu.Unlock()
}

func (c *clientControlConn) Read(b []byte) (int, error) {
	n, err := c.pipeReader.Read(b)
	if n > 0 && c.inboundFC != nil {
		c.inboundFC.AddCredit(uint32(n))
	}
	return n, err
}

func (c *clientControlConn) Write(b []byte) (int, error) {
	if c.closed.Load() {
		return 0, io.ErrClosedPipe
	}

	rem := b
	var totalWritten int

	for len(rem) > 0 {
		sendLen := len(rem)
		if c.outboundFC != nil {
			var err error
			sendLen, err = c.outboundFC.AcquirePartial(context.Background(), sendLen)
			if err != nil {
				return totalWritten, err
			}
		}

		chunk := rem[:sendLen]
		rem = rem[sendLen:]

		c.sendMu.Lock()
		err := c.stream.Send(&rc.NetConnRequest{
			Data: &rc.NetConnRequest_Bytes{Bytes: chunk},
		})
		c.sendMu.Unlock()
		if err != nil {
			return totalWritten, err
		}

		totalWritten += sendLen
	}

	return totalWritten, nil
}

func (c *clientControlConn) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		if c.cancel != nil {
			c.cancel()
		}
		_ = c.pipeReader.Close()
		_ = c.pipeWriter.Close()
		if c.outboundFC != nil {
			c.outboundFC.Close()
		}
		if c.inboundFC != nil {
			c.inboundFC.Close()
		}
		c.abortPending(io.ErrClosedPipe)
	}
	return nil
}

func (c *clientControlConn) CloseWrite() error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	return c.stream.Send(&rc.NetConnRequest{
		Data: &rc.NetConnRequest_Control_{
			Control: &rc.NetConnRequest_Control{
				Action: &rc.NetConnRequest_Control_HalfClose_{
					HalfClose: &rc.NetConnRequest_Control_HalfClose{},
				},
			},
		},
	})
}

func (c *clientControlConn) UpgradeTLS(ctx context.Context, opts *sesametls.TLSOptions) (*sesametls.TLSHandshakeResult, error) {
	if !c.serverCaps.GetSupportsOpportunisticTls() {
		return nil, statusError(codes.FailedPrecondition, "sesame/rc/netconn: server does not advertise support for opportunistic TLS")
	}

	ch := make(chan upgradeResult, 1)

	c.upgradeMu.Lock()
	if c.pendingUpgrade != nil {
		c.upgradeMu.Unlock()
		return nil, statusError(codes.AlreadyExists, "sesame/rc/netconn: another TLS upgrade is already pending")
	}
	c.pendingUpgrade = ch
	c.upgradeMu.Unlock()

	c.sendMu.Lock()
	sendErr := c.stream.Send(&rc.NetConnRequest{
		Data: &rc.NetConnRequest_Control_{
			Control: &rc.NetConnRequest_Control{
				Action: &rc.NetConnRequest_Control_UpgradeTls{
					UpgradeTls: &rc.NetConnRequest_Control_UpgradeTLS{
						Options: opts,
					},
				},
			},
		},
	})
	c.sendMu.Unlock()

	if sendErr != nil {
		c.upgradeMu.Lock()
		c.pendingUpgrade = nil
		c.upgradeMu.Unlock()
		return nil, sendErr
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res.result, res.err
	}
}

func (c *clientControlConn) Ping(ctx context.Context) (time.Duration, error) {
	id := c.pingCounter.Add(1)
	start := time.Now()
	timestampNs := start.UnixNano()

	ch := make(chan int64, 1)

	c.pingsMu.Lock()
	c.pendingPings[id] = ch
	c.pingsMu.Unlock()

	c.sendMu.Lock()
	sendErr := c.stream.Send(&rc.NetConnRequest{
		Data: &rc.NetConnRequest_Control_{
			Control: &rc.NetConnRequest_Control{
				Action: &rc.NetConnRequest_Control_Ping_{
					Ping: &rc.NetConnRequest_Control_Ping{
						Id:          id,
						TimestampNs: timestampNs,
					},
				},
			},
		},
	})
	c.sendMu.Unlock()

	if sendErr != nil {
		c.pingsMu.Lock()
		delete(c.pendingPings, id)
		c.pingsMu.Unlock()
		return 0, sendErr
	}

	select {
	case <-ctx.Done():
		c.pingsMu.Lock()
		delete(c.pendingPings, id)
		c.pingsMu.Unlock()
		return 0, ctx.Err()
	case _, ok := <-ch:
		if !ok {
			return 0, io.ErrClosedPipe
		}
		return time.Since(start), nil
	}
}

func (c *clientControlConn) TLSResult() *sesametls.TLSHandshakeResult {
	return c.tlsResult
}

func (c *clientControlConn) TLSHandshakeResult() *sesametls.TLSHandshakeResult {
	return c.tlsResult
}

func (c *clientControlConn) ProxyResult() *sesameproxy.ProxyResult {
	return c.proxyResult
}

func (c *clientControlConn) ServerCapabilities() *rc.NetConnResponse_Capabilities {
	return c.serverCaps
}

func (c *clientControlConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *clientControlConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *clientControlConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *clientControlConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *clientControlConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func statusError(code codes.Code, msg string) error {
	return grpcstatus.Error(code, msg)
}
