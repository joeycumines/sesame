package netconn

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/joeycumines/sesame/ionet"
	"github.com/joeycumines/sesame/rc"
)

// mockNetConnStream scripts clientControlConn.readLoop interactions:
// Recv pops scripted responses until exhausted (then recvErr), and Send
// returns sendErr.
type mockNetConnStream struct {
	rc.RemoteControl_NetConnClient

	mu      sync.Mutex
	recv    []*rc.NetConnResponse
	recvErr error
	sendErr error
}

func (m *mockNetConnStream) Recv() (*rc.NetConnResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.recv) == 0 {
		return nil, m.recvErr
	}
	msg := m.recv[0]
	m.recv = m.recv[1:]
	return msg, nil
}

func (m *mockNetConnStream) Send(*rc.NetConnRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sendErr
}

// newReadLoopTestConn wires a clientControlConn around a scripted mock
// stream for direct readLoop testing.
func newReadLoopTestConn(t *testing.T, recv []*rc.NetConnResponse, recvErr, sendErr error) *clientControlConn {
	t.Helper()
	local, remote := ionet.Pipe()
	_, localWriter := local.SendPipe()
	t.Cleanup(func() {
		_ = remote.Close()
		_ = local.Close()
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &clientControlConn{
		stream:       &mockNetConnStream{recv: recv, recvErr: recvErr, sendErr: sendErr},
		ctx:          ctx,
		cancel:       cancel,
		demuxLocalW:  localWriter,
		demuxRemote:  remote,
		inboundFC:    NewFlowController(1024),
		pendingPings: make(map[int64]chan int64),
	}
}

// TestClientControlConn_ReadLoop_ToleratesWindowUpdateEOF pins the fix
// for the TestClientServer_InStream_HalfClose flake: a window-update
// send racing the peer's RPC completion fails with io.EOF (the
// transport closes the request half on the graceful terminal). The
// readLoop must treat that as benign and keep draining - closing the
// pipe on it surfaced a clean EOF to the app and silently discarded
// response messages still buffered in transport, which lost the
// target's post-FIN bytes in roughly 1% of runs under -race.
func TestClientControlConn_ReadLoop_ToleratesWindowUpdateEOF(t *testing.T) {
	cc := newReadLoopTestConn(t, []*rc.NetConnResponse{
		{Data: &rc.NetConnResponse_Bytes{Bytes: []byte("PING")}},
		{Data: &rc.NetConnResponse_Bytes{Bytes: []byte("FINAL")}},
	}, io.EOF, io.EOF)
	go cc.readLoop(cc.ctx)

	// Both buffered messages must reach the app before the clean EOF;
	// before the fix, the first window update's io.EOF closed the pipe
	// and "FINAL" was discarded.
	ping := make([]byte, 4)
	if _, err := io.ReadFull(cc, ping); err != nil || string(ping) != "PING" {
		t.Fatalf("first read = %q, %v; want PING, <nil>", ping, err)
	}
	final := make([]byte, 5)
	if _, err := io.ReadFull(cc, final); err != nil || string(final) != "FINAL" {
		t.Fatalf("second read = %q, %v; want FINAL, <nil> (a benign window-update io.EOF must not discard buffered responses)", final, err)
	}
	if _, err := cc.Read(ping); err != io.EOF {
		t.Fatalf("terminal read err = %v, want io.EOF", err)
	}
}

// TestClientControlConn_ReadLoop_WindowUpdateRealErrorCloses confirms
// only the benign end-of-stream spelling is tolerated: a genuine send
// failure still surfaces to the app.
func TestClientControlConn_ReadLoop_WindowUpdateRealErrorCloses(t *testing.T) {
	sendErr := errors.New("sesame/rc/netconn: connection gone")
	cc := newReadLoopTestConn(t, []*rc.NetConnResponse{
		{Data: &rc.NetConnResponse_Bytes{Bytes: []byte("PING")}},
	}, io.EOF, sendErr)
	go cc.readLoop(cc.ctx)

	ping := make([]byte, 4)
	if _, err := io.ReadFull(cc, ping); err != nil || string(ping) != "PING" {
		t.Fatalf("first read = %q, %v; want PING, <nil>", ping, err)
	}
	buf := make([]byte, 8)
	if _, err := cc.Read(buf); !errors.Is(err, sendErr) {
		t.Fatalf("terminal read err = %v, want %v", err, sendErr)
	}
}
