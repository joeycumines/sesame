package netconn

import (
	"context"
	"net"
	"testing"

	sesamepb "github.com/joeycumines/sesame/sesamepb"
)

// fakeUpgradeStream records the TlsUpgraded response handleUpgradeTLS
// emits; any other use panics via the nil embedded interface.
type fakeUpgradeStream struct {
	sesamepb.RemoteControl_NetConnServer

	sent []*sesamepb.NetConnResponse
}

func (f *fakeUpgradeStream) Send(resp *sesamepb.NetConnResponse) error {
	f.sent = append(f.sent, resp)
	return nil
}

// fixedUpgrader is a TLSProvider that hands back a predetermined conn.
type fixedUpgrader struct {
	conn net.Conn
	seen net.Conn
}

func (p *fixedUpgrader) Handshake(_ context.Context, rawConn net.Conn, _ *sesamepb.TlsOptions) (net.Conn, *sesamepb.TlsHandshakeResult, error) {
	p.seen = rawConn
	return p.conn, &sesamepb.TlsHandshakeResult{}, nil
}

// TestHandleUpgradeTLS_SwapBeforeResumeSignal pins the ordering of the
// upgrade handoff: the upgraded conn must be installed as the active
// conn BEFORE the reader is signalled to resume. Pre-fix, the reader
// performed the swap after consuming the resume signal, so a control
// action racing that window acted on the stale pre-upgrade conn (e.g.
// half_close FINning the wrong connection while the tunnel lives on
// the upgraded one). This test fails against the pre-fix ordering
// because no reader goroutine exists to perform the swap: the upgrade
// handler itself must have done it.
func TestHandleUpgradeTLS_SwapBeforeResumeSignal(t *testing.T) {
	rawConn, rawPeer := net.Pipe()
	defer rawConn.Close()
	defer rawPeer.Close()
	upgradedConn, upgradedPeer := net.Pipe()
	defer upgradedConn.Close()
	defer upgradedPeer.Close()

	provider := &fixedUpgrader{conn: upgradedConn}
	stream := &fakeUpgradeStream{}
	s := &serverControlState{
		stream:       stream,
		tlsProvider:  provider,
		activeConn:   rawConn,
		pauseReq:     make(chan struct{}, 1),
		pausedAck:    make(chan struct{}, 1),
		resumeNotify: make(chan struct{}, 1),
	}

	// Ack the pause handshake; no reader goroutine exists.
	go func() {
		<-s.pauseReq
		s.pausedAck <- struct{}{}
	}()

	// request_id 0 exercises the id-less legacy path: the server must
	// echo it verbatim.
	if err := s.handleUpgradeTLS(context.Background(), &sesamepb.TlsOptions{Sni: "upgrade.internal"}, 0); err != nil {
		t.Fatalf("handleUpgradeTLS failed: %v", err)
	}
	if provider.seen != rawConn {
		t.Fatal("the handshake must run on the pre-upgrade active conn")
	}

	// The TlsUpgraded response carries the request id verbatim.
	if len(stream.sent) != 1 {
		t.Fatalf("expected exactly one response, got %d", len(stream.sent))
	}
	upgraded := stream.sent[0].GetControl().GetTlsUpgraded()
	if upgraded == nil {
		t.Fatalf("expected a TlsUpgraded event, got %v", stream.sent[0].GetData())
	}
	if upgraded.GetRequestId() != 0 {
		t.Fatalf("legacy request_id 0 must be echoed verbatim, got %d", upgraded.GetRequestId())
	}

	s.connMu.RLock()
	active := s.activeConn
	s.connMu.RUnlock()
	if active != upgradedConn {
		t.Fatal("the active conn must be swapped to the upgraded conn before the resume signal is consumed")
	}
	if len(s.resumeNotify) != 1 {
		t.Fatal("the resume signal must still be pending - the swap has to be linearized ahead of it, with no reader involved")
	}
}
