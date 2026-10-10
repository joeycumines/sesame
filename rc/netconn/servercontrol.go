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
	sesametls "github.com/joeycumines/sesame/rc/tls"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

const (
	// DefaultInitialWindowSize is the default flow control window (65,535 bytes).
	DefaultInitialWindowSize int32 = 65535

	// DefaultChunkSize is the maximum chunk size for streaming payload bytes.
	DefaultChunkSize = 32 * 1024
)

type (
	// FlowController implements credit-based stream flow control for RemoteControl.NetConn.
	FlowController struct {
		mu     sync.Mutex
		cond   *sync.Cond
		credit int64
		closed bool
	}

	// ServerControlHandler handles in-stream control demultiplexing for the server.
	serverControlState struct {
		stream            rc.RemoteControl_NetConnServer
		serverCaps        *rc.NetConnResponse_Capabilities
		clientCaps        *rc.NetConnRequest_Capabilities
		tlsProvider       TLSProvider
		defaultServerName string

		connMu     sync.RWMutex
		activeConn net.Conn
		sendMu     sync.Mutex

		outboundFC *FlowController
		inboundFC  *FlowController

		pauseReq     chan struct{}
		resumeNotify chan net.Conn

		// pausePending is set before handleUpgradeTLS arms the past read
		// deadline, so the reader can tell an upgrade-induced timeout from
		// a real one instead of spinning on the expired deadline.
		pausePending atomic.Bool

		pausedAck chan struct{}

		done      chan struct{}
		closeOnce sync.Once
	}
)

// NewFlowController creates a new flow controller with the given initial window credit.
// A negative initial credit is clamped to zero: callers validate wire input
// at the boundary, and the clamp keeps the accumulator robust regardless.
func NewFlowController(initialCredit int32) *FlowController {
	if initialCredit < 0 {
		initialCredit = 0
	}
	fc := &FlowController{
		credit: int64(initialCredit),
	}
	fc.cond = sync.NewCond(&fc.mu)
	return fc
}

// Acquire blocks until at least n bytes of credit are available, or context is cancelled.
func (fc *FlowController) Acquire(ctx context.Context, n int) error {
	if fc == nil || n <= 0 {
		return nil
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Wait loop
	for fc.credit < int64(n) && !fc.closed {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Use a goroutine to wake up cond on context cancellation
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				fc.mu.Lock()
				fc.cond.Broadcast()
				fc.mu.Unlock()
			case <-done:
			}
		}()

		fc.cond.Wait()
		close(done)

		if err := ctx.Err(); err != nil {
			return err
		}
	}

	if fc.closed {
		return io.ErrClosedPipe
	}

	fc.credit -= int64(n)
	return nil
}

// AcquirePartial blocks until at least 1 byte of credit is available, then claims min(available, max) bytes.
func (fc *FlowController) AcquirePartial(ctx context.Context, max int) (int, error) {
	if fc == nil || max <= 0 {
		return max, nil
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()

	for fc.credit <= 0 && !fc.closed {
		if err := ctx.Err(); err != nil {
			return 0, err
		}

		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				fc.mu.Lock()
				fc.cond.Broadcast()
				fc.mu.Unlock()
			case <-done:
			}
		}()

		fc.cond.Wait()
		close(done)

		if err := ctx.Err(); err != nil {
			return 0, err
		}
	}

	if fc.closed {
		return 0, io.ErrClosedPipe
	}

	take := int64(max)
	if fc.credit < take {
		take = fc.credit
	}

	fc.credit -= take
	return int(take), nil
}

// AddCredit increments available window credit and wakes waiters.
// Non-positive credit is a no-op: wire input is validated at the boundary
// (negative credit is a protocol violation), and internal refunds are
// always positive, so this guard is defense in depth for the accumulator.
func (fc *FlowController) AddCredit(n int32) {
	if fc == nil || n <= 0 {
		return
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.credit += int64(n)
	fc.cond.Broadcast()
}

// Close unblocks all waiters on the flow controller.
func (fc *FlowController) Close() {
	if fc == nil {
		return
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.closed = true
	fc.cond.Broadcast()
}

// RunServerDemux executes the active bidirectional message loop for a connection with in-stream control support.
func RunServerDemux(
	ctx context.Context,
	stream rc.RemoteControl_NetConnServer,
	conn net.Conn,
	serverCaps *rc.NetConnResponse_Capabilities,
	clientCaps *rc.NetConnRequest_Capabilities,
	tlsProvider TLSProvider,
	defaultServerName string,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	s := &serverControlState{
		stream:            stream,
		serverCaps:        serverCaps,
		clientCaps:        clientCaps,
		tlsProvider:       tlsProvider,
		defaultServerName: defaultServerName,
		activeConn:        conn,
		pauseReq:          make(chan struct{}, 1),
		pausedAck:         make(chan struct{}, 1),
		resumeNotify:      make(chan net.Conn, 1),
		done:              make(chan struct{}),
	}

	if serverCaps.GetSupportsFlowControl() && clientCaps.GetSupportsFlowControl() {
		clientInitWin := clientCaps.GetInitialWindowSize()
		if clientInitWin == 0 {
			clientInitWin = DefaultInitialWindowSize
		}
		serverInitWin := serverCaps.GetInitialWindowSize()
		if serverInitWin == 0 {
			serverInitWin = DefaultInitialWindowSize
		}
		s.outboundFC = NewFlowController(clientInitWin)
		s.inboundFC = NewFlowController(serverInitWin)
	}

	// Never emit chunks larger than our own advertised cap, nor larger
	// than the client's advertised receive cap; absent advertisements
	// fall back to DefaultChunkSize. The client's value wins when both
	// are set and it is the smaller.
	readChunkSize := DefaultChunkSize
	if advertised := int(serverCaps.GetMaxChunkSize()); advertised > 0 {
		readChunkSize = advertised
	}
	if clientMax := int(clientCaps.GetMaxChunkSize()); clientMax > 0 && clientMax < readChunkSize {
		readChunkSize = clientMax
	}

	defer func() {
		s.closeOnce.Do(func() {
			close(s.done)
			if s.outboundFC != nil {
				s.outboundFC.Close()
			}
			if s.inboundFC != nil {
				s.inboundFC.Close()
			}
			s.connMu.Lock()
			if s.activeConn != nil {
				_ = s.activeConn.Close()
			}
			s.connMu.Unlock()
		})
	}()

	errCh := make(chan error, 2)

	// Goroutine 1: Read from activeConn -> send to stream
	go func() {
		buf := make([]byte, readChunkSize)
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case <-s.done:
				return
			case <-s.pauseReq:
				// Pause requested for in-stream TLS upgrade.
				s.pausedAck <- struct{}{}
				select {
				case <-ctx.Done():
					return
				case <-s.done:
					return
				case newConn := <-s.resumeNotify:
					s.connMu.Lock()
					s.activeConn = newConn
					s.connMu.Unlock()
				}
			default:
			}

			s.connMu.RLock()
			c := s.activeConn
			s.connMu.RUnlock()

			if c == nil {
				return
			}

			n, err := c.Read(buf)
			if n > 0 {
				rem := buf[:n]
				for len(rem) > 0 {
					sendLen := len(rem)
					if s.outboundFC != nil {
						var acqErr error
						sendLen, acqErr = s.outboundFC.AcquirePartial(ctx, sendLen)
						if acqErr != nil {
							errCh <- acqErr
							return
						}
						if sendLen <= 0 {
							// Zero credit would leave rem unchanged and
							// spin on the same slice.
							errCh <- io.ErrNoProgress
							return
						}
					}

					chunk := make([]byte, sendLen)
					copy(chunk, rem[:sendLen])
					rem = rem[sendLen:]

					s.sendMu.Lock()
					sendErr := stream.Send(&rc.NetConnResponse{
						Data: &rc.NetConnResponse_Bytes{Bytes: chunk},
					})
					s.sendMu.Unlock()

					if sendErr != nil {
						errCh <- sendErr
						return
					}
				}
			}

			if err != nil {
				if errors.Is(err, io.EOF) {
					// Connection reached EOF
					if clientCaps.GetSupportsFlowControl() || clientCaps.GetSupportsOpportunisticTls() {
						s.sendMu.Lock()
						_ = stream.Send(&rc.NetConnResponse{
							Data: &rc.NetConnResponse_Control_{
								Control: &rc.NetConnResponse_Control{
									Event: &rc.NetConnResponse_Control_HalfClose_{
										HalfClose: &rc.NetConnResponse_Control_HalfClose{},
									},
								},
							},
						})
						s.sendMu.Unlock()
					}
					errCh <- nil
					return
				}

				// The past deadline set by handleUpgradeTLS is the only
				// expected source of a timeout here. Any other timeout is a
				// genuine read failure and must be reported, not retried.
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() && s.pausePending.Load() {
					continue
				}

				errCh <- err
				return
			}
		}
	}()

	// Goroutine 2: Read from stream -> dispatch to conn / control
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					errCh <- nil
					return
				}
				errCh <- err
				return
			}

			switch data := msg.GetData().(type) {
			case *rc.NetConnRequest_Bytes:
				s.connMu.RLock()
				c := s.activeConn
				s.connMu.RUnlock()

				if c != nil && len(data.Bytes) > 0 {
					rem := data.Bytes
					for len(rem) > 0 {
						writeLen := len(rem)
						if s.inboundFC != nil {
							var acquireErr error
							writeLen, acquireErr = s.inboundFC.AcquirePartial(ctx, writeLen)
							if acquireErr != nil {
								errCh <- acquireErr
								return
							}
							if writeLen <= 0 {
								// Zero credit would leave rem unchanged and
								// spin on the same slice.
								errCh <- io.ErrNoProgress
								return
							}
						}
						nw, writeErr := c.Write(rem[:writeLen])
						if writeErr != nil {
							errCh <- writeErr
							return
						}
						if nw != writeLen {
							// A short write with no error must not be
							// treated as complete: advance by what was
							// actually consumed and refund only that.
							if nw < 0 || nw > writeLen {
								errCh <- io.ErrShortWrite
								return
							}
							if nw == 0 {
								errCh <- io.ErrNoProgress
								return
							}
							writeLen = nw
						}
						if s.inboundFC != nil {
							s.inboundFC.AddCredit(int32(writeLen))
						}
						rem = rem[writeLen:]
					}
					// Return inbound credit to the client after draining.
					if s.inboundFC != nil {
						s.sendMu.Lock()
						_ = stream.Send(&rc.NetConnResponse{
							Data: &rc.NetConnResponse_Control_{
								Control: &rc.NetConnResponse_Control{
									Event: &rc.NetConnResponse_Control_WindowUpdate_{
										WindowUpdate: &rc.NetConnResponse_Control_WindowUpdate{
											CreditBytes: int32(len(data.Bytes)),
										},
									},
								},
							},
						})
						s.sendMu.Unlock()
					}
				}

			case *rc.NetConnRequest_Control_:
				ctl := data.Control
				if ctl == nil {
					continue
				}

				switch action := ctl.GetAction().(type) {
				case *rc.NetConnRequest_Control_UpgradeTls:
					// NOTE (upgrade-vs-window): runs inline so this loop
					// keeps servicing windowUpdate only between control
					// messages. A socket reader blocked on an exhausted
					// outbound window while an upgrade arrives can still
					// mutually wait; fixing that needs an interruptible
					// credit wait on both stacks. A parked-handoff design
					// was attempted and reverted: the pause handshake can
					// only be answered by the reader's loop select, and
					// every inline variant deadlocked the normal-path
					// STARTTLS test. Narrow residual risk, recorded.
					//
					// Fail closed: reject a missing-options upgrade before
					// the pause handshake. Accepting it would emit
					// TlsUpgraded with a nil result (no handshake ran) and
					// leave the peer cleartext while it believes TLS is
					// active.
					if action.UpgradeTls.GetOptions() == nil {
						errCh <- grpcstatus.Error(codes.InvalidArgument, "sesame/rc/netconn: upgrade_tls requires options")
						return
					}
					// An operator-disabled opportunistic-TLS flag is
					// enforced, not advisory: reject before the pause
					// handshake so a validation failure cannot disturb
					// the paused reader.
					if !s.serverCaps.GetSupportsOpportunisticTls() {
						errCh <- grpcstatus.Error(codes.FailedPrecondition, "sesame/rc/netconn: opportunistic TLS is disabled by server policy")
						return
					}
					upgradeErr := s.handleUpgradeTLS(ctx, action.UpgradeTls.GetOptions())
					if upgradeErr != nil {
						errCh <- upgradeErr
						return
					}

				case *rc.NetConnRequest_Control_WindowUpdate_:
					// Negative credit is a protocol violation, not an
					// unknown message: fail closed regardless of whether
					// flow control is active, per the schema's
					// unconditional rule and the TS endpoint.
					if action.WindowUpdate.GetCreditBytes() < 0 {
						errCh <- grpcstatus.Error(codes.InvalidArgument, "sesame/rc/netconn: negative window_update credit_bytes")
						return
					}
					if s.outboundFC != nil {
						s.outboundFC.AddCredit(action.WindowUpdate.GetCreditBytes())
					}

				case *rc.NetConnRequest_Control_HalfClose_:
					// Termination flow (d): the server MUST close only its
					// write side to the target. A transport that cannot
					// half-close cannot fulfill the contract, and silently
					// dropping the request would leave the client believing
					// the target saw a FIN. Fail closed instead.
					s.connMu.RLock()
					c := s.activeConn
					s.connMu.RUnlock()
					hc, ok := c.(interface{ CloseWrite() error })
					if !ok {
						errCh <- grpcstatus.Error(codes.Unavailable, "sesame/rc/netconn: upstream connection does not support half-close")
						return
					}
					if err := hc.CloseWrite(); err != nil && !errors.Is(err, net.ErrClosed) {
						// ErrClosed is the benign already-fully-closed race:
						// the reader is about to relay the real EOF. Anything
						// else is a genuine failure to honor the half-close.
						errCh <- grpcstatus.Errorf(codes.Unavailable, "sesame/rc/netconn: half-close failed: %v", err)
						return
					}

				case *rc.NetConnRequest_Control_Ping_:
					s.sendMu.Lock()
					_ = stream.Send(&rc.NetConnResponse{
						Data: &rc.NetConnResponse_Control_{
							Control: &rc.NetConnResponse_Control{
								Event: &rc.NetConnResponse_Control_Pong_{
									Pong: &rc.NetConnResponse_Control_Pong{
										Id:             action.Ping.GetId(),
										TimestampNanos: action.Ping.GetTimestampNanos(),
									},
								},
							},
						},
					})
					s.sendMu.Unlock()

				case *rc.NetConnRequest_Control_Reset_:
					// Termination flow iv: propagate the client's reason
					// code as the gRPC status code (out-of-range values
					// map to CANCELLED, matching the TS endpoint's
					// codeFromRpcStatus) and its message as the detail.
					errCh <- grpcstatus.Error(
						rpcStatusCode(action.Reset_.GetReason().GetCode()),
						fmt.Sprintf("sesame/rc/netconn: connection reset by client: %s", action.Reset_.GetReason().GetMessage()),
					)
					return
				}
			}
		}
	}()

	// Wait for completion or error
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// rpcStatusCode maps a google.rpc.Status code (canonical gRPC numbering)
// to a gRPC status code. Values outside [1, 16] map to CANCELLED, matching
// the TS endpoint's codeFromRpcStatus: a reset is a client-side
// cancellation, and code 0 (OK) carries no reset semantics.
func rpcStatusCode(code int32) codes.Code {
	if code >= 1 && code <= 16 {
		return codes.Code(code)
	}
	return codes.Canceled
}

func (s *serverControlState) handleUpgradeTLS(ctx context.Context, opts *sesametls.TLSOptions) error {
	// 1. Pause outbound reader
	s.connMu.RLock()
	c := s.activeConn
	s.connMu.RUnlock()

	// Flag the pause before arming the deadline so the reader recognises
	// the resulting timeout as upgrade-induced. Without the flag the reader
	// cannot distinguish it from a real read failure.
	s.pausePending.Store(true)
	defer s.pausePending.Store(false)

	// Interrupt any pending read with a past deadline before requesting the
	// pause, so the reader cannot consume the pause signal without entering
	// the paused branch.
	_ = c.SetReadDeadline(time.Now())

	select {
	case s.pauseReq <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-s.pausedAck:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Reset read deadline on raw conn
	_ = c.SetReadDeadline(time.Time{})

	// 2. Perform TLS Handshake
	upgradedConn, result, err := ExecuteTLSHandshake(ctx, c, opts, s.defaultServerName, s.tlsProvider)
	if err != nil {
		// On upgrade failure, send TlsUpgradeFailed and terminate (never fallback to cleartext)
		st, _ := grpcstatus.FromError(err)
		s.sendMu.Lock()
		_ = s.stream.Send(&rc.NetConnResponse{
			Data: &rc.NetConnResponse_Control_{
				Control: &rc.NetConnResponse_Control{
					Event: &rc.NetConnResponse_Control_TlsUpgradeFailed{
						TlsUpgradeFailed: &rc.NetConnResponse_Control_TLSUpgradeFailed{
							Error: &status.Status{
								Code:    int32(st.Code()),
								Message: st.Message(),
							},
						},
					},
				},
			},
		})
		s.sendMu.Unlock()
		return err
	}

	// 3. Send TlsUpgraded response
	s.sendMu.Lock()
	sendErr := s.stream.Send(&rc.NetConnResponse{
		Data: &rc.NetConnResponse_Control_{
			Control: &rc.NetConnResponse_Control{
				Event: &rc.NetConnResponse_Control_TlsUpgraded{
					TlsUpgraded: &rc.NetConnResponse_Control_TLSUpgraded{
						Result: result,
					},
				},
			},
		},
	})
	s.sendMu.Unlock()

	if sendErr != nil {
		_ = upgradedConn.Close()
		return sendErr
	}

	// 4. Resume reader with upgraded connection
	s.resumeNotify <- upgradedConn
	return nil
}
