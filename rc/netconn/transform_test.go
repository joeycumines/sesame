package netconn

import (
	"context"
	cryptotls "crypto/tls"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	sesameproxy "github.com/joeycumines/sesame/rc/proxy"
	sesametls "github.com/joeycumines/sesame/rc/tls"
	"github.com/joeycumines/sesame/type/netaddr"
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

func TestExecuteTLSHandshake_FailsClosedOnUnhonorableSpec(t *testing.T) {
	// The builtin engine MUST fail closed (FAILED_PRECONDITION) when a
	// requested ClientHello dimension cannot be honored exactly.
	opts := &sesametls.TLSOptions{
		ServerName: "example.com",
		ClientHello: &sesametls.ClientHelloSpec{
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
	_, _, err = ExecuteTLSHandshake(context.Background(), rawConn, &sesametls.TLSOptions{
		ServerName: "example.com",
		ClientHello: &sesametls.ClientHelloSpec{
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

func TestValidateClientHelloSpec_RejectsOutOfRangeValues(t *testing.T) {
	cases := []struct {
		name string
		spec *sesametls.ClientHelloSpec
	}{
		{"cipher suite above uint16", &sesametls.ClientHelloSpec{CipherSuites: []int32{0x1301, 1 << 20}}},
		{"negative cipher suite", &sesametls.ClientHelloSpec{CipherSuites: []int32{-1}}},
		{"supported group above uint16", &sesametls.ClientHelloSpec{SupportedGroups: []int32{70000}}},
		{"negative supported group", &sesametls.ClientHelloSpec{SupportedGroups: []int32{-29}}},
		{"signature algorithm above uint16", &sesametls.ClientHelloSpec{SignatureAlgorithms: []int32{1 << 16}}},
		{"negative signature algorithm", &sesametls.ClientHelloSpec{SignatureAlgorithms: []int32{-1}}},
		{"extension type above uint16", &sesametls.ClientHelloSpec{Extensions: []*sesametls.ClientHelloExtension{{Type: 1 << 16}}}},
		{"negative extension type", &sesametls.ClientHelloSpec{Extensions: []*sesametls.ClientHelloExtension{{Type: -1}}}},
		{"nil extension entry", &sesametls.ClientHelloSpec{Extensions: []*sesametls.ClientHelloExtension{nil}}},
		{"compression method above uint8", &sesametls.ClientHelloSpec{CompressionMethods: []int32{256}}},
		{"negative compression method", &sesametls.ClientHelloSpec{CompressionMethods: []int32{-1}}},
		{"session id length below sentinel", &sesametls.ClientHelloSpec{SessionIdLength: -2}},
		{"session id length above 32", &sesametls.ClientHelloSpec{SessionIdLength: 33}},
		{"negative pad to size", &sesametls.ClientHelloSpec{PadToSize: -1}},
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
	valid := &sesametls.ClientHelloSpec{
		CipherSuites:        []int32{0x1301, 0xc02b, 0x0a0a},
		SupportedGroups:     []int32{29, 23, 0x1a1a},
		SignatureAlgorithms: []int32{0x0403},
		Extensions: []*sesametls.ClientHelloExtension{
			{Type: 0, Body: &sesametls.ClientHelloExtension_Raw{Raw: []byte{}}},
			{Type: 65535, Body: &sesametls.ClientHelloExtension_Auto{Auto: &sesametls.AutoExtensionBody{}}},
		},
		CompressionMethods: []int32{0},
		SessionIdLength:    32,
		PadToSize:          512,
	}
	if err := ValidateClientHelloSpec(valid); err != nil {
		t.Fatalf("expected valid spec, got %v", err)
	}
	if err := ValidateClientHelloSpec(nil); err != nil {
		t.Fatalf("nil spec must validate (engine default), got %v", err)
	}
}

func TestBuildTLSConfig_RejectsMinVersionAboveMax(t *testing.T) {
	_, err := BuildTLSConfig(&sesametls.TLSOptions{
		ServerName: "example.com",
		MinVersion: sesametls.TLSVersion_TLS_1_3,
		MaxVersion: sesametls.TLSVersion_TLS_1_2,
	}, "")
	if err == nil {
		t.Fatal("expected InvalidArgument for min_version above max_version")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}

	// A valid range, and one-sided bounds, must still build.
	for _, opts := range []*sesametls.TLSOptions{
		{ServerName: "example.com", MinVersion: sesametls.TLSVersion_TLS_1_2, MaxVersion: sesametls.TLSVersion_TLS_1_3},
		{ServerName: "example.com", MinVersion: sesametls.TLSVersion_TLS_1_2},
		{ServerName: "example.com", MaxVersion: sesametls.TLSVersion_TLS_1_3},
	} {
		if _, err := BuildTLSConfig(opts, ""); err != nil {
			t.Fatalf("BuildTLSConfig failed for valid bounds %+v: %v", opts, err)
		}
	}
}

func TestExecuteTLSHandshake_RejectsMinVersionAboveMax(t *testing.T) {
	// The version-range rule applies to every provider path, not just the
	// standard runtime: validation must precede any provider dispatch.
	_, _, err := ExecuteTLSHandshake(context.Background(), nil, &sesametls.TLSOptions{
		ServerName: "example.com",
		MinVersion: sesametls.TLSVersion_TLS_1_3,
		MaxVersion: sesametls.TLSVersion_TLS_1_0,
	}, "", nil)
	if err == nil {
		t.Fatal("expected InvalidArgument for min_version above max_version")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestExecuteProxyHops_TooManyHops(t *testing.T) {
	hops := make([]*sesameproxy.ProxyHop, MaxProxyHops+1)
	for i := range hops {
		hops[i] = &sesameproxy.ProxyHop{
			Type:    sesameproxy.ProxyHop_HTTP_CONNECT,
			Address: &netaddr.NetAddr{Network: "tcp", Address: "127.0.0.1:1"},
		}
	}

	// The bound must be enforced before any dialing: the dialer failing the
	// test proves validation precedes resource commitment.
	_, _, err := ExecuteProxyHops(context.Background(), dialerFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		t.Fatal("dialer must not be called for an over-bound hop chain")
		return nil, nil
	}), &sesameproxy.ProxyOptions{Hops: hops}, "tcp", "example.com:80")
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
	validAddr := &netaddr.NetAddr{Network: "tcp", Address: "proxy.internal:8080"}

	cases := []struct {
		name    string
		target  string
		hopMods [2]func(*sesameproxy.ProxyHop)
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
			hopMods: [2]func(*sesameproxy.ProxyHop){func(h *sesameproxy.ProxyHop) {
				h.Address = &netaddr.NetAddr{Network: "tcp", Address: "proxy\r.internal:8080"}
			}},
		},
		{
			name:   "LF in second hop address",
			target: "target.internal:443",
			hopMods: [2]func(*sesameproxy.ProxyHop){
				nil,
				func(h *sesameproxy.ProxyHop) {
					h.Address = &netaddr.NetAddr{Network: "tcp", Address: "proxy2.\ninternal:8080"}
				},
			},
		},
		{
			name:    "CRLF in auth_header injects a header line",
			target:  "target.internal:443",
			hopMods: [2]func(*sesameproxy.ProxyHop){func(h *sesameproxy.ProxyHop) { h.AuthHeader = "Bearer tok\r\nX-Injected: 1" }},
		},
		{
			name:    "NUL in auth_header",
			target:  "target.internal:443",
			hopMods: [2]func(*sesameproxy.ProxyHop){func(h *sesameproxy.ProxyHop) { h.AuthHeader = "Basic abc\x00def" }},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hops := make([]*sesameproxy.ProxyHop, len(tc.hopMods))
			for i, mod := range tc.hopMods {
				hop := &sesameproxy.ProxyHop{
					Type:    sesameproxy.ProxyHop_HTTP_CONNECT,
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
			}), &sesameproxy.ProxyOptions{Hops: hops}, "tcp", tc.target)
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
	}), &sesameproxy.ProxyOptions{Hops: []*sesameproxy.ProxyHop{{
		Type:       sesameproxy.ProxyHop_HTTP_CONNECT,
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
