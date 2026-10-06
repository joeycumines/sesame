package netconn

import (
	"context"
	cryptotls "crypto/tls"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/sesame/type/netaddr"
	sesameproxy "github.com/joeycumines/sesame/type/proxy"
	sesametls "github.com/joeycumines/sesame/type/tls"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestProtoToTLSVersion(t *testing.T) {
	tests := []struct {
		proto sesametls.TLSVersion
		want  uint16
	}{
		{sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED, 0},
		{sesametls.TLSVersion_TLS_1_0, cryptotls.VersionTLS10},
		{sesametls.TLSVersion_TLS_1_1, cryptotls.VersionTLS11},
		{sesametls.TLSVersion_TLS_1_2, cryptotls.VersionTLS12},
		{sesametls.TLSVersion_TLS_1_3, cryptotls.VersionTLS13},
	}
	for _, tt := range tests {
		if got := ProtoToTLSVersion(tt.proto); got != tt.want {
			t.Errorf("ProtoToTLSVersion(%v) = %v, want %v", tt.proto, got, tt.want)
		}
		if tt.want != 0 {
			if gotProto := TLSVersionToProto(tt.want); gotProto != tt.proto {
				t.Errorf("TLSVersionToProto(%v) = %v, want %v", tt.want, gotProto, tt.proto)
			}
		}
	}
}

func TestBuildTLSConfig_ALPN_Rule(t *testing.T) {
	// Standing Directive: If alpn_protocols is empty, ALPN extension MUST NOT be sent.
	opts := &sesametls.TLSOptions{
		ServerName: "example.com",
	}
	cfg, err := BuildTLSConfig(opts, "fallback.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ServerName != "example.com" {
		t.Errorf("got ServerName %q, want example.com", cfg.ServerName)
	}
	if cfg.NextProtos != nil {
		t.Errorf("expected NextProtos to be nil when empty, got %v", cfg.NextProtos)
	}

	// When ALPN protocols are provided
	optsWithALPN := &sesametls.TLSOptions{
		ServerName:    "example.com:443",
		AlpnProtocols: []string{"h2", "http/1.1"},
	}
	cfgALPN, err := BuildTLSConfig(optsWithALPN, "fallback.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfgALPN.ServerName != "example.com" {
		t.Errorf("port should be stripped from ServerName: got %q, want example.com", cfgALPN.ServerName)
	}
	if len(cfgALPN.NextProtos) != 2 || cfgALPN.NextProtos[0] != "h2" || cfgALPN.NextProtos[1] != "http/1.1" {
		t.Errorf("unexpected NextProtos: %v", cfgALPN.NextProtos)
	}
}

func TestExecuteTLSHandshake_UnsupportedPreset(t *testing.T) {
	// Server MUST return FAILED_PRECONDITION if preset cannot be satisfied
	opts := &sesametls.TLSOptions{
		FingerprintPreset: sesametls.FingerprintPreset_CHROME_131,
	}

	rawConn, peerConn := net.Pipe()
	defer rawConn.Close()
	defer peerConn.Close()

	_, _, err := ExecuteTLSHandshake(context.Background(), rawConn, opts, "example.com", nil)
	if err == nil {
		t.Fatal("expected error for unsupported preset without custom TLSProvider, got nil")
	}
}

func TestExecuteProxyHops_HTTPConnect(t *testing.T) {
	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	targetAddr := "target.internal:443"
	proxyAddr := "proxy.internal:8080"

	// Mock HTTP proxy server goroutine reading from serverPipe
	go func() {
		buf := make([]byte, 1024)
		n, err := serverPipe.Read(buf)
		if err != nil {
			return
		}
		req := string(buf[:n])

		if !contains(req, "CONNECT "+targetAddr) {
			_, _ = serverPipe.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			return
		}

		_, _ = serverPipe.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	}()

	mockDialer := &pipeDialer{conn: clientPipe}
	proxyOpts := &sesameproxy.ProxyOptions{
		Hops: []*sesameproxy.ProxyHop{
			{
				Type: sesameproxy.ProxyHop_HTTP_CONNECT,
				Address: &netaddr.NetAddr{
					Network: "tcp",
					Address: proxyAddr,
				},
			},
		},
	}

	conn, res, err := ExecuteProxyHops(context.Background(), mockDialer, proxyOpts, "tcp", targetAddr)
	if err != nil {
		t.Fatalf("ExecuteProxyHops failed: %v", err)
	}
	defer conn.Close()

	if res == nil || len(res.GetTraversedHops()) != 1 {
		t.Fatalf("unexpected proxy result: %v", res)
	}
	if res.GetTraversedHops()[0].GetAddress() != proxyAddr {
		t.Errorf("traversed hop got %s, want %s", res.GetTraversedHops()[0].GetAddress(), proxyAddr)
	}
}

func TestHttpConnectHandshake_RejectsFramed200Body(t *testing.T) {
	// A 200 response to CONNECT must never carry a body. Both chunked
	// framing and a declared Content-Length mean the proxy is not
	// tunnelling, and the bytes must not enter the tunnel as payload.
	for _, tc := range []struct {
		name    string
		headers string
	}{
		{name: "chunked", headers: "Transfer-Encoding: chunked"},
		{name: "content-length", headers: "Content-Length: 5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientPipe, serverPipe := net.Pipe()
			defer clientPipe.Close()
			defer serverPipe.Close()

			go func() {
				defer serverPipe.Close()
				buf := make([]byte, 4096)
				if _, err := serverPipe.Read(buf); err != nil {
					return
				}
				_, _ = serverPipe.Write([]byte("HTTP/1.1 200 Connection Established\r\n" + tc.headers + "\r\n\r\nhello"))
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := httpConnectHandshake(ctx, clientPipe, &sesameproxy.ProxyHop{
				Type: sesameproxy.ProxyHop_HTTP_CONNECT,
				Address: &netaddr.NetAddr{
					Network: "tcp",
					Address: "proxy.internal:8080",
				},
			}, "target.internal:443")
			if err == nil {
				t.Fatal("expected error for 200 with framed body, got nil")
			}
		})
	}
}

type pipeDialer struct {
	conn net.Conn
}

func (p *pipeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return p.conn, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// deadlineRecordingConn wraps a net.Conn recording SetDeadline calls.
type deadlineRecordingConn struct {
	net.Conn

	mu        sync.Mutex
	deadlines []time.Time
}

func (c *deadlineRecordingConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	c.deadlines = append(c.deadlines, t)
	c.mu.Unlock()
	return c.Conn.SetDeadline(t)
}

func TestSocks5Handshake_ArmsFallbackDeadline(t *testing.T) {
	// A SOCKS5 proxy that accepts and never responds. The handshake must
	// arm a bounded deadline even when the caller ctx has none, matching
	// the HTTP CONNECT fallback. The test asserts the deadline was ARMED
	// and later restored - it does not wait for it to fire.
	blackhole, blackholeConn := net.Pipe()
	defer blackhole.Close()
	defer blackholeConn.Close()

	recording := &deadlineRecordingConn{Conn: blackholeConn}

	// Never service the SOCKS5 greeting; reads on `blackhole` are simply
	// dropped so the dialer's write does not block the test.
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := blackhole.Read(buf); err != nil {
				return
			}
		}
	}()

	// The fallback is 30s; use a short deadline only to bound the test,
	// via a ctx whose deadline is what setProxyHandshakeDeadline will
	// arm... instead, call with a deadline-less ctx to exercise the
	// fallback path. The handshake will block until the armed 30s
	// deadline fires - too long for a test. So drive it through
	// socks5Handshake directly and close the conn concurrently to
	// unblock.
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = recording.Close()
	}()

	_, err := socks5Handshake(context.Background(), recording, &sesameproxy.ProxyHop{
		Type:     sesameproxy.ProxyHop_SOCKS5,
		Username: "user",
		Password: "pass",
	}, "target.internal:443")
	if err == nil {
		t.Fatal("expected error from closed blackhole SOCKS5 handshake")
	}

	recording.mu.Lock()
	deadlines := append([]time.Time(nil), recording.deadlines...)
	recording.mu.Unlock()

	if len(deadlines) == 0 {
		t.Fatal("no deadline was armed for the SOCKS5 handshake")
	}
	armed := deadlines[0]
	if armed.IsZero() {
		t.Fatal("armed SOCKS5 deadline is zero")
	}
	// The fallback deadline must be in the future (roughly 30s out).
	if !time.Now().Before(armed) {
		t.Fatalf("armed deadline %v is not in the future", armed)
	}
	// The restore must have reset the deadline back to zero.
	restored := deadlines[len(deadlines)-1]
	if !restored.IsZero() {
		t.Fatalf("deadline was not restored after the handshake; last SetDeadline: %v", restored)
	}
}

func TestBuildTLSConfig_RejectsOutOfRangeCipherSuite(t *testing.T) {
	_, err := BuildTLSConfig(&sesametls.TLSOptions{
		ServerName:   "example.com",
		CipherSuites: []uint32{0x1301, 1 << 20},
	}, "")
	if err == nil {
		t.Fatal("expected InvalidArgument for out-of-range cipher suite")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}

	// In-range values must still build.
	cfg, err := BuildTLSConfig(&sesametls.TLSOptions{
		ServerName:   "example.com",
		CipherSuites: []uint32{0x1301},
	}, "")
	if err != nil {
		t.Fatalf("BuildTLSConfig failed for valid suite: %v", err)
	}
	if len(cfg.CipherSuites) != 1 || cfg.CipherSuites[0] != 0x1301 {
		t.Fatalf("unexpected cipher suites: %v", cfg.CipherSuites)
	}
}
