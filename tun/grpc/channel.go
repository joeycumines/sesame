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
	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
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
	return newChannel(wrapTunnelStreamClient(c.stream.get().stream, c.stream.get().tearDown)), nil
}

type tunnelStreamClient interface {
	Context() context.Context
	Send(*ClientToServer) error
	Recv() (*ServerToClient, error)
}

// Channel is a tunnel client, and implements grpc.ClientConnInterface.
// It is backed by a single stream, though this stream may be either a tunnel client, or a reverse tunnel server.
type Channel struct {
	stream   tunnelStreamClient
	ctx      context.Context
	cancel   context.CancelFunc
	tearDown func() error

	mu            sync.RWMutex
	streams       map[uint64]*tunnelClientStream
	lastStreamID  uint64
	streamCreated bool
	err           error
	finished      bool
}

var _ grpc.ClientConnInterface = (*Channel)(nil)

func newChannel(stream tunnelStreamClient, tearDown func() error) *Channel {
	ctx, cancel := context.WithCancel(stream.Context())
	c := &Channel{
		stream:   stream,
		ctx:      ctx,
		cancel:   cancel,
		tearDown: tearDown,
		streams:  make(map[uint64]*tunnelClientStream),
	}
	go c.recvLoop()
	return c
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
	err = c.stream.Send(&ClientToServer{
		StreamId: str.streamID,
		Frame: &ClientToServer_NewStream_{
			NewStream: &ClientToServer_NewStream{
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
			hdrs = append(hdrs, opt.HeaderAddr)

		case grpc.TrailerCallOption:
			tlrs = append(tlrs, opt.TrailerAddr)

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

			// TODO: custom codec and compressor support
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

	ch := make(chan isServerToClient_Frame, 1)
	ctx, cncl := context.WithCancel(ctx)
	str := &tunnelClientStream{
		ctx:              ctx,
		cncl:             cncl,
		ch:               c,
		streamID:         streamID,
		method:           methodName,
		stream:           c.stream,
		headersTargets:   hdrs,
		trailersTargets:  tlrs,
		isClientStream:   clientStreams,
		isServerStream:   serverStreams,
		ingestChan:       ch,
		readChan:         ch,
		gotHeadersSignal: make(chan struct{}),
		doneSignal:       make(chan struct{}),
	}
	c.streams[streamID] = str

	return str, md, nil
}

func (c *Channel) recvLoop() {
	for {
		in, err := c.stream.Recv()
		if err != nil {
			_ = c.close(err)
			return
		}
		str, err := c.getStream(in.StreamId)
		if err != nil {
			_ = c.close(err)
			return
		}
		str.acceptServerFrame(in.Frame)
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

func (c *Channel) close(err error) (tearDownErr error) {
	if c.tearDown != nil {
		tearDownErr = c.tearDown()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.finished {
		return
	}

	defer c.cancel()

	c.finished = true
	if err == nil {
		err = io.EOF
	}
	c.err = err
	for _, st := range c.streams {
		st.cncl()
	}
	c.streams = nil
	return
}

type tunnelClientStream struct {
	ctx      context.Context
	cncl     context.CancelFunc
	ch       *Channel
	streamID uint64
	method   string
	stream   tunnelStreamClient

	headersTargets  []*metadata.MD
	trailersTargets []*metadata.MD

	isClientStream bool
	isServerStream bool

	// for "ingesting" frames into channel, from receive loop
	ingestMu         sync.Mutex
	ingestChan       chan<- isServerToClient_Frame
	gotHeaders       bool
	gotHeadersSignal chan struct{}
	headers          metadata.MD
	done             error
	doneSignal       chan struct{}
	trailers         metadata.MD

	// for reading frames from channel, to read message data
	readMu   sync.Mutex
	readChan <-chan isServerToClient_Frame
	readErr  error

	// for sending frames to server
	writeMu    sync.Mutex
	numSent    uint32
	halfClosed bool
}

func (st *tunnelClientStream) Header() (metadata.MD, error) {
	// if we've already received headers, return them
	select {
	case <-st.gotHeadersSignal:
		return st.headers, nil
	default:
	}

	select {
	case <-st.gotHeadersSignal:
		return st.headers, nil
	case <-st.ctx.Done():
		// in the event of a race, always respect getting headers first
		select {
		case <-st.gotHeadersSignal:
			return st.headers, nil
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
		return st.trailers
	default:
		return nil
	}
}

func (st *tunnelClientStream) CloseSend() error {
	st.writeMu.Lock()
	defer st.writeMu.Unlock()

	select {
	case <-st.doneSignal:
		return st.done
	default:
		// don't block since we are holding writeMu
	}

	if st.halfClosed {
		return errors.New("already half-closed")
	}
	st.halfClosed = true
	return st.stream.Send(&ClientToServer{
		StreamId: st.streamID,
		Frame: &ClientToServer_HalfClose{
			HalfClose: &empty.Empty{},
		},
	})
}

func (st *tunnelClientStream) Context() context.Context {
	return st.ctx
}

func (st *tunnelClientStream) SendMsg(m interface{}) error {
	st.writeMu.Lock()
	defer st.writeMu.Unlock()

	if !st.isClientStream && st.numSent == 1 {
		return status.Errorf(codes.Internal, "Already sent response for non-server-stream method %q", st.method)
	}
	st.numSent++

	// TODO: support alternate codecs, compressors, etc
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
			err = st.stream.Send(&ClientToServer{
				StreamId: st.streamID,
				Frame: &ClientToServer_Message{
					Message: &EncodedMessage{
						Size: int32(len(b)),
						Data: chunk,
					},
				},
			})
		} else {
			err = st.stream.Send(&ClientToServer{
				StreamId: st.streamID,
				Frame: &ClientToServer_MessageData{
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
	// TODO: support alternate codecs, compressors, etc
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
		in, ok := <-st.readChan
		if !ok {
			// don't need lock to read st.done; observing
			// input channel close provides safe visibility
			return nil, true, st.done
		}

		switch in := in.(type) {
		case *ServerToClient_Message:
			if msgLen != -1 {
				return nil, false, status.Errorf(codes.Internal, "server sent redundant response message envelope")
			}
			msgLen = int(in.Message.Size)
			b = in.Message.Data
			if len(b) > msgLen {
				return nil, false, status.Errorf(codes.Internal, "server sent more data than indicated by response message envelope")
			}
			if len(b) == msgLen {
				return b, true, nil
			}

		case *ServerToClient_MessageData:
			if msgLen == -1 {
				return nil, false, status.Errorf(codes.Internal, "server never sent envelope for response message")
			}
			b = append(b, in.MessageData...)
			if len(b) > msgLen {
				return nil, false, status.Errorf(codes.Internal, "server sent more data than indicated by response message envelope")
			}
			if len(b) == msgLen {
				return b, true, nil
			}

		default:
			return nil, false, status.Errorf(codes.Internal, "unrecognized frame type: %T", in)
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

func (st *tunnelClientStream) acceptServerFrame(frame isServerToClient_Frame) {
	if st == nil {
		// can happen if client decided that the stream ID was recently used
		// yet inactive -- it returns nil error but also nil stream, which
		// just discards incoming messages (we assume they arrive late, racing
		// with stream being closed)
		return
	}

	switch frame := frame.(type) {
	case *ServerToClient_Header:
		st.ingestMu.Lock()
		defer st.ingestMu.Unlock()
		if st.gotHeaders {
			// TODO: cancel RPC and fail locally with internal error?
			return
		}
		st.gotHeaders = true
		st.headers = fromProto(frame.Header)
		for _, hdrs := range st.headersTargets {
			*hdrs = st.headers
		}
		close(st.gotHeadersSignal)
		return

	case *ServerToClient_CloseStream_:
		trailers := fromProto(frame.CloseStream.Trailer)
		err := status.FromProto(frame.CloseStream.Status).Err()
		st.finishStream(err, trailers)
	}

	st.ingestMu.Lock()
	defer st.ingestMu.Unlock()

	if st.done != nil {
		return
	}

	select {
	case st.ingestChan <- frame:
	case <-st.ctx.Done():
	}
}

func (st *tunnelClientStream) cancel(err error) error {
	st.finishStream(err, nil)
	// let server know
	st.writeMu.Lock()
	defer st.writeMu.Unlock()
	return st.stream.Send(&ClientToServer{
		StreamId: st.streamID,
		Frame: &ClientToServer_Cancel{
			Cancel: &empty.Empty{},
		},
	})
}

func (st *tunnelClientStream) finishStream(err error, trailers metadata.MD) {
	st.ch.removeStream(st.streamID)
	defer st.cncl()

	st.ingestMu.Lock()
	defer st.ingestMu.Unlock()

	if st.done != nil {
		// RPC already finished! just ignore...
		return
	}
	st.trailers = trailers
	for _, tlrs := range st.trailersTargets {
		*tlrs = trailers
	}
	if !st.gotHeaders {
		st.gotHeaders = true
		close(st.gotHeadersSignal)
	}
	switch err {
	case nil:
		err = io.EOF
	case context.DeadlineExceeded:
		err = status.Error(codes.DeadlineExceeded, err.Error())
	case context.Canceled:
		err = status.Error(codes.Canceled, err.Error())
	}
	st.done = err

	close(st.ingestChan)
	close(st.doneSignal)
}
