package netconn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joeycumines/sesame/ionet"
	"github.com/joeycumines/sesame/rc"
	sesameproxy "github.com/joeycumines/sesame/rc/proxy"
	sesametls "github.com/joeycumines/sesame/rc/tls"
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
		ctx         context.Context
		tlsResult   *sesametls.TLSHandshakeResult
		proxyResult *sesameproxy.ProxyResult
		serverCaps  *rc.NetConnResponse_Capabilities
		localAddr   net.Addr
		remoteAddr  net.Addr

		// Inbound data flows readLoop -> pipe end A writer -> pipe end B
		// reader -> the application. ionet.ConnPipe is a deadline-capable
		// synchronous pipe (os.ErrDeadlineExceeded on expired deadlines),
		// unlike io.Pipe, giving clientControlConn real net.Conn deadlines.
		// Only the A->B direction carries data.
		//
		// demuxLocalW writes inbound bytes and propagates stream errors to
		// the app via CloseWithError (io.EOF for a clean HalfClose).
		// demuxRemote is the app's end: Read + SetReadDeadline, and Close()
		// closes BOTH directions so a blocked readLoop Write unblocks with
		// io.ErrClosedPipe (net.Pipe-equivalent full-close semantics).
		demuxLocalW *ionet.ConnPipeWriter
		demuxRemote *ionet.ConnPipe

		sendMu     sync.Mutex
		outboundFC *FlowController
		inboundFC  *FlowController

		// writeDeadline bounds the outboundFC.AcquirePartial wait in Write.
		// The gRPC stream.Send call itself is not deadline-interruptible
		// (residual, common to net.Pipe-class transports); only the
		// flow-control wait is.
		writeDeadlineMu sync.Mutex
		writeDeadline   time.Time

		pingCounter  atomic.Uint64
		pingsMu      sync.Mutex
		pendingPings map[uint64]chan int64

		upgradeMu      sync.Mutex
		pendingUpgrade chan upgradeResult

		// maxChunk bounds each NetConnRequest_Bytes payload to the
		// server-advertised max_chunk_size. A large flow-control window
		// must not produce gRPC messages that exceed the peer's receive
		// limit (grpc-go default 4MiB) or its advertised chunk contract.
		maxChunk int

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
	local, remote := ionet.Pipe()
	// local's SendPipe writer is the readLoop's write end; remote is the
	// application's end (Read + deadline + full Close).
	_, localWriter := local.SendPipe()

	cc := &clientControlConn{
		stream:       stream,
		cancel:       cancel,
		ctx:          ctx,
		tlsResult:    connRes.GetTls(),
		proxyResult:  connRes.GetProxy(),
		serverCaps:   connRes.GetCapabilities(),
		localAddr:    connRes.GetLocal().AsGoNetAddr(),
		remoteAddr:   connRes.GetRemote().AsGoNetAddr(),
		demuxLocalW:  localWriter,
		demuxRemote:  remote,
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

	// Chunk granularity must hold with or without flow control; an absent
	// advertisement falls back to DefaultChunkSize, matching the legacy
	// ChunkWriter path.
	cc.maxChunk = DefaultChunkSize
	if advertised := int(connRes.GetCapabilities().GetMaxChunkSize()); advertised > 0 {
		cc.maxChunk = advertised
	}

	go cc.readLoop(ctx)

	return cc
}

func (c *clientControlConn) readLoop(ctx context.Context) {
	for {
		res, err := c.stream.Recv()
		if err != nil {
			_ = c.demuxLocalW.CloseWithError(err)
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
							_ = c.demuxLocalW.CloseWithError(err)
							c.abortPending(err)
							return
						}
						if writeLen <= 0 {
							// Zero credit would leave rem unchanged and spin.
							err = io.ErrNoProgress
							_ = c.demuxLocalW.CloseWithError(err)
							c.abortPending(err)
							return
						}
					}
					n, writeErr := c.demuxLocalW.Write(rem[:writeLen])
					rem = rem[n:]
					if writeErr != nil {
						_ = c.demuxLocalW.CloseWithError(writeErr)
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
							_ = c.demuxLocalW.CloseWithError(sendErr)
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
				if ch != nil {
					c.tlsResult = event.TlsUpgraded.GetResult()
					// Send under the mutex: the buffered cap-1 channel and
					// the single-send-per-slot invariant make this
					// non-blocking, and it linearizes result delivery
					// against a concurrently timing-out UpgradeTLS - the
					// result is either honored by its drain or the slot
					// is already nil and we poison. Never both nor neither.
					ch <- upgradeResult{result: event.TlsUpgraded.GetResult()}
				}
				c.upgradeMu.Unlock()
				if ch == nil {
					// TlsUpgraded with no pending upgrade: the TLS state
					// of the connection is now unknowable (a previous
					// upgrade timed out and the server upgraded anyway, or
					// a duplicate/malicious event). Fail closed: tear the
					// connection down rather than let it continue in a
					// state the caller cannot reason about.
					c.poison(errors.New("sesame/rc/netconn: unsolicited TlsUpgraded event"))
					return
				}

			case *rc.NetConnResponse_Control_TlsUpgradeFailed:
				c.upgradeMu.Lock()
				ch := c.pendingUpgrade
				c.pendingUpgrade = nil
				if ch != nil {
					errMsg := "sesame/rc/netconn: TLS upgrade failed"
					if st := event.TlsUpgradeFailed.GetError(); st != nil {
						errMsg = fmt.Sprintf("sesame/rc/netconn: TLS upgrade failed (code %d): %s", st.GetCode(), st.GetMessage())
					}
					// Under the mutex, for the same linearization as above
					// (cosmetic here - the server stays cleartext on
					// failure - but the pattern must not diverge).
					ch <- upgradeResult{err: errors.New(errMsg)}
				}
				c.upgradeMu.Unlock()
				if ch == nil {
					// Same unknowable-state reasoning as TlsUpgraded: a
					// failure result nobody is waiting for means lost
					// synchronization with the server's upgrade state.
					c.poison(errors.New("sesame/rc/netconn: unsolicited TlsUpgradeFailed event"))
					return
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
				// Clean EOF to the app; nil error stores io.EOF on the
				// pipe, matching io.Pipe Close semantics.
				_ = c.demuxLocalW.Close()

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

// poison tears the connection down without the Close() idempotence gate,
// for use by the readLoop when it encounters a state from which the
// connection can no longer be trusted (e.g. an unsolicited upgrade
// result). Mirrors Close()'s cleanup so every subsequent operation
// fails instead of continuing in an unknowable state.
func (c *clientControlConn) poison(err error) {
	if c.closed.CompareAndSwap(false, true) {
		if c.cancel != nil {
			c.cancel()
		}
		_ = c.demuxRemote.Close()
		_ = c.demuxLocalW.CloseWithError(err)
		if c.outboundFC != nil {
			c.outboundFC.Close()
		}
		if c.inboundFC != nil {
			c.inboundFC.Close()
		}
		c.abortPending(err)
	}
}

func (c *clientControlConn) Read(b []byte) (int, error) {
	n, err := c.demuxRemote.Read(b)
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
		// Clamp to the advertised chunk cap BEFORE acquiring credit so
		// any credit beyond this chunk stays available to later chunks
		// (AcquirePartial claims min(credit, want)).
		if sendLen > c.maxChunk {
			sendLen = c.maxChunk
		}
		if c.outboundFC != nil {
			var err error
			// Use the dial-derived context, not context.Background():
			// it is cancelled by Close and by the stream owner, so a
			// Write blocked on an exhausted window cannot outlive them.
			// A write deadline, when set, further bounds the credit
			// wait; the gRPC Send itself is not deadline-interruptible.
			c.writeDeadlineMu.Lock()
			deadline := c.writeDeadline
			c.writeDeadlineMu.Unlock()

			acquireCtx := c.ctx
			if !deadline.IsZero() {
				if !time.Now().Before(deadline) {
					// A past deadline fails immediately, matching
					// net.Conn semantics (testPastTimeout).
					return totalWritten, os.ErrDeadlineExceeded
				}
				var cancelAcquire context.CancelFunc
				acquireCtx, cancelAcquire = context.WithDeadline(c.ctx, deadline)
				sendLen, err = c.outboundFC.AcquirePartial(acquireCtx, sendLen)
				cancelAcquire()
				if errors.Is(err, context.DeadlineExceeded) && c.ctx.Err() == nil {
					// Only our per-write deadline fired; the dial
					// context is still alive. Report the net.Conn
					// standard error.
					err = os.ErrDeadlineExceeded
				}
			} else {
				sendLen, err = c.outboundFC.AcquirePartial(acquireCtx, sendLen)
			}
			if err != nil {
				return totalWritten, err
			}
			if sendLen <= 0 {
				// Zero credit would leave rem unchanged and spin.
				return totalWritten, io.ErrNoProgress
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
		_ = c.demuxRemote.Close()
		_ = c.demuxLocalW.Close()
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
	if c.closed.Load() {
		return nil, io.ErrClosedPipe
	}

	if !c.serverCaps.GetSupportsOpportunisticTls() {
		return nil, statusError(codes.FailedPrecondition, "sesame/rc/netconn: server does not advertise support for opportunistic TLS")
	}

	// Fail closed: without options there is no handshake to perform, and a
	// nil-options upgrade that "succeeds" would leave the connection
	// cleartext while the caller believes TLS is active.
	if opts == nil {
		return nil, statusError(codes.InvalidArgument, "sesame/rc/netconn: upgrade_tls requires options")
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
		// Clear the pending slot so a later UpgradeTLS does not fail
		// with AlreadyExists for a dead request. If the result raced
		// us and already arrived, it was buffered into ch - drain it.
		c.upgradeMu.Lock()
		c.pendingUpgrade = nil
		c.upgradeMu.Unlock()
		select {
		case res := <-ch:
			// The result arrived before the cancellation took effect;
			// honor it rather than the timeout.
			return res.result, res.err
		default:
		}
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
	// Guarded: readLoop writes tlsResult under upgradeMu when an upgrade
	// completes, and TLSResult may be called from any goroutine.
	c.upgradeMu.Lock()
	defer c.upgradeMu.Unlock()
	return c.tlsResult
}

func (c *clientControlConn) TLSHandshakeResult() *sesametls.TLSHandshakeResult {
	c.upgradeMu.Lock()
	defer c.upgradeMu.Unlock()
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
	err := c.SetReadDeadline(t)
	if werr := c.SetWriteDeadline(t); err == nil {
		err = werr
	}
	return err
}

func (c *clientControlConn) SetReadDeadline(t time.Time) error {
	// The app reads from the demux pipe's remote end, which has native
	// deadline support; an expired deadline makes a blocked Read return
	// os.ErrDeadlineExceeded.
	return c.demuxRemote.SetReadDeadline(t)
}

func (c *clientControlConn) SetWriteDeadline(t time.Time) error {
	// The gRPC stream.Send call is not deadline-interruptible, so a
	// write deadline can only bound the flow-control credit wait in
	// Write (a net.Pipe-class limitation, recorded in T3).
	c.writeDeadlineMu.Lock()
	c.writeDeadline = t
	c.writeDeadlineMu.Unlock()
	return nil
}

func statusError(code codes.Code, msg string) error {
	return grpcstatus.Error(code, msg)
}
