package grpc

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/type/grpctunnel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Test_readMsgLocked_contextCancellation verifies that readMsgLocked unblocks
// when the stream's context is cancelled, even when the buffer is empty.
// This is a regression test for a deadlock where readMsgLocked blocked
// forever on an empty buffer channel with no context check.
func Test_readMsgLocked_contextCancellation(t *testing.T) {
	chCtx, chCancel := context.WithCancel(context.Background())
	defer chCancel()
	ctx, cancel := context.WithCancel(context.Background())
	ch := &Channel{ctx: chCtx, streams: make(map[uint64]*tunnelClientStream)}
	st := &tunnelClientStream{
		ctx:            ctx,
		cncl:           cancel,
		ch:             ch,
		buffer:         make(chan *grpctunnel.ServerToClient, windowMaxBuffer),
		headerSignal:   make(chan struct{}),
		doneSignal:     make(chan struct{}),
		isServerStream: true,
	}

	done := make(chan struct{})
	var readErr error
	go func() {
		defer close(done)
		_, _, readErr = st.readMsgLocked()
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
		if readErr == nil {
			t.Error("expected error from readMsgLocked after context cancellation")
		} else if s, ok := status.FromError(readErr); !ok || s.Code() != codes.Canceled {
			t.Errorf("expected gRPC Canceled status, got %v", readErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("readMsgLocked did not return after context cancellation — DEADLOCK")
	}
}

// Test_readMsgLocked_channelContextCancellation verifies that readMsgLocked
// unblocks when the channel's context is cancelled, even if the stream's
// own context is still active. This is the primary deadlock scenario: the
// gRPC call is cancelled (closing the tunnel), which cancels ch.ctx
// immediately, but st.ctx is only cancelled later via the async chain
// (recvLoop exit → Channel.close → stream.cncl).
func Test_readMsgLocked_channelContextCancellation(t *testing.T) {
	chCtx, chCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := &Channel{ctx: chCtx, streams: make(map[uint64]*tunnelClientStream)}
	st := &tunnelClientStream{
		ctx:            ctx,
		cncl:           cancel,
		ch:             ch,
		buffer:         make(chan *grpctunnel.ServerToClient, windowMaxBuffer),
		headerSignal:   make(chan struct{}),
		doneSignal:     make(chan struct{}),
		isServerStream: true,
	}

	done := make(chan struct{})
	var readErr error
	go func() {
		defer close(done)
		_, _, readErr = st.readMsgLocked()
	}()

	time.Sleep(10 * time.Millisecond)
	// Cancel the CHANNEL context (not the stream context).
	chCancel()

	select {
	case <-done:
		if readErr == nil {
			t.Error("expected error from readMsgLocked after channel context cancellation")
		} else if s, ok := status.FromError(readErr); !ok || s.Code() != codes.Canceled {
			t.Errorf("expected gRPC Canceled status, got %v", readErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("readMsgLocked did not return after channel context cancellation — DEADLOCK")
	}
}

// Test_readMsgLocked_bufferCloseTakesPriority verifies that when the buffer
// is closed (normal stream finish), readMsgLocked returns st.done rather
// than a context error — preserving proper gRPC status codes.
func Test_readMsgLocked_bufferCloseTakesPriority(t *testing.T) {
	chCtx, chCancel := context.WithCancel(context.Background())
	defer chCancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := &Channel{ctx: chCtx, streams: make(map[uint64]*tunnelClientStream)}
	st := &tunnelClientStream{
		ctx:            ctx,
		cncl:           cancel,
		ch:             ch,
		buffer:         make(chan *grpctunnel.ServerToClient, windowMaxBuffer),
		headerSignal:   make(chan struct{}),
		doneSignal:     make(chan struct{}),
		isServerStream: true,
		done:           io.EOF,
	}
	close(st.buffer)
	close(st.doneSignal)

	_, ok, err := st.readMsgLocked()
	if !ok {
		t.Error("expected ok=true for stream-originating EOF")
	}
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

// Test_acceptServerFrame_closedBufferRecover verifies that acceptServerFrame
// does not process a message after finishStream has already completed.
// finishStream closes the buffer and sets done; acceptServerFrame's
// ingestMu-guarded check catches this.
func Test_acceptServerFrame_closedBufferRecover(t *testing.T) {
	chCtx, chCancel := context.WithCancel(context.Background())
	defer chCancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := &Channel{
		ctx:     chCtx,
		streams: make(map[uint64]*tunnelClientStream),
	}
	st := &tunnelClientStream{
		ctx:          ctx,
		cncl:         cancel,
		ch:           ch,
		streamID:     1,
		buffer:       make(chan *grpctunnel.ServerToClient, windowMaxBuffer),
		headerSignal: make(chan struct{}),
		doneSignal:   make(chan struct{}),
	}

	// Simulate finishStream having already completed.
	st.done = io.EOF
	close(st.buffer)
	close(st.doneSignal)

	// acceptServerFrame must see done != nil under ingestMu and return.
	// Must not panic.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("acceptServerFrame panicked: %v", r)
			}
		}()
		st.acceptServerFrame(&grpctunnel.ServerToClient{
			StreamId: 1,
			Frame: &grpctunnel.ServerToClient_MessageData{
				MessageData: []byte("test"),
			},
		})
	}()
}

// Test_acceptServerFrame_fullBuffer_contextCancel verifies that
// acceptServerFrame unblocks when the buffer is full and the context is
// cancelled. This prevents the recvLoop from being blocked indefinitely.
func Test_acceptServerFrame_fullBuffer_contextCancel(t *testing.T) {
	chCtx, chCancel := context.WithCancel(context.Background())
	defer chCancel()
	ctx, cancel := context.WithCancel(context.Background())
	ch := &Channel{
		ctx:          chCtx,
		streams:      make(map[uint64]*tunnelClientStream),
		updateWindow: func(msg *grpctunnel.ServerToClient) {},
	}
	st := &tunnelClientStream{
		ctx:          ctx,
		cncl:         cancel,
		ch:           ch,
		streamID:     1,
		buffer:       make(chan *grpctunnel.ServerToClient, 1), // tiny buffer
		headerSignal: make(chan struct{}),
		doneSignal:   make(chan struct{}),
	}

	// Fill the buffer.
	st.buffer <- &grpctunnel.ServerToClient{
		StreamId: 1,
		Frame:    &grpctunnel.ServerToClient_MessageData{MessageData: []byte("fill")},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		st.acceptServerFrame(&grpctunnel.ServerToClient{
			StreamId: 1,
			Frame:    &grpctunnel.ServerToClient_MessageData{MessageData: []byte("overflow")},
		})
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// acceptServerFrame returned after context cancellation — correct.
	case <-time.After(5 * time.Second):
		t.Fatal("acceptServerFrame did not return after context cancellation — DEADLOCK")
	}
}

// Test_acceptServerFrame_finishStreamBreaksDeadlock verifies that
// finishStream can proceed when acceptServerFrame is blocked on a full
// buffer. Channel.close() cancels the stream context externally, which
// makes acceptServerFrame's <-st.ctx.Done() fire and release ingestMu,
// allowing finishStream to acquire it.
func Test_acceptServerFrame_finishStreamBreaksDeadlock(t *testing.T) {
	chCtx, chCancel := context.WithCancel(context.Background())
	defer chCancel()
	ctx, cancel := context.WithCancel(context.Background())
	ch := &Channel{
		ctx:          chCtx,
		streams:      make(map[uint64]*tunnelClientStream),
		updateWindow: func(msg *grpctunnel.ServerToClient) {},
	}
	st := &tunnelClientStream{
		ctx:          ctx,
		cncl:         cancel,
		ch:           ch,
		streamID:     1,
		buffer:       make(chan *grpctunnel.ServerToClient, 1),
		headerSignal: make(chan struct{}),
		doneSignal:   make(chan struct{}),
	}

	// Fill the buffer to force blocking in acceptServerFrame.
	st.buffer <- &grpctunnel.ServerToClient{
		StreamId: 1,
		Frame:    &grpctunnel.ServerToClient_MessageData{MessageData: []byte("fill")},
	}

	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		st.acceptServerFrame(&grpctunnel.ServerToClient{
			StreamId: 1,
			Frame:    &grpctunnel.ServerToClient_MessageData{MessageData: []byte("overflow")},
		})
	}()

	time.Sleep(10 * time.Millisecond)

	// Simulate Channel.close(): cancel context externally first,
	// then call finishStream. The external cancel unblocks
	// acceptServerFrame so finishStream can acquire ingestMu.
	cancel()

	finishDone := make(chan struct{})
	go func() {
		defer close(finishDone)
		st.finishStream(io.EOF, nil)
	}()

	select {
	case <-finishDone:
		// finishStream completed — external context cancellation broke the deadlock.
	case <-time.After(5 * time.Second):
		t.Fatal("finishStream could not proceed while acceptServerFrame was blocked — DEADLOCK")
	}

	select {
	case <-acceptDone:
		// acceptServerFrame returned after context was cancelled.
	case <-time.After(5 * time.Second):
		t.Fatal("acceptServerFrame did not return after finishStream")
	}
}

// Test_channelClose_pendingRecv verifies that closing a Channel unblocks
// any pending RecvMsg calls, preventing a deadlock during teardown.
func Test_channelClose_pendingRecv(t *testing.T) {
	ts := TunnelServer{}
	// Register a streaming handler that never responds, holding the stream open.
	holdOpenImpl := struct{}{} // non-nil impl
	ts.RegisterService(&grpc.ServiceDesc{
		ServiceName: "test.HoldOpen",
		HandlerType: (*interface{})(nil),
		Streams: []grpc.StreamDesc{{
			StreamName:    "Stream",
			ServerStreams: true,
			ClientStreams: true,
			Handler: func(srv interface{}, stream grpc.ServerStream) error {
				// Block until the stream context is done (simulating a slow server).
				<-stream.Context().Done()
				return stream.Context().Err()
			},
		}},
		Metadata: "test_hold_open.proto",
	}, &holdOpenImpl)

	cc := testutil.NewBufconnClient(0, func(_ *bufconn.Listener, srv *grpc.Server) {
		grpctunnel.RegisterTunnelServiceServer(srv, &ts)
	})
	defer cc.Close()

	tunnel, err := grpctunnel.NewTunnelServiceClient(cc).OpenTunnel(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	ch, err := NewChannel(OptChannel.ClientStream(tunnel))
	if err != nil {
		t.Fatal(err)
	}

	stream, err := ch.NewStream(
		context.Background(),
		&grpc.StreamDesc{ServerStreams: true, ClientStreams: true},
		"/test.HoldOpen/Stream",
	)
	if err != nil {
		t.Fatal(err)
	}

	recvDone := make(chan error, 1)
	go func() {
		recvDone <- stream.RecvMsg(new(emptypb.Empty))
	}()

	// Give RecvMsg time to block in readMsgLocked.
	time.Sleep(50 * time.Millisecond)

	// Closing the channel cancels all stream contexts.
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- ch.Close()
	}()

	select {
	case <-closeDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Channel.Close() did not complete — DEADLOCK")
	}

	select {
	case err := <-recvDone:
		if err == nil {
			t.Error("expected error from RecvMsg after Close")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RecvMsg did not return after Channel.Close() — DEADLOCK")
	}
}

// Test_channelClose_pendingRecv_withHeader is like Test_channelClose_pendingRecv
// but the server sends a header before going silent, exercising the path
// where the header has been received but the buffer is empty.
func Test_channelClose_pendingRecv_withHeader(t *testing.T) {
	ts := TunnelServer{}
	holdOpenImpl2 := struct{}{}
	ts.RegisterService(&grpc.ServiceDesc{
		ServiceName: "test.HoldAfterHeader",
		HandlerType: (*interface{})(nil),
		Streams: []grpc.StreamDesc{{
			StreamName:    "Stream",
			ServerStreams: true,
			ClientStreams: true,
			Handler: func(srv interface{}, stream grpc.ServerStream) error {
				// Send header, then block.
				_ = stream.SendHeader(metadata.Pairs("x-test", "ok"))
				<-stream.Context().Done()
				return stream.Context().Err()
			},
		}},
		Metadata: "test_hold_after_header.proto",
	}, &holdOpenImpl2)

	cc := testutil.NewBufconnClient(0, func(_ *bufconn.Listener, srv *grpc.Server) {
		grpctunnel.RegisterTunnelServiceServer(srv, &ts)
	})
	defer cc.Close()

	tunnel, err := grpctunnel.NewTunnelServiceClient(cc).OpenTunnel(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	ch, err := NewChannel(OptChannel.ClientStream(tunnel))
	if err != nil {
		t.Fatal(err)
	}

	stream, err := ch.NewStream(
		context.Background(),
		&grpc.StreamDesc{ServerStreams: true, ClientStreams: true},
		"/test.HoldAfterHeader/Stream",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for header to arrive.
	hdr, err := stream.Header()
	if err != nil {
		t.Fatal(err)
	}
	if got := hdr.Get("x-test"); len(got) == 0 || got[0] != "ok" {
		t.Fatalf("unexpected header: %v", hdr)
	}

	recvDone := make(chan error, 1)
	go func() {
		recvDone <- stream.RecvMsg(new(emptypb.Empty))
	}()

	time.Sleep(50 * time.Millisecond)

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- ch.Close()
	}()

	select {
	case <-closeDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Channel.Close() did not complete — DEADLOCK")
	}

	select {
	case <-recvDone:
	case <-time.After(5 * time.Second):
		t.Fatal("RecvMsg did not return after Channel.Close() — DEADLOCK")
	}
}

// Test_concurrentCancelAndFinish verifies that concurrent calls to cancel
// and finishStream don't deadlock or panic. This exercises the priority
// select in readMsgLocked and the ingestMu coordination.
func Test_concurrentCancelAndFinish(t *testing.T) {
	for i := 0; i < 100; i++ {
		ts := TunnelServer{}
		concCloseImpl := struct{}{}
		ts.RegisterService(&grpc.ServiceDesc{
			ServiceName: "test.ConcClose",
			HandlerType: (*interface{})(nil),
			Streams: []grpc.StreamDesc{{
				StreamName:    "Stream",
				ServerStreams: true,
				ClientStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					<-stream.Context().Done()
					return stream.Context().Err()
				},
			}},
			Metadata: "test_conc_close.proto",
		}, &concCloseImpl)

		cc := testutil.NewBufconnClient(0, func(_ *bufconn.Listener, srv *grpc.Server) {
			grpctunnel.RegisterTunnelServiceServer(srv, &ts)
		})

		tunnel, err := grpctunnel.NewTunnelServiceClient(cc).OpenTunnel(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		ch, err := NewChannel(OptChannel.ClientStream(tunnel))
		if err != nil {
			cc.Close()
			t.Fatal(err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		stream, err := ch.NewStream(
			ctx,
			&grpc.StreamDesc{ServerStreams: true, ClientStreams: true},
			"/test.ConcClose/Stream",
		)
		if err != nil {
			cancel()
			ch.Close()
			cc.Close()
			t.Fatal(err)
		}

		recvDone := make(chan struct{})
		go func() {
			defer close(recvDone)
			_ = stream.RecvMsg(new(emptypb.Empty))
		}()

		time.Sleep(5 * time.Millisecond)

		// Race: cancel context AND close channel concurrently.
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); cancel() }()
		go func() { defer wg.Done(); _ = ch.Close() }()
		wg.Wait()

		select {
		case <-recvDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("iteration %d: RecvMsg did not return — DEADLOCK", i)
		}

		cc.Close()
	}
}
