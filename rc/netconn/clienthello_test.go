package netconn

import (
	"context"
	cryptotls "crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	sesametls "github.com/joeycumines/sesame/rc/tls"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestGoClientHelloCapabilities(t *testing.T) {
	caps := GoClientHelloCapabilities()
	if caps.CustomSignatureAlgorithms || caps.CustomExtensionOrder || caps.RawExtensions ||
		caps.GreaseValues || caps.SessionIdLength || caps.PaddingControl ||
		caps.LegacyVersionControl || caps.CompressionMethods {
		t.Fatal("Go crypto/tls caps must not over-advertise unsupported dimensions")
	}
	if !caps.CustomCipherSuites || !caps.CustomSupportedGroups {
		t.Fatal("Go crypto/tls caps must advertise cipher suite and group control")
	}
}

func TestApplyGoClientHelloSpec_FailsClosedOnUnhonorableDimensions(t *testing.T) {
	const (
		maxTLS11 = uint16(cryptotls.VersionTLS11)
		maxTLS12 = uint16(cryptotls.VersionTLS12)
		maxTLS13 = uint16(cryptotls.VersionTLS13)
	)

	cases := []struct {
		name         string
		spec         *sesametls.ClientHelloSpec
		effectiveMax uint16
	}{
		{"signature algorithms have no public API",
			&sesametls.ClientHelloSpec{SignatureAlgorithms: []int32{0x0403}}, maxTLS13},
		{"extension list has no public API",
			&sesametls.ClientHelloSpec{Extensions: []*sesametls.ClientHelloExtension{{Type: 43}}}, maxTLS13},
		{"compression other than null",
			&sesametls.ClientHelloSpec{CompressionMethods: []int32{1}}, maxTLS13},
		{"multi-method compression",
			&sesametls.ClientHelloSpec{CompressionMethods: []int32{0, 1}}, maxTLS13},
		{"session id omit sentinel",
			&sesametls.ClientHelloSpec{SessionIdLength: -1}, maxTLS13},
		{"session id short length",
			&sesametls.ClientHelloSpec{SessionIdLength: 16}, maxTLS13},
		{"legacy version pinned",
			&sesametls.ClientHelloSpec{LegacyVersion: sesametls.TLSVersion_TLS_1_2}, maxTLS13},
		{"padding control",
			&sesametls.ClientHelloSpec{PadToSize: 512}, maxTLS13},
		{"cipher suites with TLS1.3 max append fixed suites",
			&sesametls.ClientHelloSpec{CipherSuites: []int32{0xc02b}}, maxTLS13},
		{"cipher suites with sub-TLS1.2 max",
			&sesametls.ClientHelloSpec{CipherSuites: []int32{0xc02b}}, maxTLS11},
		{"cipher order reversed against engine preference",
			&sesametls.ClientHelloSpec{CipherSuites: []int32{0x0035, 0x002f}}, maxTLS12},
		{"tls13-only cipher at tls12 max",
			&sesametls.ClientHelloSpec{CipherSuites: []int32{0x1301}}, maxTLS12},
		{"group order reversed against engine preference",
			&sesametls.ClientHelloSpec{SupportedGroups: []int32{23, 29}}, maxTLS13},
		{"unknown group silently dropped by engine",
			&sesametls.ClientHelloSpec{SupportedGroups: []int32{0x7a7a}}, maxTLS13},
		{"post-quantum group below tls13",
			&sesametls.ClientHelloSpec{SupportedGroups: []int32{4588, 29}}, maxTLS12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &cryptotls.Config{}
			err := ApplyGoClientHelloSpec(tc.spec, cfg, tc.effectiveMax)
			if err == nil {
				t.Fatal("expected FailedPrecondition, got nil")
			}
			if status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("expected FailedPrecondition, got %v", err)
			}
		})
	}
}

func TestApplyGoClientHelloSpec_AppliesHonorableDimensions(t *testing.T) {
	// [0x002f, 0x0035] and [29, 23] are in Go's fixed preference order in
	// BOTH the AES-hardware and no-AES-hardware tables, so these assertions
	// are hardware-independent.
	cfg := &cryptotls.Config{}
	spec := &sesametls.ClientHelloSpec{
		CipherSuites:       []int32{0x002f, 0x0035},
		SupportedGroups:    []int32{29, 23},
		CompressionMethods: []int32{0},
		SessionIdLength:    32,
	}
	if err := ApplyGoClientHelloSpec(spec, cfg, uint16(cryptotls.VersionTLS12)); err != nil {
		t.Fatalf("expected spec to be honorable, got %v", err)
	}
	if len(cfg.CipherSuites) != 2 || cfg.CipherSuites[0] != 0x002f || cfg.CipherSuites[1] != 0x0035 {
		t.Fatalf("unexpected CipherSuites: %v", cfg.CipherSuites)
	}
	if len(cfg.CurvePreferences) != 2 || cfg.CurvePreferences[0] != 29 || cfg.CurvePreferences[1] != 23 {
		t.Fatalf("unexpected CurvePreferences: %v", cfg.CurvePreferences)
	}
}

func TestApplyGoClientHelloSpec_EmptySpecIsEngineDefault(t *testing.T) {
	cfg := &cryptotls.Config{}
	if err := ApplyGoClientHelloSpec(&sesametls.ClientHelloSpec{}, cfg, uint16(cryptotls.VersionTLS13)); err != nil {
		t.Fatalf("empty spec must select engine defaults, got %v", err)
	}
	if cfg.CipherSuites != nil || cfg.CurvePreferences != nil {
		t.Fatal("empty spec must not alter engine defaults")
	}
	if err := ApplyGoClientHelloSpec(nil, cfg, uint16(cryptotls.VersionTLS13)); err != nil {
		t.Fatalf("nil spec must select engine defaults, got %v", err)
	}
}

func TestVerifyAppliedClientHello(t *testing.T) {
	spec := &sesametls.ClientHelloSpec{
		CipherSuites:    []int32{0xc02f},
		SupportedGroups: []int32{29},
		SessionIdLength: 32,
		Extensions: []*sesametls.ClientHelloExtension{
			{Type: 0, Body: &sesametls.ClientHelloExtension_Raw{Raw: []byte{1, 2, 3}}},
			{Type: 16, Body: &sesametls.ClientHelloExtension_Auto{Auto: &sesametls.AutoExtensionBody{}}},
		},
	}

	if err := VerifyAppliedClientHello(nil, nil); err != nil {
		t.Fatalf("nil/nil must verify: %v", err)
	}
	if err := VerifyAppliedClientHello(spec, nil); err == nil {
		t.Fatal("missing echo must fail closed")
	}
	if err := VerifyAppliedClientHello(nil, spec); err == nil {
		t.Fatal("unrequested echo must fail closed")
	}
	if err := VerifyAppliedClientHello(spec, proto.Clone(spec).(*sesametls.ClientHelloSpec)); err != nil {
		t.Fatalf("identical echo must verify: %v", err)
	}

	mutated := proto.Clone(spec).(*sesametls.ClientHelloSpec)
	mutated.SessionIdLength = 0
	if err := VerifyAppliedClientHello(spec, mutated); err == nil {
		t.Fatal("mutated scalar echo must fail closed")
	}

	mutatedRaw := proto.Clone(spec).(*sesametls.ClientHelloSpec)
	mutatedRaw.Extensions[0].Body = &sesametls.ClientHelloExtension_Raw{Raw: []byte{1, 2, 4}}
	if err := VerifyAppliedClientHello(spec, mutatedRaw); err == nil {
		t.Fatal("mutated raw extension body must fail closed")
	}
}

// capturingConn records every byte written to it and EOFs every read, so
// a TLS client handshake fails immediately after its first flight -
// which is exactly the flight under test.
type capturingConn struct {
	mu     sync.Mutex
	writes []byte
}

func (c *capturingConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.writes = append(c.writes, p...)
	c.mu.Unlock()
	return len(p), nil
}

func (c *capturingConn) Read(_ []byte) (int, error) { return 0, io.EOF }

func (c *capturingConn) written() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.writes...)
}

// Dead net.Conn surface; the client handshake path never touches it.
func (c *capturingConn) Close() error                      { return nil }
func (c *capturingConn) LocalAddr() net.Addr               { return nil }
func (c *capturingConn) RemoteAddr() net.Addr              { return nil }
func (c *capturingConn) SetDeadline(_ time.Time) error     { return nil }
func (c *capturingConn) SetReadDeadline(_ time.Time) error { return nil }
func (c *capturingConn) SetWriteDeadline(_ time.Time) error {
	return nil
}

// wireClientHello holds the ClientHello dimensions the engine claims to
// control, as actually emitted on the wire.
type wireClientHello struct {
	legacyVersion uint16
	sessionIDLen  int
	cipherSuites  []uint16
	compression   []byte
	extensions    map[uint16][]byte
}

// parseWireClientHello parses the first TLS record in buf as a
// ClientHello, failing the test on any framing anomaly. The parser is
// deliberately independent of the mirrors in clienthello.go: it reads
// the wire, nothing else.
func parseWireClientHello(t *testing.T, buf []byte) wireClientHello {
	t.Helper()

	if len(buf) < 5 {
		t.Fatalf("captured flight too short for a TLS record: %d bytes", len(buf))
	}
	if buf[0] != 0x16 {
		t.Fatalf("first record content type = %#x, want handshake (0x16)", buf[0])
	}
	recLen := int(binary.BigEndian.Uint16(buf[3:5]))
	if len(buf) < 5+recLen {
		t.Fatalf("record truncated: header says %d bytes, captured %d", recLen, len(buf)-5)
	}
	rec := buf[5 : 5+recLen]

	if len(rec) < 4 {
		t.Fatalf("handshake header truncated: %d bytes", len(rec))
	}
	if rec[0] != 0x01 {
		t.Fatalf("handshake type = %#x, want ClientHello (0x01)", rec[0])
	}
	hsLen := int(rec[1])<<16 | int(rec[2])<<8 | int(rec[3])
	if len(rec) < 4+hsLen {
		t.Fatalf("ClientHello truncated: header says %d bytes, record carries %d", hsLen, len(rec)-4)
	}
	ch := rec[4 : 4+hsLen]

	off := 0
	take := func(n int, what string) []byte {
		if off+n > len(ch) {
			t.Fatalf("ClientHello overrun reading %s: need %d bytes at offset %d, have %d", what, n, off, len(ch)-off)
		}
		b := ch[off : off+n]
		off += n
		return b
	}

	out := wireClientHello{extensions: make(map[uint16][]byte)}

	out.legacyVersion = binary.BigEndian.Uint16(take(2, "legacy_version"))
	take(32, "random")
	out.sessionIDLen = int(take(1, "session_id length")[0])
	take(out.sessionIDLen, "session_id")

	csLen := int(binary.BigEndian.Uint16(take(2, "cipher_suites length")))
	suites := take(csLen, "cipher_suites")
	for i := 0; i+2 <= len(suites); i += 2 {
		out.cipherSuites = append(out.cipherSuites, binary.BigEndian.Uint16(suites[i:]))
	}

	cpLen := int(take(1, "compression_methods length")[0])
	out.compression = take(cpLen, "compression_methods")

	extsLen := int(binary.BigEndian.Uint16(take(2, "extensions length")))
	exts := take(extsLen, "extensions")
	eoff := 0
	for eoff+4 <= len(exts) {
		extType := binary.BigEndian.Uint16(exts[eoff:])
		extBodyLen := int(binary.BigEndian.Uint16(exts[eoff+2:]))
		if eoff+4+extBodyLen > len(exts) {
			t.Fatalf("extension %#x overruns the extensions block: %d bytes at offset %d of %d", extType, extBodyLen, eoff, len(exts))
		}
		out.extensions[extType] = exts[eoff+4 : eoff+4+extBodyLen]
		eoff += 4 + extBodyLen
	}

	return out
}

func TestGoClientHelloEmissionMatchesRequest(t *testing.T) {
	// The applied_client_hello echo and the mirror tables in
	// clienthello.go both assert; neither observes the wire. If a Go
	// toolchain bump changes what crypto/tls emits for a validated spec
	// - or the mirrors go stale and let a stale spec pass validation -
	// the fingerprint silently diverges, the exact mis-impersonation
	// the fail-closed engine exists to prevent. This test pins the
	// emission to the REQUESTED spec (not to the mirrors), so either
	// drift mode fails CI.
	//
	// [0x002f, 0x0035] and [29, 23] are in Go's fixed preference order
	// in BOTH the AES-hardware and no-AES-hardware tables, so the
	// assertions are hardware-independent (same values as
	// TestApplyGoClientHelloSpec_AppliesHonorableDimensions).
	spec := &sesametls.ClientHelloSpec{
		CipherSuites:       []int32{0x002f, 0x0035},
		SupportedGroups:    []int32{29, 23},
		CompressionMethods: []int32{0},
		SessionIdLength:    32,
	}

	for _, tc := range []struct {
		name string
		alpn []string
	}{
		{name: "with alpn", alpn: []string{"h2"}},
		{name: "without alpn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := &sesametls.TLSOptions{
				ServerName:    "example.com",
				MaxVersion:    sesametls.TLSVersion_TLS_1_2,
				ClientHello:   spec,
				AlpnProtocols: tc.alpn,
			}

			conn := &capturingConn{}
			_, _, err := ExecuteTLSHandshake(context.Background(), conn, opts, "example.com", nil)
			// There is no peer: the handshake must fail after its first
			// flight, and the failure must surface as Unavailable.
			if err == nil {
				t.Fatal("expected handshake failure against the capture-only conn, got nil")
			}
			if status.Code(err) != codes.Unavailable {
				t.Fatalf("expected Unavailable from the failed handshake, got %v", err)
			}

			ch := parseWireClientHello(t, conn.written())

			if ch.legacyVersion != cryptotls.VersionTLS12 {
				t.Errorf("wire legacy_version = %#04x, want 0x0303", ch.legacyVersion)
			}
			if ch.sessionIDLen != 32 {
				t.Errorf("wire session_id length = %d, want 32", ch.sessionIDLen)
			}
			wantCiphers := []uint16{0x002f, 0x0035}
			if len(ch.cipherSuites) != len(wantCiphers) {
				t.Fatalf("wire cipher_suites = %v, want %v", ch.cipherSuites, wantCiphers)
			}
			for i := range wantCiphers {
				if ch.cipherSuites[i] != wantCiphers[i] {
					t.Fatalf("wire cipher_suites = %v, want %v", ch.cipherSuites, wantCiphers)
				}
			}
			if len(ch.compression) != 1 || ch.compression[0] != 0 {
				t.Errorf("wire compression_methods = %v, want [0]", ch.compression)
			}

			groupsBody, ok := ch.extensions[10]
			if !ok {
				t.Fatal("no supported_groups extension on the wire")
			}
			if len(groupsBody) != 2+4 {
				t.Fatalf("supported_groups body = %x, want a 2-entry list", groupsBody)
			}
			if binary.BigEndian.Uint16(groupsBody[2:]) != 29 || binary.BigEndian.Uint16(groupsBody[4:]) != 23 {
				t.Errorf("wire supported_groups = %v, want [29 23]", []uint16{binary.BigEndian.Uint16(groupsBody[2:]), binary.BigEndian.Uint16(groupsBody[4:])})
			}

			alpnBody, hasALPN := ch.extensions[16]
			if len(tc.alpn) > 0 {
				if !hasALPN {
					t.Fatal("ALPN protocols were requested but the wire carries no ALPN extension")
				}
				// ALPN body: u16 protocol-list length, then each entry
				// as u8 length + bytes - for "h2": 00 03 02 68 32.
				if len(alpnBody) != 5 ||
					binary.BigEndian.Uint16(alpnBody[:2]) != 3 ||
					alpnBody[2] != 2 ||
					string(alpnBody[3:]) != "h2" {
					t.Errorf("wire ALPN body = %x, want the single protocol \"h2\"", alpnBody)
				}
			} else if hasALPN {
				t.Fatalf("no ALPN protocols were requested but the wire carries an ALPN extension: %x", alpnBody)
			}
		})
	}
}
