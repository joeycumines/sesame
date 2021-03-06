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
	"github.com/fullstorydev/grpchan"
	"github.com/joeycumines/sesame/internal/flowcontrol"
	"github.com/joeycumines/sesame/type/grpcmetadata"
	"github.com/joeycumines/sesame/type/grpctunnel"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"io"
	"strings"
	"sync"
)

const (
	// maxChunkSize is used to chunk the encoded messages (aiming to be 1-1 with HTTP/2 data frames)
	// NOTE: fudge factor is for whatever else goes into the data frames (should probably check that)
	maxChunkSize = maxChunkSizeBase - 100
	// maxChunkSizeBase is maxChunkSize w/o fudge factor
	maxChunkSizeBase = grpcMaxFrameSize - 28
	// currently hardcoded to the minimum (for HTTP/2)
	// https://github.com/grpc/grpc-go/blob/master/internal/transport/http_util.go#L47
	// relevant issue https://github.com/grpc/grpc-go/issues/4630
	grpcMaxFrameSize = 16384
)

var (
	// compile time assertions
	_ = map[bool]struct{}{false: {}, grpcMaxFrameSize > 0: {}}
	_ = map[bool]struct{}{false: {}, maxChunkSizeBase > 0 && maxChunkSizeBase < grpcMaxFrameSize: {}}
	_ = map[bool]struct{}{false: {}, maxChunkSize > 0 && maxChunkSize <= maxChunkSizeBase: {}}
)

func ServeTunnel(options ...TunnelOption) error {
	var c tunnelConfig
	for _, o := range options {
		o(&c)
	}
	if err := c.validate(); err != nil {
		return err
	}
	return serveTunnel(&c)
}

func serveTunnel(c *tunnelConfig) error {
	stream := c.stream.get().stream

	// wrap the stream to add concurrency controls
	stream = newTunnelStreamServerWrapper(stream)

	// initialise flow control, wrapping stream
	// WARNING not ready for use yet (needs to be wired up with the tunnelServer)
	fcInterface := &flowControlServer{tunnelStreamServer: stream}
	fcConn := flowcontrol.NewConn[uint64, *grpctunnel.ServerToClient, *grpctunnel.ClientToServer](fcInterface)
	stream = fcConn

	tun := tunnelServer{
		stream:       stream,
		updateWindow: fcConn.UpdateWindow,
		services:     c.handlers.get().val.grpchan(),
		stop:         c.stop.get().ch,
		streams:      make(map[uint64]*tunnelServerStream),
	}

	// finish wiring up fcInterface with tun
	fcInterface.tun = &tun

	// start serving the tunnel
	return tun.serve()
}

type tunnelStreamServer interface {
	Context() context.Context
	Send(*grpctunnel.ServerToClient) error
	Recv() (*grpctunnel.ClientToServer, error)
}

type tunnelServer struct {
	stream       tunnelStreamServer
	updateWindow func(msg *grpctunnel.ClientToServer)
	services     grpchan.HandlerMap
	stop         <-chan struct{} // graceful

	// WARNING this mutex MUST NOT be held while blocking on sending to a stream
	mu       sync.RWMutex
	streams  map[uint64]*tunnelServerStream
	lastSeen uint64
}

func (s *tunnelServer) serve() error {
	ctx, cancel := context.WithCancel(s.stream.Context())
	defer cancel()

	// implements graceful stop
	go func() {
		select {
		case <-ctx.Done():
		case <-s.stop:
			if s.waitAllStreams(ctx) == nil {
				cancel()
			}
		}
	}()

	// the behavior below is necessary due to ctx not controlling the stream send/receive (blocking)
	// could use a tidy up - didn't bother restructuring this when adding graceful stop
	out := make(chan error, 1)
	go func() {
		defer cancel()
		out <- func() error {
			for {
				msg, err := s.stream.Recv()
				if err != nil {
					if err == io.EOF {
						return nil
					}
					return err
				}

				if msg.GetStreamId() == 0 || msg.GetFrame() == nil {
					return status.Error(codes.InvalidArgument, "invalid frame")
				}

				if f, ok := msg.Frame.(*grpctunnel.ClientToServer_NewStream_); ok {
					if ok, err := s.createStream(ctx, msg.StreamId, f.NewStream); err != nil {
						if !ok {
							return err
						} else {
							st, _ := status.FromError(err)
							_ = s.stream.Send(&grpctunnel.ServerToClient{
								StreamId: msg.StreamId,
								Frame: &grpctunnel.ServerToClient_CloseStream_{
									CloseStream: &grpctunnel.ServerToClient_CloseStream{
										Status: st.Proto(),
									},
								},
							})
						}
					}
					continue
				}

				str, err := s.getStream(msg.StreamId)
				if err != nil {
					return err
				}

				if _, ok := msg.GetFrame().(*grpctunnel.ClientToServer_WindowUpdate); ok {
					// handled in the flow control wrapper impl.
					continue
				}

				str.acceptClientFrame(msg)
			}
		}()
	}()

	// only block until context cancel (support graceful close)
	// WARNING the recv loop above may still be blocking
	<-ctx.Done()
	select {
	case err := <-out:
		return err
	default:
		return nil
	}
}

func (s *tunnelServer) waitAllStreams(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var closed chan struct{}
		s.mu.RLock()
		for _, stream := range s.streams {
			select {
			case <-stream.closed:
				continue
			default:
			}
			closed = stream.closed
			break
		}
		s.mu.RUnlock()
		if closed == nil {
			// there weren't any streams that were not closed
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-closed:
			// we have to try again though, as we released the lock
		}
	}
}

func (s *tunnelServer) createStream(ctx context.Context, streamID uint64, frame *grpctunnel.ClientToServer_NewStream) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.streams[streamID]
	if ok {
		// stream already active!
		return false, fmt.Errorf("cannot create stream ID %d: already exists", streamID)
	}
	if s.lastSeen < streamID {
		s.lastSeen = streamID
	}

	select {
	case <-s.stop:
		return true, status.Error(codes.Unavailable, `transport is closing`)
	default:
	}

	var parts []string
	if method := frame.GetMethod(); method != `` {
		if method[0] == '/' {
			method = method[1:]
		}
		parts = strings.SplitN(method, "/", 2)
	}
	if len(parts) != 2 {
		return true, status.Errorf(codes.InvalidArgument, "%q is not a well-formed method name", frame.GetMethod())
	}

	var md interface{}
	sd, svc := s.services.QueryService(parts[0])
	if sd != nil {
		md = findMethod(sd, parts[1])
	}
	var isClientStream, isServerStream bool
	if streamDesc, ok := md.(*grpc.StreamDesc); ok {
		isClientStream, isServerStream = streamDesc.ClientStreams, streamDesc.ServerStreams
	}

	if md == nil {
		return true, status.Errorf(codes.Unimplemented, "%s not implemented", frame.Method)
	}

	ctx = metadata.NewIncomingContext(ctx, fromProto(frame.Header))

	str := &tunnelServerStream{
		ctx:            ctx,
		tun:            s,
		streamID:       streamID,
		method:         frame.Method,
		stream:         s.stream,
		isClientStream: isClientStream,
		isServerStream: isServerStream,
		buffer:         make(chan *grpctunnel.ClientToServer, windowMaxBuffer),
		closed:         make(chan struct{}),
	}
	s.streams[streamID] = str
	str.ctx = grpc.NewContextWithServerTransportStream(str.ctx, (*tunnelServerTransportStream)(str))
	go str.serveStream(md, svc)
	return true, nil
}

func (s *tunnelServer) getStream(streamID uint64) (*tunnelServerStream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	target, ok := s.streams[streamID]
	if !ok {
		if streamID <= s.lastSeen {
			// used and disposed of stream; ignore subsequent frames
			return nil, nil
		}
		// stream never created!
		return nil, fmt.Errorf("received frame for stream ID %d: stream never created", streamID)
	}

	return target, nil
}

func (s *tunnelServer) removeStream(streamID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.streams, streamID)
}

func findMethod(sd *grpc.ServiceDesc, method string) interface{} {
	for i, md := range sd.Methods {
		if md.MethodName == method {
			return &sd.Methods[i]
		}
	}
	for i, md := range sd.Streams {
		if md.StreamName == method {
			return &sd.Streams[i]
		}
	}
	return nil
}

type tunnelServerStream struct {
	ctx      context.Context
	tun      *tunnelServer
	streamID uint64
	method   string
	stream   tunnelStreamServer

	fcSend int64
	fcRecv int64

	isClientStream bool
	isServerStream bool

	ingestMu   sync.Mutex
	buffer     chan *grpctunnel.ClientToServer
	halfClosed error

	readMu  sync.Mutex
	readErr error

	stateMu    sync.Mutex
	header     metadata.MD
	trailer    metadata.MD
	sentHeader bool
	closed     chan struct{}

	writeMu sync.Mutex
	hasSent bool
}

func (st *tunnelServerStream) acceptClientFrame(msg *grpctunnel.ClientToServer) {
	if st == nil {
		// can happen if server decided that the stream ID was recently used
		// yet inactive -- it returns nil error but also nil stream, which
		// just discards incoming messages (we assume they arrive late, racing
		// with stream being closed)
		return
	}

	switch msg.GetFrame().(type) {
	case *grpctunnel.ClientToServer_HalfClose:
		st.halfClose(io.EOF)
		return

	case *grpctunnel.ClientToServer_Cancel:
		st.finishStream(context.Canceled)
		return
	}

	st.ingestMu.Lock()
	defer st.ingestMu.Unlock()

	if st.halfClosed != nil {
		// stream is half closed -- ignore subsequent messages
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

func (st *tunnelServerStream) SetHeader(md metadata.MD) error {
	return st.setHeader(md, false)
}

func (st *tunnelServerStream) SendHeader(md metadata.MD) error {
	return st.setHeader(md, true)
}

func (st *tunnelServerStream) setHeader(md metadata.MD, send bool) error {
	st.stateMu.Lock()
	defer st.stateMu.Unlock()
	if st.sentHeader {
		return errors.New("already sent header")
	}
	if md != nil {
		st.header = metadata.Join(st.header, md)
	}
	if send {
		return st.sendHeadersLocked()
	}
	return nil
}

func (st *tunnelServerStream) sendHeadersLocked() error {
	err := st.stream.Send(&grpctunnel.ServerToClient{
		StreamId: st.streamID,
		Frame: &grpctunnel.ServerToClient_Header{
			Header: toProto(st.header),
		},
	})
	st.header = nil
	st.sentHeader = true
	return err
}

func fromProto(md *grpcmetadata.GrpcMetadata) metadata.MD {
	if md == nil {
		return nil
	}
	vals := make(metadata.MD, len(md.GetData()))
	for k, v := range md.GetData() {
		switch v := v.GetData().(type) {
		case *grpcmetadata.GrpcMetadata_Value_Str:
			vals[k] = v.Str.GetValues()
		case *grpcmetadata.GrpcMetadata_Value_Bin:
			var a []string
			if l := len(v.Bin.GetValues()); l != 0 {
				a = make([]string, 0, l)
				for _, v := range v.Bin.GetValues() {
					a = append(a, string(v))
				}
			}
			vals[k] = a
		default:
			vals[k] = nil
		}
	}
	return vals
}

func toProto(md metadata.MD) *grpcmetadata.GrpcMetadata {
	var r grpcmetadata.GrpcMetadata
	r.Data = make(map[string]*grpcmetadata.GrpcMetadata_Value, len(md))
	for k, v := range md {
		if strings.HasSuffix(k, `-bin`) {
			var a [][]byte
			if len(v) != 0 {
				a = make([][]byte, 0, len(v))
				for _, v := range v {
					a = append(a, []byte(v))
				}
			}
			r.Data[k] = &grpcmetadata.GrpcMetadata_Value{Data: &grpcmetadata.GrpcMetadata_Value_Bin{Bin: &grpcmetadata.GrpcMetadata_Bin{Values: a}}}
		} else {
			r.Data[k] = &grpcmetadata.GrpcMetadata_Value{Data: &grpcmetadata.GrpcMetadata_Value_Str{Str: &grpcmetadata.GrpcMetadata_Str{Values: v}}}
		}
	}
	return &r
}

func (st *tunnelServerStream) SetTrailer(md metadata.MD) {
	_ = st.setTrailer(md)
}

func (st *tunnelServerStream) setTrailer(md metadata.MD) error {
	st.stateMu.Lock()
	defer st.stateMu.Unlock()
	select {
	case <-st.closed:
		return errors.New("already finished")
	default:
		st.trailer = metadata.Join(st.trailer, md)
		return nil
	}
}

func (st *tunnelServerStream) Context() context.Context {
	return st.ctx
}

func (st *tunnelServerStream) SendMsg(m interface{}) error {
	// send header if not already sent
	_ = st.setHeader(nil, true)

	st.writeMu.Lock()
	defer st.writeMu.Unlock()

	if !st.hasSent {
		st.hasSent = true
	} else if !st.isServerStream {
		return status.Errorf(codes.Internal, "Already sent response for non-server-stream method %q", st.method)
	}

	b, err := proto.Marshal(m.(proto.Message))
	if err != nil {
		return err
	}

	i := 0
	for {
		if err := st.ctx.Err(); err != nil {
			return err
		}

		chunk := b
		if len(b) > maxChunkSize {
			chunk = b[:maxChunkSize]
		}

		if i == 0 {
			err = st.stream.Send(&grpctunnel.ServerToClient{
				StreamId: st.streamID,
				Frame: &grpctunnel.ServerToClient_Message{
					Message: &grpctunnel.EncodedMessage{
						Size: int32(len(b)),
						Data: chunk,
					},
				},
			})
		} else {
			err = st.stream.Send(&grpctunnel.ServerToClient{
				StreamId: st.streamID,
				Frame: &grpctunnel.ServerToClient_MessageData{
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

func (st *tunnelServerStream) RecvMsg(m interface{}) error {
	data, ok, err := st.readMsg()
	if err != nil {
		if !ok {
			st.finishStream(err)
		}
		return err
	}
	return proto.Unmarshal(data, m.(proto.Message))
}

func (st *tunnelServerStream) readMsg() (data []byte, ok bool, err error) {
	st.readMu.Lock()
	defer st.readMu.Unlock()

	data, ok, err = st.readMsgLocked()
	if err == nil && !st.isClientStream {
		// no stream; so eagerly see if there's another message
		// and fail RPC if so (due to bad input)
		_, ok, err := st.readMsgLocked()
		if err == nil {
			err = status.Errorf(codes.InvalidArgument, "Already received request for non-client-stream method %q", st.method)
			st.readErr = err
			return nil, false, err
		}
		if err != io.EOF || !ok {
			return nil, ok, err
		}
	}

	return data, ok, err
}

func (st *tunnelServerStream) readMsgLocked() (data []byte, ok bool, err error) {
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
		// if stream is canceled, return context error
		if err := st.ctx.Err(); err != nil {
			return nil, true, err
		}

		// otherwise, try to read request data, but interrupt if
		// stream is canceled or half-closed
		select {
		case <-st.ctx.Done():
			return nil, true, st.ctx.Err()

		case msg, ok := <-st.buffer:
			if !ok {
				// don't need lock to read st.halfClosed; observing
				// input channel close provides safe visibility
				return nil, true, st.halfClosed
			}

			st.tun.updateWindow(msg)

			switch frame := msg.GetFrame().(type) {
			case *grpctunnel.ClientToServer_Message:
				if msgLen != -1 {
					return nil, false, status.Errorf(codes.InvalidArgument, "received redundant request message envelope")
				}
				msgLen = int(frame.Message.Size)
				b = frame.Message.Data
				if len(b) > msgLen {
					return nil, false, status.Errorf(codes.InvalidArgument, "received more data than indicated by request message envelope")
				}
				if len(b) == msgLen {
					return b, true, nil
				}

			case *grpctunnel.ClientToServer_MessageData:
				if msgLen == -1 {
					return nil, false, status.Errorf(codes.InvalidArgument, "never received envelope for request message")
				}
				b = append(b, frame.MessageData...)
				if len(b) > msgLen {
					return nil, false, status.Errorf(codes.InvalidArgument, "received more data than indicated by request message envelope")
				}
				if len(b) == msgLen {
					return b, true, nil
				}

			default:
				return nil, false, status.Errorf(codes.InvalidArgument, "unrecognized frame type: %T", frame)
			}
		}
	}
}

func (st *tunnelServerStream) serveStream(md interface{}, srv interface{}) {
	var err error
	panicked := true // pessimistic assumption

	defer func() {
		if err == nil && panicked {
			err = status.Errorf(codes.Internal, "panic")
		}
		st.finishStream(err)
	}()

	switch md := md.(type) {
	case *grpc.MethodDesc:
		var resp interface{}
		resp, err = md.Handler(srv, st.ctx, st.RecvMsg, nil)
		if err == nil {
			err = st.SendMsg(resp)
		}
	case *grpc.StreamDesc:
		err = md.Handler(srv, st)
	default:
		err = status.Errorf(codes.Internal, "unknown type of method desc: %T", md)
	}
	// if we get here, we did not panic
	panicked = false
}

func (st *tunnelServerStream) finishStream(err error) {
	defer st.tun.removeStream(st.streamID)

	st.halfClose(err)

	st.stateMu.Lock()
	defer st.stateMu.Unlock()

	select {
	case <-st.closed:
		// already closed
		return
	default:
	}

	if !st.sentHeader {
		_ = st.sendHeadersLocked()
	}

	stat, _ := status.FromError(err)
	_ = st.stream.Send(&grpctunnel.ServerToClient{
		StreamId: st.streamID,
		Frame: &grpctunnel.ServerToClient_CloseStream_{
			CloseStream: &grpctunnel.ServerToClient_CloseStream{
				Status:  stat.Proto(),
				Trailer: toProto(st.trailer),
			},
		},
	})
	st.trailer = nil

	close(st.closed)
}

func (st *tunnelServerStream) halfClose(err error) {
	st.ingestMu.Lock()
	defer st.ingestMu.Unlock()
	if st.halfClosed != nil {
		// already closed
		return
	}
	if err != nil {
		st.halfClosed = err
	} else {
		st.halfClosed = io.EOF
	}
	close(st.buffer)
}

type tunnelServerTransportStream tunnelServerStream

func (st *tunnelServerTransportStream) Method() string {
	return (*tunnelServerStream)(st).method
}

func (st *tunnelServerTransportStream) SetHeader(md metadata.MD) error {
	return (*tunnelServerStream)(st).SetHeader(md)
}

func (st *tunnelServerTransportStream) SendHeader(md metadata.MD) error {
	return (*tunnelServerStream)(st).SendHeader(md)
}

func (st *tunnelServerTransportStream) SetTrailer(md metadata.MD) error {
	return (*tunnelServerStream)(st).setTrailer(md)
}
