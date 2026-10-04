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
	sesametls "github.com/joeycumines/sesame/type/tls"
	"google.golang.org/genproto/googleapis/rpc/status"
	grpcstatus "google.golang.org/grpc/status"
)

const (
	// DefaultInitialWindowSize is the flow control window defined in RFC-0001 (65,535 bytes).
	DefaultInitialWindowSize uint32 = 65535

	// DefaultChunkSize is the maximum chunk size for streaming payload bytes.
	DefaultChunkSize = 32 * 1024
)

type (
	// FlowController implements credit-based stream flow control per RFC-0001 Section 4.5.
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
func NewFlowController(initialCredit uint32) *FlowController {
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
func (fc *FlowController) AddCredit(n uint32) {
	if fc == nil || n == 0 {
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
		buf := make([]byte, DefaultChunkSize)
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
						if _, writeErr := c.Write(rem[:writeLen]); writeErr != nil {
							errCh <- writeErr
							return
						}
						if s.inboundFC != nil {
							s.inboundFC.AddCredit(uint32(writeLen))
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
											CreditBytes: uint32(len(data.Bytes)),
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
					upgradeErr := s.handleUpgradeTLS(ctx, action.UpgradeTls.GetOptions())
					if upgradeErr != nil {
						errCh <- upgradeErr
						return
					}

				case *rc.NetConnRequest_Control_WindowUpdate_:
					if s.outboundFC != nil {
						s.outboundFC.AddCredit(action.WindowUpdate.GetCreditBytes())
					}

				case *rc.NetConnRequest_Control_HalfClose_:
					s.connMu.RLock()
					c := s.activeConn
					s.connMu.RUnlock()
					if hc, ok := c.(interface{ CloseWrite() error }); ok {
						_ = hc.CloseWrite()
					}

				case *rc.NetConnRequest_Control_Ping_:
					s.sendMu.Lock()
					_ = stream.Send(&rc.NetConnResponse{
						Data: &rc.NetConnResponse_Control_{
							Control: &rc.NetConnResponse_Control{
								Event: &rc.NetConnResponse_Control_Pong_{
									Pong: &rc.NetConnResponse_Control_Pong{
										Id:          action.Ping.GetId(),
										TimestampNs: action.Ping.GetTimestampNs(),
									},
								},
							},
						},
					})
					s.sendMu.Unlock()

				case *rc.NetConnRequest_Control_Reset_:
					errCh <- fmt.Errorf("sesame/rc/netconn: connection reset by client: %s", action.Reset_.GetReason().GetMessage())
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
		// RFC Section 4.4: On upgrade failure, send TlsUpgradeFailed and terminate (never fallback to cleartext)
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
