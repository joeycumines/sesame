package grpc

import (
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"sync"
)

type (
	// tunnelStreamClientWrapper wraps a client or server stream acting as the client (in the proxied gRPC scenario),
	// managing send concurrency.
	tunnelStreamClientWrapper struct {
		stream    tunnelStreamClient
		tearDown  func() error
		mu        sync.Mutex
		streamID  uint64
		streamMap map[uint64]*tscwStreamDone // channels closed when send is closed
		in        chan *ClientToServer
		out       chan error
		once      sync.Once
		stop      chan struct{}
		done      chan struct{}
		err       error
	}

	tscwStreamDone struct {
		half chan struct{}
		full chan struct{}
	}

	// tunnelStreamServerWrapper wraps a client or server stream acting as the server (in the proxied gRPC scenario),
	// managing send concurrency.
	tunnelStreamServerWrapper struct {
		stream tunnelStreamServer
		mu     sync.Mutex
	}
)

var (
	// ErrSendCanceled occurs when a message send was dropped due to another message taking precedence, e.g. a half
	// close message (not yet started to send) replaced with a cancel message, or a cancel message attempted more than
	// one. Note that this error DOES NOT imply that the message, that the send was canceled for, has been successfully
	// sent, or has even started sending yet, just that it was scheduled.
	ErrSendCanceled = errors.New(`sesame/tun/grpc: send canceled`)

	// compile time assertions

	_ tunnelStreamClient = (*tunnelStreamClientWrapper)(nil)
	_ io.Closer          = (*tunnelStreamClientWrapper)(nil)
	_ tunnelStreamServer = (*tunnelStreamServerWrapper)(nil)
)

func wrapTunnelStreamClient(stream tunnelStreamClient, tearDown func() error) (tunnelStreamClient, func() error) {
	w := newTunnelStreamClientWrapper(stream, tearDown)
	return w, w.Close
}

func newTunnelStreamClientWrapper(stream tunnelStreamClient, tearDown func() error) *tunnelStreamClientWrapper {
	r := tunnelStreamClientWrapper{
		stream:    stream,
		tearDown:  tearDown,
		streamMap: make(map[uint64]*tscwStreamDone),
		in:        make(chan *ClientToServer),
		out:       make(chan error),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	go r.worker()
	return &r
}

func (x *tunnelStreamClientWrapper) Context() context.Context { return x.stream.Context() }

func (x *tunnelStreamClientWrapper) Recv() (*ServerToClient, error) {
	// TODO validate that we don't need to track this (shouldn't if cancel is always sent)
	return x.stream.Recv()
}

func (x *tunnelStreamClientWrapper) Send(msg *ClientToServer) error {
	// handle message
	done, err := func() (<-chan struct{}, error) {
		x.mu.Lock()
		defer x.mu.Unlock()

		var (
			streamID    = msg.GetStreamId()
			isHalfClose bool
			isFullClose bool
		)
		switch frame := msg.GetFrame().(type) {
		case *ClientToServer_NewStream_:
			// note that we ensure the stream id increments by one so we can reliably detect cases where the stream was
			// never created (for non-new requests, if the streamID is <= x.streamID, then it's creation was attempted)
			if streamID <= x.streamID || streamID != x.streamID+1 || x.streamMap[streamID] != nil {
				return nil, fmt.Errorf(`sesame/tun/grpc: expected new stream id > %d got %d`, x.streamID, streamID)
			}

			x.streamID = streamID
			x.streamMap[streamID] = &tscwStreamDone{
				half: make(chan struct{}),
				full: make(chan struct{}),
			}

			// we disallow cancellation of new stream requests, to avoid confusing the server
			return nil, nil

		case *ClientToServer_Message:

		case *ClientToServer_MessageData:

		case *ClientToServer_HalfClose:
			isHalfClose = true

		case *ClientToServer_Cancel:
			isHalfClose = true
			isFullClose = true

		default:
			return nil, fmt.Errorf(`sesame/tun/grpc: unexpected or invalid client to server frame %T`, frame)
		}

		// note that 0 is the initial x.streamID value, and is invalid
		if streamID > x.streamID || streamID == 0 {
			return nil, fmt.Errorf(`sesame/tun/grpc: expected stream id <= %d got %d`, x.streamID, streamID)
		}

		var done tscwStreamDone
		if v := x.streamMap[streamID]; v != nil {
			done = *v

			if isHalfClose && v.half != nil {
				close(v.half)
				v.half = nil
			}

			if isFullClose && v.full != nil {
				close(v.full)
				v.full = nil
			}

			if *v == (tscwStreamDone{}) {
				x.streamMap[streamID] = nil
			}
		}

		if done.full == nil {
			// already sent or are already sending cancel
			return nil, ErrSendCanceled
		}

		if isFullClose {
			// we are the first to send cancel, we have nothing to wait on except ourselves
			return nil, nil
		}

		if done.half == nil {
			// already sent or are already sending half close
			return nil, ErrSendCanceled
		}

		if isHalfClose {
			// we are the first to send half close, wait on cancel
			return done.full, nil
		}

		// we are sending one of the other messages, wait on either half close or cancel
		return done.half, nil
	}()
	if err != nil {
		return err
	}

	// note we don't need to pre-emptively check x.stop, it's checked in the worker
	select {
	case <-done:
		return ErrSendCanceled
	default:
	}

	// wait for a send attempt / result
	select {
	case <-done:
		return ErrSendCanceled

	case <-x.stop:
		return ErrSendCanceled

	case x.in <- msg:
		select {
		case <-x.done:
			// panic (goexit) case
			return x.err

		case err := <-x.out:
			return err
		}
	}
}

func (x *tunnelStreamClientWrapper) Close() error {
	x.once.Do(func() { close(x.stop) })
	<-x.done
	return x.err
}

func (x *tunnelStreamClientWrapper) worker() {
	x.err = errors.New(`sesame/tun/grpc: panic in tunnel stream client`)
	defer close(x.done)

ControlLoop:
	for {
		select {
		case <-x.stop:
			break ControlLoop

		case msg := <-x.in:
			select {
			case <-x.stop:
				x.out <- ErrSendCanceled
				break ControlLoop

			default:
				x.out <- x.stream.Send(msg)
			}
		}
	}

	if x.tearDown != nil {
		x.err = x.tearDown()
	} else {
		x.err = nil
	}
}

func newTunnelStreamServerWrapper(stream tunnelStreamServer) *tunnelStreamServerWrapper {
	return &tunnelStreamServerWrapper{stream: stream}
}

func (x *tunnelStreamServerWrapper) Context() context.Context { return x.stream.Context() }

func (x *tunnelStreamServerWrapper) Send(msg *ServerToClient) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.stream.Send(msg)
}

func (x *tunnelStreamServerWrapper) Recv() (*ClientToServer, error) { return x.stream.Recv() }

func errTransportClosing() error { return status.Error(codes.Unavailable, `transport is closing`) }
