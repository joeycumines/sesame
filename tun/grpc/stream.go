package grpc

import (
	"context"
	"errors"
	"fmt"
	"github.com/joeycumines/sesame/internal/flowcontrol"
	typepb "github.com/joeycumines/sesame/typepb"
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
		stream   tunnelStreamClient
		tearDown func() error
		in       chan *typepb.OpenTunnelRequest
		out      chan error
		once     sync.Once
		stop     chan struct{}
		done     chan struct{}
		err      error
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
	// compile time assertions

	_ tunnelStreamClient = (*tunnelStreamClientWrapper)(nil)
	_ io.Closer          = (*tunnelStreamClientWrapper)(nil)
	_ tunnelStreamServer = (*tunnelStreamServerWrapper)(nil)

	_ tunnelStreamClient = reverseTunnelClientAdapter{}
	_ tunnelStreamServer = reverseTunnelServerAdapter{}

	_ = map[bool]struct{}{false: {}, windowMaxSize > 0: {}}
	_ = map[bool]struct{}{false: {}, windowMaxBuffer > 1: {}}
	_ = map[bool]struct{}{false: {}, windowMinConsume > 0 && windowMinConsume*(windowMaxBuffer-1) > windowMaxSize: {}}
	_ = map[bool]struct{}{false: {}, windowUpdateSize >= 1 && windowUpdateSize <= windowMaxSize: {}}
)

// reverseTunnelClientAdapter adapts the server side of an OpenReverseTunnel
// stream - on which the server initiates the tunneled RPCs - to the
// client-oriented tunnelStreamClient interface. The reverse tunnel's frames
// are wire-identical to the forward tunnel's; the adapter shallow-copies
// between the mirrored message vocabularies (OpenReverseTunnelResponse carries
// the client-shaped frames, OpenReverseTunnelRequest the server-shaped ones).
type reverseTunnelClientAdapter struct {
	stream typepb.TunnelService_OpenReverseTunnelServer
}

func (x reverseTunnelClientAdapter) Context() context.Context { return x.stream.Context() }

func (x reverseTunnelClientAdapter) Send(msg *typepb.OpenTunnelRequest) error {
	return x.stream.Send(typepb.ReverseResponseFromTunnelRequest(msg))
}

func (x reverseTunnelClientAdapter) Recv() (*typepb.OpenTunnelResponse, error) {
	msg, err := x.stream.Recv()
	if err != nil {
		return nil, err
	}
	return typepb.TunnelResponseFromReverseRequest(msg), nil
}

// reverseTunnelServerAdapter adapts the client side of an OpenReverseTunnel
// stream - on which the endpoint serves the RPCs the server initiates - to
// the server-oriented tunnelStreamServer interface, mirroring
// reverseTunnelClientAdapter.
type reverseTunnelServerAdapter struct {
	stream typepb.TunnelService_OpenReverseTunnelClient
}

func (x reverseTunnelServerAdapter) Context() context.Context { return x.stream.Context() }

func (x reverseTunnelServerAdapter) Send(msg *typepb.OpenTunnelResponse) error {
	return x.stream.Send(typepb.ReverseRequestFromTunnelResponse(msg))
}

func (x reverseTunnelServerAdapter) Recv() (*typepb.OpenTunnelRequest, error) {
	msg, err := x.stream.Recv()
	if err != nil {
		return nil, err
	}
	return typepb.TunnelRequestFromReverseResponse(msg), nil
}

func newTunnelStreamClientWrapper(stream tunnelStreamClient, tearDown func() error) *tunnelStreamClientWrapper {
	r := tunnelStreamClientWrapper{
		stream:   stream,
		tearDown: tearDown,
		in:       make(chan *typepb.OpenTunnelRequest),
		out:      make(chan error),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go r.worker()
	return &r
}

func (x *tunnelStreamClientWrapper) Context() context.Context { return x.stream.Context() }

func (x *tunnelStreamClientWrapper) Recv() (*typepb.OpenTunnelResponse, error) {
	return x.stream.Recv()
}

func (x *tunnelStreamClientWrapper) Send(msg *typepb.OpenTunnelRequest) error {
	// wait for a send attempt / result
	// note we don't need to pre-emptively check x.stop, it's checked in the worker
	select {
	case <-x.stop:
		return context.Canceled

	case <-x.done:
		return context.Canceled

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
				x.out <- context.Canceled
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

func (x *tunnelStreamServerWrapper) Send(msg *typepb.OpenTunnelResponse) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.stream.Send(msg)
}

func (x *tunnelStreamServerWrapper) Recv() (*typepb.OpenTunnelRequest, error) {
	return x.stream.Recv()
}

func (x *flowControlClient) SendWindowUpdate(streamID uint64, windowUpdate uint32) *typepb.OpenTunnelRequest {
	return &typepb.OpenTunnelRequest{StreamId: int64(streamID), Frame: &typepb.OpenTunnelRequest_WindowUpdate{WindowUpdate: int32(windowUpdate)}}
}
func (x *flowControlClient) RecvWindowUpdate(msg *typepb.OpenTunnelResponse) (uint32, bool) {
	if v, ok := msg.GetFrame().(*typepb.OpenTunnelResponse_WindowUpdate); ok {
		return uint32(v.WindowUpdate), true
	}
	return 0, false
}
func (x *flowControlClient) SendSize(msg *typepb.OpenTunnelRequest) (uint32, bool) {
	return clientToServerMessageSize(msg)
}
func (x *flowControlClient) RecvSize(msg *typepb.OpenTunnelResponse) (uint32, bool) {
	return serverToClientMessageSize(msg)
}
func (x *flowControlClient) SendKey(msg *typepb.OpenTunnelRequest) uint64 {
	return uint64(msg.GetStreamId())
}
func (x *flowControlClient) RecvKey(msg *typepb.OpenTunnelResponse) uint64 {
	return uint64(msg.GetStreamId())
}
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

func (x *flowControlServer) SendWindowUpdate(streamID uint64, windowUpdate uint32) *typepb.OpenTunnelResponse {
	return &typepb.OpenTunnelResponse{StreamId: int64(streamID), Frame: &typepb.OpenTunnelResponse_WindowUpdate{WindowUpdate: int32(windowUpdate)}}
}
func (x *flowControlServer) RecvWindowUpdate(msg *typepb.OpenTunnelRequest) (uint32, bool) {
	if v, ok := msg.GetFrame().(*typepb.OpenTunnelRequest_WindowUpdate); ok {
		return uint32(v.WindowUpdate), true
	}
	return 0, false
}
func (x *flowControlServer) SendSize(msg *typepb.OpenTunnelResponse) (uint32, bool) {
	return serverToClientMessageSize(msg)
}
func (x *flowControlServer) RecvSize(msg *typepb.OpenTunnelRequest) (uint32, bool) {
	return clientToServerMessageSize(msg)
}
func (x *flowControlServer) SendKey(msg *typepb.OpenTunnelResponse) uint64 {
	return uint64(msg.GetStreamId())
}
func (x *flowControlServer) RecvKey(msg *typepb.OpenTunnelRequest) uint64 {
	return uint64(msg.GetStreamId())
}
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

func clientToServerMessageSize(msg *typepb.OpenTunnelRequest) (uint32, bool) {
	var size int64
	switch frame := msg.GetFrame().(type) {
	case *typepb.OpenTunnelRequest_Message:
		size = int64(len(frame.Message.GetData()))
	case *typepb.OpenTunnelRequest_MessageData:
		size = int64(len(frame.MessageData))
	default:
		return 0, false
	}
	if size > math.MaxUint32 {
		panic(fmt.Errorf(`sesame/tun/grpc: unexpected client to server message size: %d`, size))
	}
	return uint32(size), true
}
func serverToClientMessageSize(msg *typepb.OpenTunnelResponse) (uint32, bool) {
	var size int64
	switch frame := msg.GetFrame().(type) {
	case *typepb.OpenTunnelResponse_Message:
		size = int64(len(frame.Message.GetData()))
	case *typepb.OpenTunnelResponse_MessageData:
		size = int64(len(frame.MessageData))
	default:
		return 0, false
	}
	if size > math.MaxUint32 {
		panic(fmt.Errorf(`sesame/tun/grpc: unexpected server to client message size: %d`, size))
	}
	return uint32(size), true
}
