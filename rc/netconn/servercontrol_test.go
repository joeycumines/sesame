package netconn

import (
	"context"
	"net"
	"testing"

	"github.com/joeycumines/sesame/rc"
	sesametls "github.com/joeycumines/sesame/rc/tls"
)

// fakeUpgradeStream lets handleUpgradeTLS emit its TlsUpgraded response
// without a live gRPC stream; any other use panics via the nil
// embedded interface.
type fakeUpgradeStream struct {
	rc.RemoteControl_NetConnServer
}

func (f *fakeUpgradeStream) Send(*rc.NetConnResponse) error { return nil }

// fixedUpgrader is a TLSProvider that hands back a predetermined conn.
type fixedUpgrader struct {
	conn net.Conn
	seen net.Conn
}

func (p *fixedUpgrader) Handshake(_ context.Context, rawConn net.Conn, _ *sesametls.TLSOptions) (net.Conn, *sesametls.TLSHandshakeResult, error) {
	p.seen = rawConn
	return p.conn, &sesametls.TLSHandshakeResult{}, nil
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
	s := &serverControlState{
		stream:       &fakeUpgradeStream{},
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

	if err := s.handleUpgradeTLS(context.Background(), &sesametls.TLSOptions{ServerName: "upgrade.internal"}); err != nil {
		t.Fatalf("handleUpgradeTLS failed: %v", err)
	}
	if provider.seen != rawConn {
		t.Fatal("the handshake must run on the pre-upgrade active conn")
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
