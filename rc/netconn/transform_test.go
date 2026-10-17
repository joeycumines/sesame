package netconn

import (
	"context"
	cryptotls "crypto/tls"
	"errors"
	sesamepb "github.com/joeycumines/sesame/sesamepb"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	typepb "github.com/joeycumines/sesame/typepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestProtoToTLSVersion(t *testing.T) {
	tests := []struct {
		proto sesamepb.TlsVersion
		want  uint16
	}{
		{sesamepb.TlsVersion_TLS_VERSION_UNSPECIFIED, 0},
		{sesamepb.TlsVersion_TLS_VERSION_TLS_1_0, cryptotls.VersionTLS10},
		{sesamepb.TlsVersion_TLS_VERSION_TLS_1_1, cryptotls.VersionTLS11},
		{sesamepb.TlsVersion_TLS_VERSION_TLS_1_2, cryptotls.VersionTLS12},
		{sesamepb.TlsVersion_TLS_VERSION_TLS_1_3, cryptotls.VersionTLS13},
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
	opts := &sesamepb.TlsOptions{
		Sni: "example.com",
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
	optsWithALPN := &sesamepb.TlsOptions{
		Sni:           "example.com:443",
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

func TestExecuteTLSHandshake_FailsClosedOnUnhonorableSpec(t *testing.T) {
	// The builtin engine MUST fail closed (FAILED_PRECONDITION) when a
	// requested ClientHello dimension cannot be honored exactly.
	opts := &sesamepb.TlsOptions{
		Sni: "example.com",
		ClientHello: &sesamepb.ClientHelloSpec{
			SignatureAlgorithms: []int32{0x0403},
		},
	}

	rawConn, peerConn := net.Pipe()
	defer rawConn.Close()
	defer peerConn.Close()

	_, _, err := ExecuteTLSHandshake(context.Background(), rawConn, opts, "example.com", nil)
	if err == nil {
		t.Fatal("expected error for un-honorable client_hello dimension without custom TLSProvider, got nil")
	}
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", err)
	}

	// Range violations are INVALID_ARGUMENT and equally rejected.
	_, _, err = ExecuteTLSHandshake(context.Background(), rawConn, &sesamepb.TlsOptions{
		Sni: "example.com",
		ClientHello: &sesamepb.ClientHelloSpec{
			CipherSuites: []int32{1 << 20},
		},
	}, "example.com", nil)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for out-of-range cipher suite, got %v", err)
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
	proxyOpts := &sesamepb.ProxyOptions{
		Hops: []*sesamepb.ProxyHop{
			{
				Type: sesamepb.ProxyHop_TYPE_HTTP_CONNECT,
				Address: &typepb.NetAddr{
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
			_, err := httpConnectHandshake(ctx, clientPipe, &sesamepb.ProxyHop{
				Type: sesamepb.ProxyHop_TYPE_HTTP_CONNECT,
				Address: &typepb.NetAddr{
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
	// via a ctx whose deadline is what setHandshakeDeadline will
	// arm... instead, call with a deadline-less ctx to exercise the
	// fallback path. The handshake will block until the armed 30s
	// deadline fires - too long for a test. So drive it through
	// socks5Handshake directly and close the conn concurrently to
	// unblock.
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = recording.Close()
	}()

	_, err := socks5Handshake(context.Background(), recording, &sesamepb.ProxyHop{
		Type:     sesamepb.ProxyHop_TYPE_SOCKS5,
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

func TestExecuteTLSHandshake_ArmsAndRestoresHandshakeDeadline(t *testing.T) {
	// A stalled peer must not hang the TLS handshake: with no caller
	// deadline the same fallback the proxy handshakes use is armed, and
	// restored afterwards so steady-state I/O stays unbounded. Mirrors
	// TestSocks5Handshake_ArmsFallbackDeadline.
	blackhole, blackholeConn := net.Pipe()
	defer blackhole.Close()
	defer blackholeConn.Close()

	recording := &deadlineRecordingConn{Conn: blackholeConn}

	// Never service the ClientHello; drain so the write does not block.
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := blackhole.Read(buf); err != nil {
				return
			}
		}
	}()

	// The 30s fallback is too long to wait out in a test; close the
	// conn concurrently to unblock the handshake.
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = recording.Close()
	}()

	_, _, err := ExecuteTLSHandshake(context.Background(), recording, &sesamepb.TlsOptions{
		Sni: "example.com",
	}, "example.com", nil)
	if err == nil {
		t.Fatal("expected error from closed blackhole TLS handshake")
	}

	recording.mu.Lock()
	deadlines := append([]time.Time(nil), recording.deadlines...)
	recording.mu.Unlock()

	if len(deadlines) == 0 {
		t.Fatal("no deadline was armed for the TLS handshake")
	}
	armed := deadlines[0]
	if armed.IsZero() {
		t.Fatal("armed TLS handshake deadline is zero")
	}
	if !time.Now().Before(armed) {
		t.Fatalf("armed deadline %v is not in the future", armed)
	}
	restored := deadlines[len(deadlines)-1]
	if !restored.IsZero() {
		t.Fatalf("deadline was not restored after the TLS handshake; last SetDeadline: %v", restored)
	}
}

func TestExecuteTLSHandshake_HonorsContextDeadline(t *testing.T) {
	// A caller deadline bounds the handshake (this is what the
	// dial-time path derives from the request's dial timeout) instead
	// of leaving the stalled peer to the 30s fallback.
	blackhole, blackholeConn := net.Pipe()
	defer blackhole.Close()
	defer blackholeConn.Close()

	// Never service the ClientHello; drain so the write does not block.
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := blackhole.Read(buf); err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := ExecuteTLSHandshake(ctx, blackholeConn, &sesamepb.TlsOptions{
		Sni: "example.com",
	}, "example.com", nil)
	if err == nil {
		t.Fatal("expected the stalled handshake to fail")
	}
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("handshake outlived the context deadline: %v", elapsed)
	}
}

func TestExecuteTLSHandshake_HonorsContextCancellation(t *testing.T) {
	// A canceled context surfaces Canceled - the same mapping the dial
	// path and the TypeScript reference use - rather than Unavailable.
	blackhole, blackholeConn := net.Pipe()
	defer blackhole.Close()
	defer blackholeConn.Close()

	// Never service the ClientHello; drain so the write does not block.
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := blackhole.Read(buf); err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := ExecuteTLSHandshake(ctx, blackholeConn, &sesamepb.TlsOptions{
		Sni: "example.com",
	}, "example.com", nil)
	if err == nil {
		t.Fatal("expected the canceled handshake to fail")
	}
	if status.Code(err) != codes.Canceled {
		t.Fatalf("expected Canceled, got %v", err)
	}
}

func TestValidateClientHelloSpec_RejectsOutOfRangeValues(t *testing.T) {
	cases := []struct {
		name string
		spec *sesamepb.ClientHelloSpec
	}{
		{"cipher suite above uint16", &sesamepb.ClientHelloSpec{CipherSuites: []int32{0x1301, 1 << 20}}},
		{"negative cipher suite", &sesamepb.ClientHelloSpec{CipherSuites: []int32{-1}}},
		{"supported group above uint16", &sesamepb.ClientHelloSpec{SupportedGroups: []int32{70000}}},
		{"negative supported group", &sesamepb.ClientHelloSpec{SupportedGroups: []int32{-29}}},
		{"signature algorithm above uint16", &sesamepb.ClientHelloSpec{SignatureAlgorithms: []int32{1 << 16}}},
		{"negative signature algorithm", &sesamepb.ClientHelloSpec{SignatureAlgorithms: []int32{-1}}},
		{"extension type above uint16", &sesamepb.ClientHelloSpec{Extensions: []*sesamepb.ClientHelloExtension{{Type: 1 << 16}}}},
		{"negative extension type", &sesamepb.ClientHelloSpec{Extensions: []*sesamepb.ClientHelloExtension{{Type: -1}}}},
		{"nil extension entry", &sesamepb.ClientHelloSpec{Extensions: []*sesamepb.ClientHelloExtension{nil}}},
		{"compression method above uint8", &sesamepb.ClientHelloSpec{CompressionMethods: []int32{256}}},
		{"negative compression method", &sesamepb.ClientHelloSpec{CompressionMethods: []int32{-1}}},
		{"session id length below sentinel", &sesamepb.ClientHelloSpec{SessionIdLength: -2}},
		{"session id length above 32", &sesamepb.ClientHelloSpec{SessionIdLength: 33}},
		{"negative pad to size", &sesamepb.ClientHelloSpec{TargetSize: -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateClientHelloSpec(tc.spec)
			if err == nil {
				t.Fatal("expected InvalidArgument, got nil")
			}
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("expected InvalidArgument, got %v", err)
			}
		})
	}

	// In-range values must validate cleanly, including GREASE-range IDs.
	valid := &sesamepb.ClientHelloSpec{
		CipherSuites:        []int32{0x1301, 0xc02b, 0x0a0a},
		SupportedGroups:     []int32{29, 23, 0x1a1a},
		SignatureAlgorithms: []int32{0x0403},
		Extensions: []*sesamepb.ClientHelloExtension{
			{Type: 0, Body: &sesamepb.ClientHelloExtension_Raw{Raw: []byte{}}},
			{Type: 65535, Body: &sesamepb.ClientHelloExtension_Auto{Auto: &sesamepb.AutoExtensionBody{}}},
		},
		CompressionMethods: []int32{0},
		SessionIdLength:    32,
		TargetSize:         512,
	}
	if err := ValidateClientHelloSpec(valid); err != nil {
		t.Fatalf("expected valid spec, got %v", err)
	}
	if err := ValidateClientHelloSpec(nil); err != nil {
		t.Fatalf("nil spec must validate (engine default), got %v", err)
	}
}

func TestBuildTLSConfig_RejectsMinVersionAboveMax(t *testing.T) {
	_, err := BuildTLSConfig(&sesamepb.TlsOptions{
		Sni:        "example.com",
		MinVersion: sesamepb.TlsVersion_TLS_VERSION_TLS_1_3,
		MaxVersion: sesamepb.TlsVersion_TLS_VERSION_TLS_1_2,
	}, "")
	if err == nil {
		t.Fatal("expected InvalidArgument for min_version above max_version")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}

	// A valid range, and one-sided bounds, must still build.
	for _, opts := range []*sesamepb.TlsOptions{
		{Sni: "example.com", MinVersion: sesamepb.TlsVersion_TLS_VERSION_TLS_1_2, MaxVersion: sesamepb.TlsVersion_TLS_VERSION_TLS_1_3},
		{Sni: "example.com", MinVersion: sesamepb.TlsVersion_TLS_VERSION_TLS_1_2},
		{Sni: "example.com", MaxVersion: sesamepb.TlsVersion_TLS_VERSION_TLS_1_3},
	} {
		if _, err := BuildTLSConfig(opts, ""); err != nil {
			t.Fatalf("BuildTLSConfig failed for valid bounds %+v: %v", opts, err)
		}
	}
}

func TestExecuteTLSHandshake_RejectsMinVersionAboveMax(t *testing.T) {
	// The version-range rule applies to every provider path, not just the
	// standard runtime: validation must precede any provider dispatch.
	_, _, err := ExecuteTLSHandshake(context.Background(), nil, &sesamepb.TlsOptions{
		Sni:        "example.com",
		MinVersion: sesamepb.TlsVersion_TLS_VERSION_TLS_1_3,
		MaxVersion: sesamepb.TlsVersion_TLS_VERSION_TLS_1_0,
	}, "", nil)
	if err == nil {
		t.Fatal("expected InvalidArgument for min_version above max_version")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestExecuteTLSHandshake_NilOptionsRejected(t *testing.T) {
	// The exported handshake must not silently no-op for nil options:
	// a caller passing nil needs an explicit error, not a passthrough
	// that leaves the connection cleartext while the caller believes
	// TLS was negotiated.
	_, _, err := ExecuteTLSHandshake(context.Background(), nil, nil, "example.com", nil)
	if err == nil {
		t.Fatal("expected an explicit error for nil options")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestBuildTLSConfig_NilOptionsRejected(t *testing.T) {
	// Same contract as ExecuteTLSHandshake: a silent (nil, nil) return
	// let callers proceed with no TLS configuration and no error.
	cfg, err := BuildTLSConfig(nil, "example.com")
	if err == nil || status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for nil options, got cfg=%v err=%v", cfg, err)
	}
}

func TestExecuteProxyHops_TooManyHops(t *testing.T) {
	hops := make([]*sesamepb.ProxyHop, MaxProxyHops+1)
	for i := range hops {
		hops[i] = &sesamepb.ProxyHop{
			Type:    sesamepb.ProxyHop_TYPE_HTTP_CONNECT,
			Address: &typepb.NetAddr{Network: "tcp", Address: "127.0.0.1:1"},
		}
	}

	// The bound must be enforced before any dialing: the dialer failing the
	// test proves validation precedes resource commitment.
	_, _, err := ExecuteProxyHops(context.Background(), dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		t.Fatal("dialer must not be called for an over-bound hop chain")
		return nil, nil
	}), &sesamepb.ProxyOptions{Hops: hops}, "tcp", "example.com:80")
	if err == nil {
		t.Fatal("expected InvalidArgument for too many hops")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestExecuteProxyHops_RejectsControlBytes(t *testing.T) {
	// CR/LF in a proxy handshake field would inject header or request
	// lines into the raw CONNECT request; other control bytes are
	// garbage in an authority or header value. Validation must fire
	// before any dialing.
	validAddr := &typepb.NetAddr{Network: "tcp", Address: "proxy.internal:8080"}

	cases := []struct {
		name    string
		target  string
		hopMods [2]func(*sesamepb.ProxyHop)
	}{
		{
			name:   "CR/LF in target injects a Host line",
			target: "target.internal:443\r\nHost: evil.example",
		},
		{
			name:   "NUL in target",
			target: "target\x00.internal:443",
		},
		{
			name:   "DEL in target",
			target: "target.internal:443\x7f",
		},
		{
			name:   "CR in hop address",
			target: "target.internal:443",
			hopMods: [2]func(*sesamepb.ProxyHop){func(h *sesamepb.ProxyHop) {
				h.Address = &typepb.NetAddr{Network: "tcp", Address: "proxy\r.internal:8080"}
			}},
		},
		{
			name:   "LF in second hop address",
			target: "target.internal:443",
			hopMods: [2]func(*sesamepb.ProxyHop){
				nil,
				func(h *sesamepb.ProxyHop) {
					h.Address = &typepb.NetAddr{Network: "tcp", Address: "proxy2.\ninternal:8080"}
				},
			},
		},
		{
			name:    "CRLF in auth_header injects a header line",
			target:  "target.internal:443",
			hopMods: [2]func(*sesamepb.ProxyHop){func(h *sesamepb.ProxyHop) { h.AuthHeader = "Bearer tok\r\nX-Injected: 1" }},
		},
		{
			name:    "NUL in auth_header",
			target:  "target.internal:443",
			hopMods: [2]func(*sesamepb.ProxyHop){func(h *sesamepb.ProxyHop) { h.AuthHeader = "Basic abc\x00def" }},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hops := make([]*sesamepb.ProxyHop, len(tc.hopMods))
			for i, mod := range tc.hopMods {
				hop := &sesamepb.ProxyHop{
					Type:    sesamepb.ProxyHop_TYPE_HTTP_CONNECT,
					Address: validAddr,
				}
				if mod != nil {
					mod(hop)
				}
				hops[i] = hop
			}

			_, _, err := ExecuteProxyHops(context.Background(), dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
				t.Fatal("dialer must not be called for a control-byte proxy field")
				return nil, nil
			}), &sesamepb.ProxyOptions{Hops: hops}, "tcp", tc.target)
			if err == nil {
				t.Fatal("expected InvalidArgument for a control byte in a proxy handshake field")
			}
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("expected InvalidArgument, got %v", err)
			}
		})
	}

	// Clean fields must still reach the dialer (validation must not
	// over-reject): the dial failure surfacing proves it got past
	// validation. The dial error is formatted with %v (not wrapped),
	// so assert on the code and the message.
	_, _, err := ExecuteProxyHops(context.Background(), dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		return nil, errors.New("dial reached")
	}), &sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{{
		Type:       sesamepb.ProxyHop_TYPE_HTTP_CONNECT,
		Address:    validAddr,
		AuthHeader: "Bearer valid-token",
	}}}, "tcp", "target.internal:443")
	if err == nil {
		t.Fatal("expected the dialer to be reached for clean fields")
	}
	if status.Code(err) != codes.Unavailable || !strings.Contains(err.Error(), "dialing first proxy hop") {
		t.Fatalf("expected a first-hop dial failure for clean fields, got %v", err)
	}
}

func TestExecuteProxyHops_RejectsOversizedSocks5Domain(t *testing.T) {
	// RFC 1928 carries a domain target behind a u8 length field
	// (ATYP 0x03): a host longer than 255 bytes cannot be encoded, and
	// both stacks previously wrote the truncated byte (Go via x/net,
	// TS directly). Validation must reject before any dialing, without
	// over-rejecting HTTP CONNECT (raw request line, no length limit)
	// or the 255-byte boundary.
	oversized := strings.Repeat("a", 256) + ".example:443"
	exact255 := strings.Repeat("a", 250) + ".test:443"
	socksHop := func(address string) *sesamepb.ProxyHop {
		return &sesamepb.ProxyHop{
			Type:    sesamepb.ProxyHop_TYPE_SOCKS5,
			Address: &typepb.NetAddr{Network: "tcp", Address: address},
		}
	}
	httpHop := &sesamepb.ProxyHop{
		Type:    sesamepb.ProxyHop_TYPE_HTTP_CONNECT,
		Address: &typepb.NetAddr{Network: "tcp", Address: "proxy.internal:8080"},
	}

	neverDial := dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		t.Fatal("dialer must not be called for an unencodable SOCKS5 target")
		return nil, nil
	})

	// (a) Oversized domain on a SOCKS5 hop's target (last hop).
	_, _, err := ExecuteProxyHops(context.Background(), neverDial,
		&sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{socksHop("proxy.internal:1080")}},
		"tcp", oversized)
	if err == nil || status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for a >255-byte SOCKS5 target domain, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "255-byte limit") {
		t.Fatalf("expected the RFC 1928 limit in the error, got %v", err)
	}

	// (b) The same oversized domain on an HTTP CONNECT hop: no length
	// field on the wire, so validation must pass and the dial stage
	// must be reached.
	_, _, err = ExecuteProxyHops(context.Background(), dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		return nil, errors.New("dial reached")
	}),
		&sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{httpHop}},
		"tcp", oversized)
	if err == nil || status.Code(err) != codes.Unavailable || !strings.Contains(err.Error(), "dialing first proxy hop") {
		t.Fatalf("expected HTTP CONNECT to accept an oversized domain target and reach the dial, got %v", err)
	}

	// (c) Exactly 255 bytes: passes validation (reaches the dial).
	_, _, err = ExecuteProxyHops(context.Background(), dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		return nil, errors.New("dial reached")
	}),
		&sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{socksHop("proxy.internal:1080")}},
		"tcp", exact255)
	if err == nil || status.Code(err) != codes.Unavailable || !strings.Contains(err.Error(), "dialing first proxy hop") {
		t.Fatalf("expected the 255-byte boundary to pass validation and reach the dial, got %v", err)
	}

	// (d) Chained case: an oversized NEXT-hop address as a SOCKS5
	// hop's CONNECT target - rejected before any dialing.
	_, _, err = ExecuteProxyHops(context.Background(), neverDial,
		&sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{
			socksHop("proxy.internal:1080"),
			{Type: sesamepb.ProxyHop_TYPE_SOCKS5, Address: &typepb.NetAddr{Network: "tcp", Address: oversized}},
		}},
		"tcp", "target.internal:443")
	if err == nil || status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for an oversized chained SOCKS5 target, got %v", err)
	}

	// (e) IP-literal targets use ATYP 1/4 (no length field) and must
	// pass validation regardless of address length semantics.
	_, _, err = ExecuteProxyHops(context.Background(), dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		return nil, errors.New("dial reached")
	}),
		&sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{socksHop("proxy.internal:1080")}},
		"tcp", "[::1]:443")
	if err == nil || status.Code(err) != codes.Unavailable || !strings.Contains(err.Error(), "dialing first proxy hop") {
		t.Fatalf("expected the IPv6-literal SOCKS5 target to pass validation and reach the dial, got %v", err)
	}
}

func TestBuildTLSConfig_SniHandling(t *testing.T) {
	// A colon-bearing sni that is not a bare IPv6 literal must parse as
	// host:port; anything else is malformed client input, rejected with
	// INVALID_ARGUMENT, mirroring the TypeScript reference. Valid forms
	// strip their port; bare IPv6 literals pass through unchanged;
	// zoned literals are rejected (a zone is interface-local, and
	// net.ParseIP rejects zone IDs, so they are not bare literals).
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"plain host", "example.com", "example.com"},
		{"host with port", "example.com:443", "example.com"},
		{"bracketed ipv6 with port", "[::1]:443", "::1"},
		{"bare ipv6 literal", "::1", "::1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := BuildTLSConfig(&sesamepb.TlsOptions{Sni: tc.in}, "")
			if err != nil {
				t.Fatalf("expected sni %q to validate, got %v", tc.in, err)
			}
			if cfg.ServerName != tc.want {
				t.Fatalf("got ServerName %q, want %q", cfg.ServerName, tc.want)
			}
		})
	}

	for _, tc := range []struct{ name, in string }{
		{"multiple separators", "host:80:90"},
		{"whitespace port", "host: 80"},
		{"non-numeric port", "host:notaport"},
		{"bracketed literal without port", "[::1]"},
		{"zoned ipv6 literal", "::1%eth0"},
		// A zone is interface-local and can never be SNI; the
		// bracketed form must not survive the port-strip.
		{"bracketed zoned literal with port", "[::1%eth0]:443"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildTLSConfig(&sesamepb.TlsOptions{Sni: tc.in}, "")
			if err == nil || status.Code(err) != codes.InvalidArgument {
				t.Fatalf("expected InvalidArgument for sni %q, got %v", tc.in, err)
			}
		})
	}
}

// dialerFunc adapts a function to the Dialer interface.
type dialerFunc func(ctx context.Context, network, address string) (net.Conn, error)

func (f dialerFunc) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return f(ctx, network, address)
}

// closeWriteRecorder counts CloseWrite calls and stands in for a
// half-close-capable upstream.
type closeWriteRecorder struct {
	net.Conn
	calls int
}

func (c *closeWriteRecorder) CloseWrite() error {
	c.calls++
	return nil
}

func TestBufferedPrefixConn_CloseWriteDelegates(t *testing.T) {
	// The server's half-close handler type-asserts
	// interface{ CloseWrite() error } against the active conn; a
	// bufferedPrefixConn returned from the HTTP CONNECT path must promote
	// the wrapped conn's CloseWrite, or client half-closes are silently
	// dropped on every proxied tunnel.
	_, pipeEnd := net.Pipe()
	defer pipeEnd.Close()

	recorder := &closeWriteRecorder{Conn: pipeEnd}
	bpc := &bufferedPrefixConn{Conn: recorder, prefix: []byte("PRE")}

	if err := bpc.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite failed: %v", err)
	}
	if recorder.calls != 1 {
		t.Fatalf("expected CloseWrite to delegate to the wrapped conn once, got %d calls", recorder.calls)
	}

	// The prefix is read-side state and must still drain after the write
	// side closed.
	buf := make([]byte, 3)
	if n, err := bpc.Read(buf); err != nil || string(buf[:n]) != "PRE" {
		t.Fatalf("prefix read after CloseWrite = %q, %v; want PRE, <nil>", buf[:n], err)
	}

	// A wrapped conn without CloseWrite must report the limitation
	// instead of pretending the half-close happened.
	_, bareEnd := net.Pipe()
	defer bareEnd.Close()
	bare := &bufferedPrefixConn{Conn: bareEnd}
	if err := bare.CloseWrite(); err == nil {
		t.Fatal("expected an error when the wrapped conn does not support CloseWrite, got nil")
	}
}

func TestExecuteProxyHops_RejectsPasswordWithoutUsername(t *testing.T) {
	// Both handshake builders gate authentication on a non-empty
	// username, so a password-only hop would silently negotiate NO-AUTH
	// and the configured password would never reach the proxy; the
	// misconfiguration must be rejected before any dialing. A username
	// with an empty password remains valid (RFC 1929 allows it).
	validAddr := &typepb.NetAddr{Network: "tcp", Address: "proxy.internal:1080"}

	for _, tc := range []struct {
		name string
		hop  *sesamepb.ProxyHop
	}{
		{"socks5", &sesamepb.ProxyHop{Type: sesamepb.ProxyHop_TYPE_SOCKS5, Address: validAddr, Password: "secret"}},
		{"http connect", &sesamepb.ProxyHop{Type: sesamepb.ProxyHop_TYPE_HTTP_CONNECT, Address: validAddr, Password: "secret"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ExecuteProxyHops(context.Background(), dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
				t.Fatal("dialer must not be called for a password-only hop")
				return nil, nil
			}), &sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{tc.hop}}, "tcp", "target.internal:443")
			if err == nil || status.Code(err) != codes.InvalidArgument {
				t.Fatalf("expected InvalidArgument for a password-only hop, got %v", err)
			}
			if !strings.Contains(err.Error(), "password but no username") {
				t.Fatalf("expected the password/username rule in the error, got %v", err)
			}
		})
	}

	// A username with an empty password is valid: validation passes and
	// the dial stage is reached.
	_, _, err := ExecuteProxyHops(context.Background(), dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		return nil, errors.New("dial reached")
	}), &sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{{
		Type: sesamepb.ProxyHop_TYPE_SOCKS5, Address: validAddr, Username: "user", Password: "",
	}}}, "tcp", "target.internal:443")
	if err == nil || status.Code(err) != codes.Unavailable || !strings.Contains(err.Error(), "dialing first proxy hop") {
		t.Fatalf("expected a username-only hop to reach the dial stage, got %v", err)
	}
}

func TestExecuteProxyHops_RejectsMalformedAddresses(t *testing.T) {
	// A dial target or hop address that is not a parseable host:port is
	// malformed client input: INVALID_ARGUMENT at the boundary (AIP-193)
	// rather than an opaque failure from the dial path. Empty hop
	// addresses keep their specific missing-address errors.
	validHop := func() *sesamepb.ProxyHop {
		return &sesamepb.ProxyHop{
			Type:    sesamepb.ProxyHop_TYPE_HTTP_CONNECT,
			Address: &typepb.NetAddr{Network: "tcp", Address: "proxy.internal:8080"},
		}
	}

	neverDial := dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		t.Fatal("dialer must not be called for a malformed address")
		return nil, nil
	})
	dialReached := dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		return nil, errors.New("dial reached")
	})

	for _, target := range []string{"example.com", "::1", "host:80:90", "host:0", "host:65536", "host:+80", "host: 80", "host:"} {
		_, _, err := ExecuteProxyHops(context.Background(), neverDial, &sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{validHop()}}, "tcp", target)
		if err == nil || status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument for malformed target %q, got %v", target, err)
		}
		// The 0-hop path validates the target too.
		_, _, err = ExecuteProxyHops(context.Background(), neverDial, nil, "tcp", target)
		if err == nil || status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument for malformed 0-hop target %q, got %v", target, err)
		}
	}

	// A malformed hop address is rejected before dialing.
	_, _, err := ExecuteProxyHops(context.Background(), neverDial, &sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{{
		Type:    sesamepb.ProxyHop_TYPE_HTTP_CONNECT,
		Address: &typepb.NetAddr{Network: "tcp", Address: "proxy.internal"},
	}}}, "tcp", "target.internal:443")
	if err == nil || status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for a malformed hop address, got %v", err)
	}

	// An empty hop address keeps its specific missing-address error.
	_, _, err = ExecuteProxyHops(context.Background(), neverDial, &sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{{
		Type: sesamepb.ProxyHop_TYPE_HTTP_CONNECT,
	}}}, "tcp", "target.internal:443")
	if err == nil || status.Code(err) != codes.InvalidArgument || !strings.Contains(err.Error(), "first proxy hop missing address") {
		t.Fatalf("expected the specific missing-address error for an empty hop address, got %v", err)
	}

	// A missing middle-hop address is rejected before the first dial:
	// the neverDial dialer proves validation precedes the side effect.
	_, _, err = ExecuteProxyHops(context.Background(), neverDial, &sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{
		{Type: sesamepb.ProxyHop_TYPE_HTTP_CONNECT, Address: &typepb.NetAddr{Network: "tcp", Address: "proxy.internal:8080"}},
		{Type: sesamepb.ProxyHop_TYPE_HTTP_CONNECT},
	}}, "tcp", "target.internal:443")
	if err == nil || status.Code(err) != codes.InvalidArgument || !strings.Contains(err.Error(), "proxy hop 1 missing address") {
		t.Fatalf("expected the missing middle-hop address to be rejected before dialing, got %v", err)
	}

	// Well-formed targets and hop addresses must still reach the dial
	// stage.
	for _, target := range []string{"target.internal:443", "127.0.0.1:1", "[::1]:443", "host:80"} {
		_, _, err := ExecuteProxyHops(context.Background(), dialReached, &sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{validHop()}}, "tcp", target)
		if err == nil || status.Code(err) != codes.Unavailable || !strings.Contains(err.Error(), "dialing first proxy hop") {
			t.Fatalf("expected target %q to pass validation and reach the dial stage, got %v", target, err)
		}
	}
}

func TestExecuteProxyHops_MapsDialErrorCodes(t *testing.T) {
	// Raw dial errors surface with their canonical code (AIP-193) while
	// keeping the first-hop message; status errors from a custom dialer
	// pass through verbatim.
	hop := &sesamepb.ProxyHop{
		Type:    sesamepb.ProxyHop_TYPE_HTTP_CONNECT,
		Address: &typepb.NetAddr{Network: "tcp", Address: "proxy.internal:8080"},
	}

	for _, tc := range []struct {
		name    string
		err     error
		want    codes.Code
		wantMsg string
	}{
		{"plain error", errors.New("boom"), codes.Unavailable, "dialing first proxy hop"},
		{"canceled context", context.Canceled, codes.Canceled, "dialing first proxy hop"},
		{"context deadline", context.DeadlineExceeded, codes.DeadlineExceeded, "dialing first proxy hop"},
		{"net timeout", &net.OpError{Op: "dial", Err: fakeTimeoutError{}}, codes.DeadlineExceeded, "dialing first proxy hop"},
		{"status passthrough", status.Error(codes.NotFound, "custom dialer status"), codes.NotFound, "custom dialer status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ExecuteProxyHops(context.Background(), dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
				return nil, tc.err
			}), &sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{hop}}, "tcp", "target.internal:443")
			if err == nil || status.Code(err) != tc.want || !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("expected %v (%q), got %v", tc.want, tc.wantMsg, err)
			}
		})
	}
}

// fakeTimeoutError is a net.Error that always reports a timeout,
// standing in for a dial deadline firing.
type fakeTimeoutError struct{}

func (fakeTimeoutError) Error() string   { return "fake timeout" }
func (fakeTimeoutError) Timeout() bool   { return true }
func (fakeTimeoutError) Temporary() bool { return false }

func TestProxyHandshakes_UnblockOnContextCancel(t *testing.T) {
	// A client that disconnects mid-handshake must not hold the goroutine
	// and socket until the 30s fallback: the cancel watcher arms a past
	// deadline and the stall surfaces as Canceled, mirroring the
	// TypeScript reference's abort wiring.
	for _, tc := range []struct {
		name string
		hop  *sesamepb.ProxyHop
	}{
		{"http connect", &sesamepb.ProxyHop{Type: sesamepb.ProxyHop_TYPE_HTTP_CONNECT, Address: &typepb.NetAddr{Network: "tcp", Address: "proxy.internal:8080"}}},
		{"socks5", &sesamepb.ProxyHop{Type: sesamepb.ProxyHop_TYPE_SOCKS5, Address: &typepb.NetAddr{Network: "tcp", Address: "proxy.internal:8080"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A blackhole conn: the greeting/CONNECT is written, no
			// response ever arrives.
			serverPipe, clientPipe := net.Pipe()
			defer serverPipe.Close()
			defer clientPipe.Close()

			// Drain so writes do not block.
			go func() {
				buf := make([]byte, 1024)
				for {
					if _, err := serverPipe.Read(buf); err != nil {
						return
					}
				}
			}()

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				time.Sleep(100 * time.Millisecond)
				cancel()
			}()

			start := time.Now()
			_, _, err := ExecuteProxyHops(ctx, &pipeDialer{conn: clientPipe}, &sesamepb.ProxyOptions{Hops: []*sesamepb.ProxyHop{tc.hop}}, "tcp", "target.internal:443")
			if err == nil {
				t.Fatal("expected error from the cancelled handshake")
			}
			if status.Code(err) != codes.Canceled {
				t.Fatalf("expected Canceled, got %v", err)
			}
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Fatalf("handshake outlived the cancellation: %v", elapsed)
			}
		})
	}
}
