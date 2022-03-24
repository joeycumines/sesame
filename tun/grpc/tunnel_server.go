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
	"github.com/joeycumines/sesame/genproto/type/grpcmetadata"
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

const maxChunkSize = 16384

// ServeTunnel uses the given services to handle incoming RPC requests that
// arrive via the given incoming tunnel stream.
//
// It returns if, in the process of reading requests, it detects invalid usage
// of the stream (client sending references to invalid stream IDs or sending
// frames for a stream ID in improper order) or if the stream itself fails (for
// example, if the client cancels the tunnel or there is a network disruption).
//
// This is typically called from a handler that implements the TunnelService.
// Typical usage looks like so:
//
//    func (h tunnelHandler) OpenTunnel(stream grpctunnel.TunnelService_OpenTunnelServer) error {
//        return grpctunnel.ServeTunnel(stream, h.handlers)
//    }
//
func ServeTunnel(stream TunnelService_OpenTunnelServer, handlers grpchan.HandlerMap) error {
	return serveTunnel(stream, handlers)
}

// ServeReverseTunnel uses the given services to handle incoming RPC requests
// that arrive via the given client tunnel stream. Since this is a reverse
// tunnel, RPC requests are initiated by the server, and this end (the client)
// processes the requests and sends responses.
//
// It returns if, in the process of reading requests, it detects invalid usage
// of the stream (client sending references to invalid stream IDs or sending
// frames for a stream ID in improper order) or if the stream itself fails (for
// example, if the client cancels the tunnel or there is a network disruption).
//
// On return the provided stream should be canceled as soon as possible. Typical
// usage looks like so:
//
//    ctx, cancel := context.WithCancel(ctx)
//    defer cancel()
//    stream, err := stub.OpenReverseTunnel(ctx)
//    if err != nil {
//        return err
//    }
//    return grpctunnel.ServeReverseTunnel(stream, handlers)
//
func ServeReverseTunnel(stream TunnelService_OpenReverseTunnelClient, handlers grpchan.HandlerMap) error {
	return serveTunnel(stream, handlers)
}

// TODO: how to expose API to allow for graceful shutdown? Maybe above functions
// should return the server, whose serve method could be exported. It could also
// provide a Stop and GracefulStop method. If requests are received while the
// channel is in graceful-stopping mode, it could immediately fail them with an
// "unavailable" response code.

func serveTunnel(stream tunnelStreamServer, handlers grpchan.HandlerMap) error {
	svr := &tunnelServer{
		stream:   stream,
		services: handlers,
		streams:  make(map[uint64]*tunnelServerStream),
	}
	return svr.serve()
}

type tunnelStreamServer interface {
	Context() context.Context
	Send(*ServerToClient) error
	Recv() (*ClientToServer, error)
}

type tunnelServer struct {
	stream   tunnelStreamServer
	services grpchan.HandlerMap

	mu       sync.RWMutex
	streams  map[uint64]*tunnelServerStream
	lastSeen uint64
}

func (s *tunnelServer) serve() error {
	ctx, cancel := context.WithCancel(s.stream.Context())
	defer cancel()
	for {
		in, err := s.stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if f, ok := in.Frame.(*ClientToServer_NewStream_); ok {
			if ok, err := s.createStream(ctx, in.StreamId, f.NewStream); err != nil {
				if !ok {
					return err
				} else {
					st, _ := status.FromError(err)
					_ = s.stream.Send(&ServerToClient{
						StreamId: in.StreamId,
						Frame: &ServerToClient_CloseStream_{
							CloseStream: &ServerToClient_CloseStream{
								Status: st.Proto(),
							},
						},
					})
				}
			}
			continue
		}

		str, err := s.getStream(in.StreamId)
		if err != nil {
			return err
		}
		str.acceptClientFrame(in.Frame)
	}
}

func (s *tunnelServer) createStream(ctx context.Context, streamID uint64, frame *ClientToServer_NewStream) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.streams[streamID]
	if ok {
		// stream already active!
		return false, fmt.Errorf("cannot create stream ID %d: already exists", streamID)
	}
	if streamID <= s.lastSeen {
		return false, fmt.Errorf("cannot create stream ID %d: that ID has already been used", streamID)
	}
	s.lastSeen = streamID

	if frame.Method[0] == '/' {
		frame.Method = frame.Method[1:]
	}
	parts := strings.SplitN(frame.Method, "/", 2)
	if len(parts) != 2 {
		return true, status.Errorf(codes.InvalidArgument, "%s is not a well-formed method name", frame.Method)
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
		delete(s.streams, streamID)
		return true, status.Errorf(codes.Unimplemented, "%s not implemented", frame.Method)
	}

	ctx = metadata.NewIncomingContext(ctx, fromProto(frame.Header))

	ch := make(chan isClientToServer_Frame, 1)
	str := &tunnelServerStream{
		ctx:            ctx,
		svr:            s,
		streamID:       streamID,
		method:         frame.Method,
		stream:         s.stream,
		isClientStream: isClientStream,
		isServerStream: isServerStream,
		readChan:       ch,
		ingestChan:     ch,
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
	svr      *tunnelServer
	streamID uint64
	method   string
	stream   tunnelStreamServer

	isClientStream bool
	isServerStream bool

	// for "ingesting" frames into channel, from receive loop
	ingestMu   sync.Mutex
	ingestChan chan<- isClientToServer_Frame
	halfClosed error

	// for reading frames from channel, to read message data
	readMu   sync.Mutex
	readChan <-chan isClientToServer_Frame
	readErr  error

	// for sending frames to client
	writeMu     sync.Mutex
	numSent     uint32
	headers     metadata.MD
	trailers    metadata.MD
	sentHeaders bool
	closed      bool
}

func (st *tunnelServerStream) acceptClientFrame(frame isClientToServer_Frame) {
	if st == nil {
		// can happen if server decided that the stream ID was recently used
		// yet inactive -- it returns nil error but also nil stream, which
		// just discards incoming messages (we assume they arrive late, racing
		// with stream being closed)
		return
	}

	switch frame.(type) {
	case *ClientToServer_HalfClose:
		st.halfClose(io.EOF)
		return

	case *ClientToServer_Cancel:
		st.finishStream(context.Canceled)
		return
	}

	st.ingestMu.Lock()
	defer st.ingestMu.Unlock()

	if st.halfClosed != nil {
		// stream is half closed -- ignore subsequent messages
		return
	}

	select {
	case st.ingestChan <- frame:
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
	st.writeMu.Lock()
	defer st.writeMu.Unlock()

	if st.sentHeaders {
		return errors.New("already sent headers")
	}
	if md != nil {
		st.headers = metadata.Join(st.headers, md)
	}
	if send {
		return st.sendHeadersLocked()
	}
	return nil
}

func (st *tunnelServerStream) sendHeadersLocked() error {
	err := st.stream.Send(&ServerToClient{
		StreamId: st.streamID,
		Frame: &ServerToClient_Header{
			Header: toProto(st.headers),
		},
	})
	st.sentHeaders = true
	st.headers = nil
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
	st.writeMu.Lock()
	defer st.writeMu.Unlock()

	if st.closed {
		return errors.New("already finished")
	}
	st.trailers = metadata.Join(st.trailers, md)
	return nil
}

func (st *tunnelServerStream) Context() context.Context {
	return st.ctx
}

func (st *tunnelServerStream) SendMsg(m interface{}) error {
	st.writeMu.Lock()
	defer st.writeMu.Unlock()

	if !st.sentHeaders {
		_ = st.sendHeadersLocked()
	}

	if !st.isServerStream && st.numSent == 1 {
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
		if err := st.ctx.Err(); err != nil {
			return err
		}

		chunk := b
		if len(b) > maxChunkSize {
			chunk = b[:maxChunkSize]
		}

		if i == 0 {
			err = st.stream.Send(&ServerToClient{
				StreamId: st.streamID,
				Frame: &ServerToClient_Message{
					Message: &EncodedMessage{
						Size: int32(len(b)),
						Data: chunk,
					},
				},
			})
		} else {
			err = st.stream.Send(&ServerToClient{
				StreamId: st.streamID,
				Frame: &ServerToClient_MessageData{
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
	// TODO: support alternate codecs, compressors, etc
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

		case in, ok := <-st.readChan:
			if !ok {
				// don't need lock to read st.halfClosed; observing
				// input channel close provides safe visibility
				return nil, true, st.halfClosed
			}

			switch in := in.(type) {
			case *ClientToServer_Message:
				if msgLen != -1 {
					return nil, false, status.Errorf(codes.InvalidArgument, "received redundant request message envelope")
				}
				msgLen = int(in.Message.Size)
				b = in.Message.Data
				if len(b) > msgLen {
					return nil, false, status.Errorf(codes.InvalidArgument, "received more data than indicated by request message envelope")
				}
				if len(b) == msgLen {
					return b, true, nil
				}

			case *ClientToServer_MessageData:
				if msgLen == -1 {
					return nil, false, status.Errorf(codes.InvalidArgument, "never received envelope for request message")
				}
				b = append(b, in.MessageData...)
				if len(b) > msgLen {
					return nil, false, status.Errorf(codes.InvalidArgument, "received more data than indicated by request message envelope")
				}
				if len(b) == msgLen {
					return b, true, nil
				}

			default:
				return nil, false, status.Errorf(codes.InvalidArgument, "unrecognized frame type: %T", in)
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
	st.svr.removeStream(st.streamID)

	st.halfClose(err)

	st.writeMu.Lock()
	defer st.writeMu.Unlock()

	if st.closed {
		return
	}

	if !st.sentHeaders {
		_ = st.sendHeadersLocked()
	}

	stat, _ := status.FromError(err)
	_ = st.stream.Send(&ServerToClient{
		StreamId: st.streamID,
		Frame: &ServerToClient_CloseStream_{
			CloseStream: &ServerToClient_CloseStream{
				Status:  stat.Proto(),
				Trailer: toProto(st.trailers),
			},
		},
	})

	st.closed = true
	st.trailers = nil
}

func (st *tunnelServerStream) halfClose(err error) {
	st.ingestMu.Lock()
	defer st.ingestMu.Unlock()

	if st.halfClosed != nil {
		// already closed
		return
	}

	if err == nil {
		err = io.EOF
	}
	st.halfClosed = err
	close(st.ingestChan)
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
