package grpc

import (
	"errors"
	"fmt"
	"github.com/joeycumines/sesame/internal/flowcontrol"
	"golang.org/x/net/context"
	"io"
	"math"
	"sync"
)

const (
	// windowMaxSize is the initial size of the per-stream per-sender flow control window.
	// Unlike HTTP/2, this implementation does not support updating it.
	// Also unlike HTTP/2, the buffering is per-message, and so is allowed to be exceeded.
	// See also windowMaxBuffer and windowMinConsume.
	windowMaxSize = 65535
	// windowUpdateSize is the threshold for window updates, which is the point at which a window update message will
	// be sent to the remote sender.
	windowUpdateSize = windowMaxSize * 0.4
	// windowMaxBuffer is the target number of messages that we want to allow to be buffered (at most) per stream.
	windowMaxBuffer = 256
	// windowMinConsume is a value that aims to support windowMaxBuffer by means of adjusting consumption from
	// the window.
	windowMinConsume = windowMaxSize/(windowMaxBuffer-1) + 1
)

type (
	// tunnelStreamClientWrapper wraps a client or server stream acting as the client (in the proxied gRPC scenario),
	// managing send concurrency.
	tunnelStreamClientWrapper struct {
		stream    tunnelStreamClient
		tearDown  func() error
		mu        sync.Mutex
		streamMap map[uint64]*tscwStream
		in        chan *ClientToServer
		out       chan error
		once      sync.Once
		stop      chan struct{}
		done      chan struct{}
		err       error
	}

	tscwStream struct {
		half chan struct{}
		full chan struct{}
	}

	// tunnelStreamServerWrapper wraps a client or server stream acting as the server (in the proxied gRPC scenario),
	// managing send concurrency.
	tunnelStreamServerWrapper struct {
		stream tunnelStreamServer
		mu     sync.Mutex
	}

	flowControlClient struct {
		flowControlConfig
		tunnelStreamClient
		ch *Channel
	}

	flowControlServer struct {
		flowControlConfig
		tunnelStreamServer
		tun *tunnelServer
	}

	flowControlConfig struct{}
)

var (
	// errSendCanceled occurs when a message send was dropped due to another message taking precedence, e.g. a half
	// close message (not yet started to send) replaced with a cancel message, or a cancel message attempted more than
	// one. Note that this error DOES NOT imply that the message, that the send was canceled for, has been successfully
	// sent, or has even started sending yet, just that it was scheduled.
	errSendCanceled = errors.New(`sesame/tun/grpc: send canceled`)

	// compile time assertions

	_ tunnelStreamClient = (*tunnelStreamClientWrapper)(nil)
	_ io.Closer          = (*tunnelStreamClientWrapper)(nil)
	_ tunnelStreamServer = (*tunnelStreamServerWrapper)(nil)

	_ = map[bool]struct{}{false: {}, windowMaxSize > 0: {}}
	_ = map[bool]struct{}{false: {}, windowMaxBuffer > 1: {}}
	_ = map[bool]struct{}{false: {}, windowMinConsume > 0 && windowMinConsume*(windowMaxBuffer-1) > windowMaxSize: {}}
	_ = map[bool]struct{}{false: {}, windowUpdateSize >= 1 && windowUpdateSize <= windowMaxSize: {}}
)

func newTunnelStreamClientWrapper(stream tunnelStreamClient, tearDown func() error) *tunnelStreamClientWrapper {
	r := tunnelStreamClientWrapper{
		stream:    stream,
		tearDown:  tearDown,
		streamMap: make(map[uint64]*tscwStream),
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
		streamID := msg.GetStreamId()
		if streamID == 0 {
			// shouldn't happen
			panic(`sesame/tun/grpc: stream id 0 is invalid`)
		}

		x.mu.Lock()
		defer x.mu.Unlock()

		var (
			isHalfClose bool
			isFullClose bool
		)
		switch frame := msg.GetFrame().(type) {
		case *ClientToServer_WindowUpdate:
			// always send window updates
			return nil, nil

		case *ClientToServer_NewStream_:
			if _, ok := x.streamMap[streamID]; ok {
				// shouldn't happen
				panic(fmt.Errorf(`sesame/tun/grpc: duplicate new stream message or recreated stream id %d`, streamID))
			}
			x.streamMap[streamID] = &tscwStream{
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

		var done tscwStream
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

			if *v == (tscwStream{}) {
				x.streamMap[streamID] = nil
			}
		}

		if done.full == nil {
			// already sent or are already sending cancel
			return nil, errSendCanceled
		}

		if isFullClose {
			// we are the first to send cancel, we have nothing to wait on except ourselves
			return nil, nil
		}

		if done.half == nil {
			// already sent or are already sending half close
			return nil, errSendCanceled
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
		return errSendCanceled
	default:
	}

	// wait for a send attempt / result
	select {
	case <-done:
		return errSendCanceled

	case <-x.stop:
		return errSendCanceled

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
				x.out <- errSendCanceled
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
	if msg.GetStreamId() == 0 {
		return errors.New(`sesame/tun/grpc: stream id 0 invalid`)
	}
	switch frame := msg.GetFrame().(type) {
	case *ServerToClient_WindowUpdate:
	case *ServerToClient_Header:
	case *ServerToClient_Message:
	case *ServerToClient_MessageData:
	case *ServerToClient_CloseStream_:
	default:
		return fmt.Errorf(`sesame/tun/grpc: unexpected or invalid server to client frame %T`, frame)
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.stream.Send(msg)
}

func (x *tunnelStreamServerWrapper) Recv() (*ClientToServer, error) { return x.stream.Recv() }

func (x *flowControlClient) SendWindowUpdate(streamID uint64, windowUpdate uint32) *ClientToServer {
	return &ClientToServer{StreamId: streamID, Frame: &ClientToServer_WindowUpdate{WindowUpdate: windowUpdate}}
}
func (x *flowControlClient) RecvWindowUpdate(msg *ServerToClient) (uint32, bool) {
	if v, ok := msg.GetFrame().(*ServerToClient_WindowUpdate); ok {
		return v.WindowUpdate, true
	}
	return 0, false
}
func (x *flowControlClient) SendSize(msg *ClientToServer) (uint32, bool) {
	return clientToServerMessageSize(msg)
}
func (x *flowControlClient) RecvSize(msg *ServerToClient) (uint32, bool) {
	return serverToClientMessageSize(msg)
}
func (x *flowControlClient) SendKey(msg *ClientToServer) uint64 { return msg.GetStreamId() }
func (x *flowControlClient) RecvKey(msg *ServerToClient) uint64 { return msg.GetStreamId() }
func (x *flowControlClient) SendLoad(streamID uint64) int64 {
	x.ch.mu.RLock()
	defer x.ch.mu.RUnlock()
	if v := x.ch.streams[streamID]; v != nil {
		return v.fcSend
	}
	return 0
}
func (x *flowControlClient) RecvLoad(streamID uint64) int64 {
	x.ch.mu.RLock()
	defer x.ch.mu.RUnlock()
	if v := x.ch.streams[streamID]; v != nil {
		return v.fcRecv
	}
	return 0
}
func (x *flowControlClient) SendStore(streamID uint64, value int64) {
	x.ch.mu.RLock()
	defer x.ch.mu.RUnlock()
	if v := x.ch.streams[streamID]; v != nil {
		v.fcSend = value
	}
}
func (x *flowControlClient) RecvStore(streamID uint64, value int64) {
	x.ch.mu.RLock()
	defer x.ch.mu.RUnlock()
	if v := x.ch.streams[streamID]; v != nil {
		v.fcRecv = value
	}
}
func (x *flowControlClient) StreamContext(streamID uint64) context.Context {
	x.ch.mu.RLock()
	defer x.ch.mu.RUnlock()
	if v := x.ch.streams[streamID]; v != nil {
		return v.ctx
	}
	return flowcontrol.StreamNotFound
}

func (x *flowControlServer) SendWindowUpdate(streamID uint64, windowUpdate uint32) *ServerToClient {
	return &ServerToClient{StreamId: streamID, Frame: &ServerToClient_WindowUpdate{WindowUpdate: windowUpdate}}
}
func (x *flowControlServer) RecvWindowUpdate(msg *ClientToServer) (uint32, bool) {
	if v, ok := msg.GetFrame().(*ClientToServer_WindowUpdate); ok {
		return v.WindowUpdate, true
	}
	return 0, false
}
func (x *flowControlServer) SendSize(msg *ServerToClient) (uint32, bool) {
	return serverToClientMessageSize(msg)
}
func (x *flowControlServer) RecvSize(msg *ClientToServer) (uint32, bool) {
	return clientToServerMessageSize(msg)
}
func (x *flowControlServer) SendKey(msg *ServerToClient) uint64 { return msg.GetStreamId() }
func (x *flowControlServer) RecvKey(msg *ClientToServer) uint64 { return msg.GetStreamId() }
func (x *flowControlServer) SendLoad(streamID uint64) int64 {
	x.tun.mu.RLock()
	defer x.tun.mu.RUnlock()
	if v := x.tun.streams[streamID]; v != nil {
		return v.fcSend
	}
	return 0
}
func (x *flowControlServer) RecvLoad(streamID uint64) int64 {
	x.tun.mu.RLock()
	defer x.tun.mu.RUnlock()
	if v := x.tun.streams[streamID]; v != nil {
		return v.fcRecv
	}
	return 0
}
func (x *flowControlServer) SendStore(streamID uint64, value int64) {
	x.tun.mu.RLock()
	defer x.tun.mu.RUnlock()
	if v := x.tun.streams[streamID]; v != nil {
		v.fcSend = value
	}
}
func (x *flowControlServer) RecvStore(streamID uint64, value int64) {
	x.tun.mu.RLock()
	defer x.tun.mu.RUnlock()
	if v := x.tun.streams[streamID]; v != nil {
		v.fcRecv = value
	}
}
func (x *flowControlServer) StreamContext(streamID uint64) context.Context {
	x.tun.mu.RLock()
	defer x.tun.mu.RUnlock()
	if v := x.tun.streams[streamID]; v != nil {
		return v.ctx
	}
	return flowcontrol.StreamNotFound
}

func (flowControlConfig) SendConfig(streamID uint64) flowcontrol.Config {
	return flowcontrol.Config{
		MaxWindow:  windowMaxSize,
		MinConsume: windowMinConsume,
	}
}
func (flowControlConfig) RecvConfig(streamID uint64) flowcontrol.Config {
	return flowcontrol.Config{
		MaxWindow:  uint32(windowUpdateSize),
		MinConsume: windowMinConsume,
	}
}

func clientToServerMessageSize(msg *ClientToServer) (uint32, bool) {
	var size int
	switch frame := msg.GetFrame().(type) {
	case *ClientToServer_Message:
		size = len(frame.Message.GetData())
	case *ClientToServer_MessageData:
		size = len(frame.MessageData)
	default:
		return 0, false
	}
	if size > math.MaxUint32 {
		panic(fmt.Errorf(`sesame/tun/grpc: unexpected client to server message size: %d`, size))
	}
	return uint32(size), true
}
func serverToClientMessageSize(msg *ServerToClient) (uint32, bool) {
	var size int
	switch frame := msg.GetFrame().(type) {
	case *ServerToClient_Message:
		size = len(frame.Message.GetData())
	case *ServerToClient_MessageData:
		size = len(frame.MessageData)
	default:
		return 0, false
	}
	if size > math.MaxUint32 {
		panic(fmt.Errorf(`sesame/tun/grpc: unexpected server to client message size: %d`, size))
	}
	return uint32(size), true
}
