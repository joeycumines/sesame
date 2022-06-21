// Copyright 2018 Joshua Humphries
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grpc

import (
	"errors"
	"fmt"
	"github.com/joeycumines/sesame/internal/flowcontrol"
	"github.com/joeycumines/sesame/type/grpctunnel"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"math"
	"sync"
)

func NewChannel(options ...ChannelOption) (*Channel, error) {
	var c channelConfig
	for _, o := range options {
		o(&c)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return newChannel(&c), nil
}

type tunnelStreamClient interface {
	Context() context.Context
	Send(*grpctunnel.ClientToServer) error
	Recv() (*grpctunnel.ServerToClient, error)
}

// Channel is a tunnel client, and implements grpc.ClientConnInterface.
// It is backed by a single stream, though this stream may be either a tunnel client, or a reverse tunnel server.
type Channel struct {
	stream       tunnelStreamClient
	updateWindow func(msg *grpctunnel.ServerToClient)
	ctx          context.Context
	cancel       context.CancelFunc
	tearDown     func() error

	// WARNING this mutex MUST NOT be held while blocking on sending to a stream
	mu            sync.RWMutex
	streams       map[uint64]*tunnelClientStream
	lastStreamID  uint64
	streamCreated bool
	err           error
	finished      bool
}

var _ grpc.ClientConnInterface = (*Channel)(nil)

func newChannel(c *channelConfig) *Channel {
	var (
		stream   = c.stream.get().stream
		tearDown = c.stream.get().tearDown
	)

	// wrap the stream (and tearDown) to add concurrency controls
	{
		wrapper := newTunnelStreamClientWrapper(stream, tearDown)
		stream, tearDown = wrapper, wrapper.Close
	}

	// initialise flow control, wrapping stream
	// WARNING not ready for use yet (needs to be wired up with the ChanneL)
	fcInterface := &flowControlClient{tunnelStreamClient: stream}
	fcConn := flowcontrol.NewConn[uint64, *grpctunnel.ClientToServer, *grpctunnel.ServerToClient](fcInterface)
	stream = fcConn

	ch := Channel{
		stream:       stream,
		updateWindow: fcConn.UpdateWindow,
		tearDown:     tearDown,
		streams:      make(map[uint64]*tunnelClientStream),
	}

	// finish wiring up fcInterface with ch
	fcInterface.ch = &ch

	// finish initialisation of ch, starting the recv worker
	ch.ctx, ch.cancel = context.WithCancel(stream.Context())
	go ch.recvLoop()
	return &ch
}

func (c *Channel) Context() context.Context {
	return c.ctx
}

func (c *Channel) Done() <-chan struct{} {
	return c.ctx.Done()
}

func (c *Channel) Canceled() bool {
	if c.ctx.Err() != nil {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.finished
}

func (c *Channel) Err() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	switch c.err {
	case nil:
		return c.ctx.Err()
	case io.EOF:
		return nil
	default:
		return c.err
	}
}

func (c *Channel) Close() error { return c.close(nil) }

func (c *Channel) Invoke(ctx context.Context, methodName string, req, resp interface{}, opts ...grpc.CallOption) error {
	str, err := c.newStream(ctx, false, false, methodName, opts...)
	if err != nil {
		return err
	}
	if err := str.SendMsg(req); err != nil {
		return err
	}
	if err := str.CloseSend(); err != nil {
		return err
	}
	return str.RecvMsg(resp)
}

func (c *Channel) NewStream(ctx context.Context, desc *grpc.StreamDesc, methodName string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return c.newStream(ctx, desc.ClientStreams, desc.ServerStreams, methodName, opts...)
}

func (c *Channel) newStream(ctx context.Context, clientStreams, serverStreams bool, methodName string, opts ...grpc.CallOption) (*tunnelClientStream, error) {
	str, md, err := c.allocateStream(ctx, clientStreams, serverStreams, methodName, opts)
	if err != nil {
		return nil, err
	}
	err = c.stream.Send(&grpctunnel.ClientToServer{
		StreamId: str.streamID,
		Frame: &grpctunnel.ClientToServer_NewStream_{
			NewStream: &grpctunnel.ClientToServer_NewStream{
				Method: methodName,
				Header: toProto(md),
			},
		},
	})
	if err != nil {
		c.removeStream(str.streamID)
		return nil, err
	}
	go func() {
		// if context gets cancelled, make sure
		// we shutdown the stream
		<-str.ctx.Done()
		_ = str.cancel(str.ctx.Err())
	}()
	return str, nil
}

func (c *Channel) allocateStream(ctx context.Context, clientStreams, serverStreams bool, methodName string, opts []grpc.CallOption) (*tunnelClientStream, metadata.MD, error) {
	md, _ := metadata.FromOutgoingContext(ctx)
	var hdrs, tlrs []*metadata.MD
	pr, _ := peer.FromContext(c.ctx)
	authority := "<unknown>"
	isSecure := false
	if pr != nil {
		authority = pr.Addr.String()
		isSecure = pr.AuthInfo != nil
	}

	for _, opt := range opts {
		switch opt := opt.(type) {
		case grpc.HeaderCallOption:
			if opt.HeaderAddr != nil {
				hdrs = append(hdrs, opt.HeaderAddr)
			}

		case grpc.TrailerCallOption:
			if opt.TrailerAddr != nil {
				tlrs = append(tlrs, opt.TrailerAddr)
			}

		case grpc.PeerCallOption:
			if pr != nil {
				*opt.PeerAddr = *pr
			}

		case grpc.PerRPCCredsCallOption:
			if opt.Creds.RequireTransportSecurity() && !isSecure {
				return nil, nil, fmt.Errorf("per-RPC credentials %T cannot be used with insecure channel", opt.Creds)
			}

			mdVals, err := opt.Creds.GetRequestMetadata(ctx, fmt.Sprintf("tunnel://%s%s", authority, methodName))
			if err != nil {
				return nil, nil, err
			}
			for k, v := range mdVals {
				md.Append(k, v)
			}

			//case grpc.ContentSubtypeCallOption:
			//case grpc.CustomCodecCallOption:
			//case grpc.CompressorCallOption:
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.finished {
		return nil, nil, errors.New("channel is closed")
	}

	if c.lastStreamID == math.MaxUint64 {
		return nil, nil, errors.New("all stream IDs exhausted (must create a new channel)")
	}

	c.streamCreated = true
	c.lastStreamID++
	streamID := c.lastStreamID
	if _, ok := c.streams[streamID]; ok {
		// should never happen
		panic(errors.New("sesame/tun/grpc: next stream ID not available"))
	}

	ctx, cncl := context.WithCancel(ctx)
	str := &tunnelClientStream{
		ctx:            ctx,
		cncl:           cncl,
		ch:             c,
		streamID:       streamID,
		method:         methodName,
		stream:         c.stream,
		headerTargets:  hdrs,
		trailerTargets: tlrs,
		isClientStream: clientStreams,
		isServerStream: serverStreams,
		buffer:         make(chan *grpctunnel.ServerToClient, windowMaxBuffer),
		headerSignal:   make(chan struct{}),
		doneSignal:     make(chan struct{}),
	}
	c.streams[streamID] = str

	return str, md, nil
}

func (c *Channel) recvLoop() {
	for {
		msg, err := c.stream.Recv()
		if err != nil {
			_ = c.close(err)
			return
		}

		if msg.GetStreamId() == 0 || msg.GetFrame() == nil {
			_ = c.close(errors.New("sesame/tun/grpc: invalid frame"))
			return
		}

		str, err := c.getStream(msg.StreamId)
		if err != nil {
			_ = c.close(err)
			return
		}

		if _, ok := msg.GetFrame().(*grpctunnel.ServerToClient_WindowUpdate); ok {
			// handled in the flow control wrapper impl.
			continue
		}

		str.acceptServerFrame(msg)
	}
}

func (c *Channel) getStream(streamID uint64) (*tunnelClientStream, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	target, ok := c.streams[streamID]
	if !ok {
		if c.streamCreated && streamID <= c.lastStreamID {
			// used and disposed of stream; ignore subsequent frames
			return nil, nil
		}
		// stream never created!
		return nil, fmt.Errorf("received frame for stream ID %d: stream never created", streamID)
	}

	return target, nil
}

func (c *Channel) removeStream(streamID uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.streams != nil {
		delete(c.streams, streamID)
	}
}

func (c *Channel) close(err error) error {
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.finished {
			return
		}
		c.finished = true
		if err != nil {
			c.err = err
		} else {
			c.err = io.EOF
		}
		for _, stream := range c.streams {
			stream.cncl()
		}
		c.streams = nil
		c.cancel()
	}()
	return c.tearDown()
}

type tunnelClientStream struct {
	ctx      context.Context
	cncl     context.CancelFunc
	ch       *Channel
	streamID uint64
	method   string
	stream   tunnelStreamClient

	fcSend int64
	fcRecv int64

	headerTargets  []*metadata.MD
	trailerTargets []*metadata.MD

	isClientStream bool
	isServerStream bool

	ingestMu     sync.Mutex
	buffer       chan *grpctunnel.ServerToClient
	header       metadata.MD
	headerSignal chan struct{}
	trailer      metadata.MD
	done         error
	doneSignal   chan struct{}

	readMu  sync.Mutex
	readErr error

	// for message frame to server (WARNING: unsafe to hold while sending cancelation etc)
	writeMu sync.Mutex
	hasSent bool
}

func (st *tunnelClientStream) Header() (metadata.MD, error) {
	select {
	case <-st.headerSignal:
		return st.header, nil
	case <-st.ctx.Done():
		select {
		case <-st.headerSignal:
			return st.header, nil
		default:
		}
		return nil, st.ctx.Err()
	}
}

func (st *tunnelClientStream) Trailer() metadata.MD {
	// Unlike Header(), this method does not block and should only be
	// used by client after stream is closed.
	select {
	case <-st.doneSignal:
		return st.trailer
	default:
		return nil
	}
}

func (st *tunnelClientStream) CloseSend() error {
	// replicate the behavior of the grpc implementation...
	// https://github.com/grpc/grpc-go/blob/e63e1230fd01bc4390afdeb27a42c8e631ee9026/stream.go#L865
	select {
	case <-st.doneSignal:
		return nil
	default:
	}
	_ = st.stream.Send(&grpctunnel.ClientToServer{
		StreamId: st.streamID,
		Frame: &grpctunnel.ClientToServer_HalfClose{
			HalfClose: &emptypb.Empty{},
		},
	})
	return nil
}

func (st *tunnelClientStream) Context() context.Context { return st.ctx }

func (st *tunnelClientStream) SendMsg(m interface{}) error {
	st.writeMu.Lock()
	defer st.writeMu.Unlock()

	if !st.hasSent {
		st.hasSent = true
	} else if !st.isClientStream {
		return status.Errorf(codes.Internal, "Already sent response for non-client-stream method %q", st.method)
	}

	b, err := proto.Marshal(m.(proto.Message))
	if err != nil {
		return err
	}

	i := 0
	for {
		if err := st.err(); err != nil {
			return io.EOF
		}

		chunk := b
		if len(b) > maxChunkSize {
			chunk = b[:maxChunkSize]
		}

		if i == 0 {
			err = st.stream.Send(&grpctunnel.ClientToServer{
				StreamId: st.streamID,
				Frame: &grpctunnel.ClientToServer_Message{
					Message: &grpctunnel.EncodedMessage{
						Size: int32(len(b)),
						Data: chunk,
					},
				},
			})
		} else {
			err = st.stream.Send(&grpctunnel.ClientToServer{
				StreamId: st.streamID,
				Frame: &grpctunnel.ClientToServer_MessageData{
					MessageData: chunk,
				},
			})
		}

		if err != nil {
			return err
		}

		if len(b) <= maxChunkSize {
			break
		}

		b = b[maxChunkSize:]
		i++
	}

	return nil
}

func (st *tunnelClientStream) RecvMsg(m interface{}) error {
	data, ok, err := st.readMsg()
	if err != nil {
		if !ok {
			_ = st.cancel(err)
		}
		return err
	}
	return proto.Unmarshal(data, m.(proto.Message))
}

func (st *tunnelClientStream) readMsg() (data []byte, ok bool, err error) {
	st.readMu.Lock()
	defer st.readMu.Unlock()

	data, ok, err = st.readMsgLocked()
	if err == nil && !st.isServerStream {
		// no stream; so eagerly see if there's another message
		// and fail RPC if so (due to bad input)
		_, ok, err := st.readMsgLocked()
		if err == nil {
			err = status.Errorf(codes.Internal, "Server sent multiple responses for non-server-stream method %q", st.method)
			st.readErr = err
			return nil, false, err
		}
		if err != io.EOF || !ok {
			return nil, ok, err
		}
	}

	return data, ok, err
}

func (st *tunnelClientStream) readMsgLocked() (data []byte, ok bool, err error) {
	if st.readErr != nil {
		return nil, true, st.readErr
	}

	defer func() {
		if err != nil {
			st.readErr = err
		}
	}()

	msgLen := -1
	var b []byte
	for {
		msg, ok := <-st.buffer
		if !ok {
			// don't need lock to read st.done; observing
			// input channel close provides safe visibility
			return nil, true, st.done
		}

		st.ch.updateWindow(msg)

		switch frame := msg.GetFrame().(type) {
		case *grpctunnel.ServerToClient_Message:
			if msgLen != -1 {
				return nil, false, status.Errorf(codes.Internal, "server sent redundant response message envelope")
			}
			msgLen = int(frame.Message.Size)
			b = frame.Message.Data
			if len(b) > msgLen {
				return nil, false, status.Errorf(codes.Internal, "server sent more data than indicated by response message envelope")
			}
			if len(b) == msgLen {
				return b, true, nil
			}

		case *grpctunnel.ServerToClient_MessageData:
			if msgLen == -1 {
				return nil, false, status.Errorf(codes.Internal, "server never sent envelope for response message")
			}
			b = append(b, frame.MessageData...)
			if len(b) > msgLen {
				return nil, false, status.Errorf(codes.Internal, "server sent more data than indicated by response message envelope")
			}
			if len(b) == msgLen {
				return b, true, nil
			}

		default:
			return nil, false, status.Errorf(codes.Internal, "unrecognized frame type: %T", frame)
		}
	}
}

func (st *tunnelClientStream) err() error {
	select {
	case <-st.doneSignal:
		return st.done
	default:
		return st.ctx.Err()
	}
}

func (st *tunnelClientStream) acceptServerFrame(msg *grpctunnel.ServerToClient) {
	if st == nil {
		// can happen if client decided that the stream ID was recently used
		// yet inactive -- it returns nil error but also nil stream, which
		// just discards incoming messages (we assume they arrive late, racing
		// with stream being closed)
		return
	}

	switch frame := msg.GetFrame().(type) {
	case *grpctunnel.ServerToClient_Header:
		st.ingestMu.Lock()
		defer st.ingestMu.Unlock()
		select {
		case <-st.headerSignal:
		default:
			st.header = fromProto(frame.Header)
			for _, target := range st.headerTargets {
				*target = st.header
			}
			close(st.headerSignal)
		}
		return

	case *grpctunnel.ServerToClient_CloseStream_:
		trailer := fromProto(frame.CloseStream.Trailer)
		err := status.FromProto(frame.CloseStream.Status).Err()
		st.finishStream(err, trailer)
	}

	st.ingestMu.Lock()
	defer st.ingestMu.Unlock()

	if st.done != nil {
		return
	}

	// without considering external factors (like network IO),
	// this should never block, if both ends have implemented correct
	// (and compatible) flow control
	select {
	case st.buffer <- msg:
	case <-st.ctx.Done():
	}
}

func (st *tunnelClientStream) cancel(err error) error {
	st.finishStream(err, nil)
	// let server know
	return st.stream.Send(&grpctunnel.ClientToServer{
		StreamId: st.streamID,
		Frame: &grpctunnel.ClientToServer_Cancel{
			Cancel: &emptypb.Empty{},
		},
	})
}

func (st *tunnelClientStream) finishStream(err error, trailer metadata.MD) {
	// only remove from the parent once everything is cleaned up
	defer st.ch.removeStream(st.streamID)

	// only cancel the context after all state updates are applied
	// this is necessary for the behavior of methods like tunnelClientStream.err
	defer st.cncl()

	st.ingestMu.Lock()
	defer st.ingestMu.Unlock()

	// guard against multiple close attempts
	if st.done != nil {
		// RPC already finished! just ignore...
		return
	}

	// update trailer targets and tunnelClientStream.Trailer
	st.trailer = trailer
	for _, target := range st.trailerTargets {
		*target = trailer
	}

	// unblock header requests - we aren't getting one
	select {
	case <-st.headerSignal:
	default:
		close(st.headerSignal)
	}

	// update the value to be returned by tunnelClientStream.err
	switch err {
	case nil:
		err = io.EOF
	case context.DeadlineExceeded:
		err = status.Error(codes.DeadlineExceeded, err.Error())
	case context.Canceled:
		err = status.Error(codes.Canceled, err.Error())
	}
	st.done = err

	// EOF for buffer, for receives
	// has to be closed after st.done has been updated
	close(st.buffer)

	// has to be closed after st.done and st.trailer have been updated
	// see also tunnelClientStream.err tunnelClientStream.Trailer
	close(st.doneSignal)
}
