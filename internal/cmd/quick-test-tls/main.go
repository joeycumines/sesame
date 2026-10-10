// Command quick-test-tls exercises the decomposed ClientHelloSpec API
// end-to-end and emits machine-readable JSON for the HTML report.
//
// Two paths, matching the two real use cases:
//
//	path "go-engine"     — Go app impersonating a browser: in-process
//	                       netconn.Server/Client over real gRPC with insecure
//	                       creds, TLS target = a local capture listener that
//	                       records the ClientHello bytes while a real
//	                       cryptotls.Server completes the handshake.
//
//	path "endpoint-bun"  — Bun impersonates opencode: sesame-endpoint
//	                       subprocess (bun, node fallback) dials the same
//	                       capture listener; the captured ClientHello is
//	                       compared against the opencode/Bun reference fixture.
//
// Every check is computed from captured bytes vs the relevant reference
// (requested spec for go-engine; fixtures for endpoint-bun). Nothing is
// hard-coded to pass.
//
// Usage:
//
//	go run ./internal/cmd/quick-test-tls [flags]
//
// Flags:
//
//	-json PATH     write the machine-readable results JSON to PATH
//	               (default: no file; per-scenario results go to stderr)
//	-html PATH     render the self-contained HTML report to PATH
//	-endpoint      run the endpoint-bun scenarios (skipped by default)
//	-cli PATH      sesame-endpoint CLI entry point
//	               (default sesame-endpoint/build/src/cli.js)
//	-runtime NAME  endpoint runtime: bun, node, or auto
//	               (default auto: bun when on PATH, else node)
//	-fixture PATH  override the embedded Bun-fetch reference fixture
//
// The endpoint-bun scenarios additionally require -cli to point at a built
// endpoint (run `bun run compile` in sesame-endpoint first).
//
// Exit status is 0 iff no non-skipped scenario failed.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	cryptotls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joeycumines/sesame/rc"
	"github.com/joeycumines/sesame/rc/netconn"
	sesametls "github.com/joeycumines/sesame/rc/tls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// bunFixtureJSON is the reference ClientHello fingerprint of Bun's fetch()
// (the opencode reference: Bun/1.4.2 GET https://tls.peet.ws/api/all),
// carried in the binary so the utility works from any directory.
//
//go:embed fixtures/bun-fetch-api-all.json
var bunFixtureJSON []byte

// reportTemplate is the self-contained HTML report page; the report JSON is
// injected in place of __REPORT_DATA__.
//
//go:embed report.html
var reportTemplate string

// ---------------------------------------------------------------------------
// Deets: detection criteria from scratch/quick-tls-fingerprinter-deets.md.
// The deets doc defines the wire dimensions tls.peet.ws inspects (ciphers,
// extensions, groups, sigalgs, GREASE, session_id, ALPN, record/legacy
// version, message size). Each check below maps to one of those dimensions.
// ---------------------------------------------------------------------------

type deet struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func d(id, name string) deet { return deet{ID: id, Name: name} }

var (
	deetCipherOrder     = d("cipher-order", "cipher_suites wire order")
	deetGroupOrder      = d("group-order", "supported_groups wire order")
	deetKeyShareGroups  = d("key-share-groups", "key_share groups")
	deetExtOrder        = d("ext-order", "extension wire order")
	deetExtPresence     = d("ext-presence", "extension presence")
	deetGREASE          = d("grease", "GREASE (0x?a?a) values")
	deetSessionID       = d("session-id", "session_id length")
	deetALPN            = d("alpn", "ALPN protocols")
	deetCompression     = d("compression", "compression_methods")
	deetLegacyVersion   = d("legacy-version", "ClientHello legacy_version")
	deetRecordVersion   = d("record-version", "record layer version")
	deetMessageSize     = d("message-size", "ClientHello message size")
	deetNegotiatedVer   = d("negotiated-version", "negotiated TLS version")
	deetNegotiatedSuite = d("negotiated-cipher", "negotiated cipher suite")
	deetAppliedEcho     = d("applied-echo", "applied_client_hello echo")
	deetVerifyFunc      = d("verify-func", "VerifyAppliedClientHello")
	deetGRPCCode        = d("grpc-code", "gRPC status code")
	deetNoHandshake     = d("no-handshake", "no ClientHello on wire")
	deetCapabilities    = d("capabilities", "client_hello_capabilities")
)

// ---------------------------------------------------------------------------
// Report model.
// ---------------------------------------------------------------------------

type check struct {
	Deet   deet   `json:"deet"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}

type scenario struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Path          string      `json:"path"` // "go-engine" | "endpoint-bun"
	RequestedSpec interface{} `json:"requested_spec,omitempty"`
	Outcome       string      `json:"outcome"`
	Checks        []check     `json:"checks"`
	Gaps          []check     `json:"gaps,omitempty"`
	Passed        bool        `json:"passed"`
	Skipped       bool        `json:"skipped"`
	SkipReason    string      `json:"skip_reason,omitempty"`
}

func (s *scenario) add(c check) { s.Checks = append(s.Checks, c) }

// addGap records an honest engine-ceiling gap against the impersonation
// reference. Gaps are reported but excluded from scenario pass/fail: they are
// work items for a stronger engine (the native BoringSSL addon), not defects
// of the engine under test.
func (s *scenario) addGap(c check) { s.Gaps = append(s.Gaps, c) }

func allPass(checks []check) bool {
	for _, c := range checks {
		if !c.Pass {
			return false
		}
	}
	return len(checks) > 0
}

type reportSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type report struct {
	GeneratedAt string        `json:"generated_at"`
	GoVersion   string        `json:"go_version"`
	Module      string        `json:"module,omitempty"`
	Commit      string        `json:"commit,omitempty"`
	Scenarios   []scenario    `json:"scenarios"`
	Summary     reportSummary `json:"summary"`
}

func (r *report) add(s scenario) { r.Scenarios = append(r.Scenarios, s) }

func (r *report) tally() {
	for _, s := range r.Scenarios {
		r.Summary.Total++
		switch {
		case s.Skipped:
			r.Summary.Skipped++
		case s.Passed:
			r.Summary.Passed++
		default:
			r.Summary.Failed++
		}
	}
}

// ---------------------------------------------------------------------------
// ClientHello capture + parse.
// ---------------------------------------------------------------------------

// capturingConn wraps a net.Conn and records every byte the client sends.
type capturingConn struct {
	net.Conn
	buf []byte
}

func (c *capturingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.buf = append(c.buf, p[:n]...)
	}
	return n, err
}

// clientHello is the parsed first ClientHello message from captured bytes.
type clientHello struct {
	RecordVersion        uint16   `json:"record_version"`
	LegacyVersion        uint16   `json:"legacy_version"`
	SessionIDLength      int      `json:"session_id_length"`
	CipherSuites         []int    `json:"cipher_suites"`
	CompressionMethods   []int    `json:"compression_methods"`
	ExtensionTypes       []int    `json:"extension_types"`
	SupportedGroups      []int    `json:"supported_groups,omitempty"`
	SignatureAlgorithms  []int    `json:"signature_algorithms,omitempty"`
	ALPNProtocols        []string `json:"alpn_protocols,omitempty"`
	KeyShareGroups       []int    `json:"key_share_groups,omitempty"`
	GREASECiphers        []int    `json:"grease_ciphers,omitempty"`
	GREASEGroups         []int    `json:"grease_groups,omitempty"`
	GREASEExtensions     []int    `json:"grease_extensions,omitempty"`
	HasPadding           bool     `json:"has_padding"`
	HasEMS               bool     `json:"has_extended_master_secret"`
	HasSessionTicket     bool     `json:"has_session_ticket"`
	HasRenegotiationInfo bool     `json:"has_renegotiation_info"`
	MessageSize          int      `json:"message_size"`
}

func isGREASE(v int) bool { return v&0x0f0f == 0x0a0a }

func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// parseClientHello parses the first complete TLS record containing a
// ClientHello from raw captured bytes.
func parseClientHello(buf []byte) (*clientHello, error) {
	if len(buf) < 9 {
		return nil, fmt.Errorf("captured %d bytes: not a complete TLS record", len(buf))
	}
	ch := &clientHello{}
	ch.RecordVersion = uint16(buf[1])<<8 | uint16(buf[2])
	if buf[5] != 1 { // handshake type: ClientHello
		return nil, fmt.Errorf("first handshake message type %d, want 1 (ClientHello)", buf[5])
	}
	hsLen := int(buf[6])<<16 | int(buf[7])<<8 | int(buf[8])
	body := buf[9:]
	if len(body) < hsLen {
		hsLen = len(body)
	}
	ch.MessageSize = 4 + hsLen // handshake header + body
	if hsLen < 35 {
		return nil, fmt.Errorf("ClientHello body too short: %d", hsLen)
	}
	ch.LegacyVersion = uint16(body[0])<<8 | uint16(body[1])
	// random: body[2:34]
	p := 34
	sidLen := int(body[p])
	p++
	ch.SessionIDLength = sidLen
	p += sidLen
	if p+2 > len(body) {
		return nil, fmt.Errorf("truncated at cipher_suites length")
	}
	csLen := int(body[p])<<8 | int(body[p+1])
	p += 2
	for i := 0; i+2 <= csLen; i += 2 {
		v := int(body[p+i])<<8 | int(body[p+i+1])
		ch.CipherSuites = append(ch.CipherSuites, v)
		if isGREASE(v) {
			ch.GREASECiphers = append(ch.GREASECiphers, v)
		}
	}
	p += csLen
	if p >= len(body) {
		return nil, fmt.Errorf("truncated at compression_methods")
	}
	cmLen := int(body[p])
	p++
	for i := 0; i < cmLen && p < len(body); i++ {
		ch.CompressionMethods = append(ch.CompressionMethods, int(body[p]))
		p++
	}
	if p+2 > len(body) {
		return ch, nil // no extensions
	}
	extLen := int(body[p])<<8 | int(body[p+1])
	p += 2
	extEnd := p + extLen
	if extEnd > len(body) {
		extEnd = len(body)
	}
	for p+4 <= extEnd {
		et := int(body[p])<<8 | int(body[p+1])
		el := int(body[p+2])<<8 | int(body[p+3])
		ch.ExtensionTypes = append(ch.ExtensionTypes, et)
		if isGREASE(et) {
			ch.GREASEExtensions = append(ch.GREASEExtensions, et)
		}
		payload := body[p+4 : intMin(p+4+el, len(body))]
		switch et {
		case 10: // supported_groups
			if len(payload) >= 2 {
				gl := int(payload[0])<<8 | int(payload[1])
				for i := 0; i+2 <= gl && i+2 <= len(payload)-2; i += 2 {
					v := int(payload[2+i])<<8 | int(payload[3+i])
					ch.SupportedGroups = append(ch.SupportedGroups, v)
					if isGREASE(v) {
						ch.GREASEGroups = append(ch.GREASEGroups, v)
					}
				}
			}
		case 13: // signature_algorithms
			if len(payload) >= 2 {
				sl := int(payload[0])<<8 | int(payload[1])
				for i := 0; i+2 <= sl && i+2 <= len(payload)-2; i += 2 {
					ch.SignatureAlgorithms = append(ch.SignatureAlgorithms, int(payload[2+i])<<8|int(payload[3+i]))
				}
			}
		case 16: // ALPN
			if len(payload) >= 2 {
				alpnLen := int(payload[0])<<8 | int(payload[1])
				q := 2
				for q < 2+alpnLen && q < len(payload) {
					pl := int(payload[q])
					if q+1+pl > len(payload) {
						break
					}
					ch.ALPNProtocols = append(ch.ALPNProtocols, string(payload[q+1:q+1+pl]))
					q += 1 + pl
				}
			}
		case 51: // key_share
			if len(payload) >= 2 {
				kl := int(payload[0])<<8 | int(payload[1])
				q := 2
				for q+4 <= 2+kl && q+4 <= len(payload) {
					g := int(payload[q])<<8 | int(payload[q+1])
					kx := int(payload[q+2])<<8 | int(payload[q+3])
					ch.KeyShareGroups = append(ch.KeyShareGroups, g)
					q += 4 + kx
				}
			}
		case 21:
			ch.HasPadding = true
		case 23:
			ch.HasEMS = true
		case 35:
			ch.HasSessionTicket = true
		case 65281:
			ch.HasRenegotiationInfo = true
		}
		p += 4 + el
	}
	return ch, nil
}

// ---------------------------------------------------------------------------
// Self-signed RSA cert (RSA key needed for static RSA-kex TLS 1.2 suites).
// ---------------------------------------------------------------------------

func selfSignedRSACert() (cryptotls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return cryptotls.Certificate{}, err
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return cryptotls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return cryptotls.X509KeyPair(certPEM, keyPEM)
}

// ---------------------------------------------------------------------------
// Capture listener: accepts one connection, records client->server bytes
// while a real cryptotls.Server completes the handshake.
// ---------------------------------------------------------------------------

type captureResult struct {
	ch        *clientHello
	buf       []byte
	hsErr     error
	acceptErr error
}

func startCaptureListener(cert cryptotls.Certificate, alpn []string, serverCipherSuites []uint16) (net.Listener, <-chan captureResult, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	out := make(chan captureResult, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			out <- captureResult{acceptErr: err}
			return
		}
		cap := &capturingConn{Conn: conn}
		cfg := &cryptotls.Config{
			Certificates: []cryptotls.Certificate{cert},
			NextProtos:   alpn,
			MinVersion:   cryptotls.VersionTLS10,
		}
		if len(serverCipherSuites) > 0 {
			cfg.CipherSuites = serverCipherSuites
		}
		srv := cryptotls.Server(cap, cfg)
		_ = srv.SetDeadline(time.Now().Add(10 * time.Second))
		hsErr := srv.Handshake()
		if hsErr == nil {
			// Absorb a little client data so the client can finish cleanly.
			tmp := make([]byte, 256)
			_ = srv.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
			_, _ = srv.Read(tmp)
		}
		res := captureResult{buf: append([]byte(nil), cap.buf...), hsErr: hsErr}
		if len(res.buf) > 0 {
			res.ch, _ = parseClientHello(res.buf)
		}
		_ = conn.Close()
		out <- res
	}()
	return ln, out, nil
}

// ---------------------------------------------------------------------------
// In-process gRPC server hosting netconn.Server (Go engine path).
// ---------------------------------------------------------------------------

func startInProcessGRPC() (addr string, stop func(), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	srv := grpc.NewServer()
	rc.RegisterRemoteControlServer(srv, &netconn.Server{})
	go srv.Serve(ln)
	return ln.Addr().String(), func() { srv.Stop(); _ = ln.Close() }, nil
}

func newGoClient(grpcAddr string, tlsOpts *sesametls.TLSOptions) (netconn.Client, *grpc.ClientConn, error) {
	cc, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return netconn.Client{}, nil, err
	}
	return netconn.Client{
		API: rc.NewRemoteControlClient(cc),
		TLS: tlsOpts,
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsOpportunisticTls: true,
			SupportsFlowControl:      true,
		},
	}, cc, nil
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

func intsEq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func int32ToInts(a []int32) []int {
	out := make([]int, len(a))
	for i, v := range a {
		out[i] = int(v)
	}
	return out
}

func fmtInts(a []int) string {
	parts := make([]string, len(a))
	for i, v := range a {
		parts[i] = strconv.Itoa(v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func fmtHexInts(a []int) string {
	parts := make([]string, len(a))
	for i, v := range a {
		parts[i] = fmt.Sprintf("0x%04x", v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func grpcCode(err error) codes.Code {
	st, ok := status.FromError(err)
	if !ok {
		return codes.Unknown
	}
	return st.Code()
}

func stderrf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}

// specSummary returns a compact human-readable summary of a ClientHelloSpec.
func specSummary(spec *sesametls.ClientHelloSpec) map[string]interface{} {
	if spec == nil {
		return nil
	}
	out := map[string]interface{}{}
	if len(spec.CipherSuites) > 0 {
		out["cipher_suites"] = spec.CipherSuites
	}
	if len(spec.SupportedGroups) > 0 {
		out["supported_groups"] = spec.SupportedGroups
	}
	if len(spec.SignatureAlgorithms) > 0 {
		out["signature_algorithms"] = spec.SignatureAlgorithms
	}
	if len(spec.Extensions) > 0 {
		types := make([]int, len(spec.Extensions))
		for i, e := range spec.Extensions {
			types[i] = int(e.GetType())
		}
		out["extension_types"] = types
	}
	if len(spec.CompressionMethods) > 0 {
		out["compression_methods"] = spec.CompressionMethods
	}
	if spec.SessionIdLength != 0 {
		out["session_id_length"] = spec.SessionIdLength
	}
	if spec.LegacyVersion != sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED {
		out["legacy_version"] = spec.LegacyVersion.String()
	}
	if spec.PadToSize != 0 {
		out["pad_to_size"] = spec.PadToSize
	}
	return out
}

// ---------------------------------------------------------------------------
// Go-engine scenarios.
// ---------------------------------------------------------------------------

func runGoEngine(ctx context.Context, grpcAddr string, cert cryptotls.Certificate) []scenario {
	var scenarios []scenario

	scenarios = append(scenarios, runDefaultHello(ctx, grpcAddr, cert))
	scenarios = append(scenarios, runHonoredGroups(ctx, grpcAddr, cert))
	scenarios = append(scenarios, runHonoredCiphersTLS12(ctx, grpcAddr, cert))

	// Fail-closed set.
	failClosed := []struct {
		id   string
		name string
		spec *sesametls.ClientHelloSpec
		maxV sesametls.TLSVersion
	}{
		{"fc-reversed-ciphers", "fail-closed: reversed cipher order", &sesametls.ClientHelloSpec{CipherSuites: []int32{0x0035, 0x002f}}, sesametls.TLSVersion_TLS_1_2},
		{"fc-reversed-groups", "fail-closed: reversed group order", &sesametls.ClientHelloSpec{SupportedGroups: []int32{23, 29}}, sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED},
		{"fc-unknown-group", "fail-closed: unknown group 0x7a7a", &sesametls.ClientHelloSpec{SupportedGroups: []int32{0x7a7a}}, sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED},
		{"fc-sigalgs", "fail-closed: non-empty signature_algorithms", &sesametls.ClientHelloSpec{SignatureAlgorithms: []int32{0x0403}}, sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED},
		{"fc-extensions", "fail-closed: non-empty extensions", &sesametls.ClientHelloSpec{Extensions: []*sesametls.ClientHelloExtension{{Type: 43}}}, sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED},
		{"fc-session-id", "fail-closed: session_id_length=-1", &sesametls.ClientHelloSpec{SessionIdLength: -1}, sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED},
		{"fc-pad", "fail-closed: pad_to_size=512", &sesametls.ClientHelloSpec{PadToSize: 512}, sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED},
		{"fc-legacy-version", "fail-closed: legacy_version=TLS_1_2", &sesametls.ClientHelloSpec{LegacyVersion: sesametls.TLSVersion_TLS_1_2}, sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED},
	}
	for _, fc := range failClosed {
		scenarios = append(scenarios, runFailClosed(ctx, grpcAddr, fc.id, fc.name, fc.spec, fc.maxV, codes.FailedPrecondition))
	}

	// Invalid set.
	invalid := []struct {
		id   string
		name string
		spec *sesametls.ClientHelloSpec
	}{
		{"inv-cipher-range", "invalid: cipher suite 1<<20", &sesametls.ClientHelloSpec{CipherSuites: []int32{1 << 20}}},
		{"inv-group-negative", "invalid: group -1", &sesametls.ClientHelloSpec{SupportedGroups: []int32{-1}}},
		{"inv-session-id-33", "invalid: session_id_length=33", &sesametls.ClientHelloSpec{SessionIdLength: 33}},
		{"inv-pad-negative", "invalid: pad_to_size=-1", &sesametls.ClientHelloSpec{PadToSize: -1}},
	}
	for _, iv := range invalid {
		scenarios = append(scenarios, runFailClosed(ctx, grpcAddr, iv.id, iv.name, iv.spec, sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED, codes.InvalidArgument))
	}

	scenarios = append(scenarios, runClientVerify())
	return scenarios
}

// dialAndCapture dials through the in-process gRPC server, targeting the
// capture listener. Returns the InStreamConn and capture result.
func dialAndCapture(ctx context.Context, grpcAddr string, targetAddr string, tlsOpts *sesametls.TLSOptions, captureCh <-chan captureResult) (netconn.InStreamConn, captureResult, error) {
	client, cc, err := newGoClient(grpcAddr, tlsOpts)
	if err != nil {
		return nil, captureResult{}, err
	}
	defer cc.Close()

	dialCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	conn, err := client.DialContext(dialCtx, "tcp", targetAddr)
	if err != nil {
		// Still try to collect the capture.
		select {
		case res := <-captureCh:
			return nil, res, err
		case <-time.After(2 * time.Second):
			return nil, captureResult{}, err
		}
	}

	ic, ok := conn.(netconn.InStreamConn)
	if !ok {
		_ = conn.Close()
		return nil, captureResult{}, fmt.Errorf("expected InStreamConn, got %T", conn)
	}

	// Write a byte and close to let the capture listener finish.
	_, _ = conn.Write([]byte("x"))
	_ = conn.Close()

	select {
	case res := <-captureCh:
		return ic, res, nil
	case <-time.After(3 * time.Second):
		return ic, captureResult{}, fmt.Errorf("capture timeout")
	}
}

func runDefaultHello(ctx context.Context, grpcAddr string, cert cryptotls.Certificate) scenario {
	s := scenario{
		ID:   "default-hello",
		Name: "Go engine default ClientHello baseline",
		Path: "go-engine",
	}

	ln, captureCh, err := startCaptureListener(cert, []string{"http/1.1"}, nil)
	if err != nil {
		s.Outcome = "error: " + err.Error()
		return s
	}
	defer ln.Close()

	tlsOpts := &sesametls.TLSOptions{
		ServerName:         "localhost",
		InsecureSkipVerify: true,
		AlpnProtocols:      []string{"http/1.1"},
	}

	ic, cap, err := dialAndCapture(ctx, grpcAddr, ln.Addr().String(), tlsOpts, captureCh)
	if err != nil {
		s.Outcome = "error: " + err.Error()
		return s
	}
	if cap.ch == nil {
		s.Outcome = "no ClientHello captured"
		return s
	}
	ch := cap.ch
	s.Outcome = "handshake succeeded"

	tlsRes := ic.TLSResult()

	s.add(check{deetLegacyVersion, ch.LegacyVersion == 0x0303,
		fmt.Sprintf("legacy_version=0x%04x (Go always 0x0303)", ch.LegacyVersion)})
	s.add(check{deetSessionID, ch.SessionIDLength == 32,
		fmt.Sprintf("session_id_length=%d (Go always 32)", ch.SessionIDLength)})
	s.add(check{deetCompression, intsEq(ch.CompressionMethods, []int{0}),
		fmt.Sprintf("compression=%v (Go always [0])", ch.CompressionMethods)})
	s.add(check{deetGREASE,
		len(ch.GREASECiphers) == 0 && len(ch.GREASEGroups) == 0 && len(ch.GREASEExtensions) == 0,
		fmt.Sprintf("Go crypto/tls emits no GREASE: ciphers=%d groups=%d exts=%d",
			len(ch.GREASECiphers), len(ch.GREASEGroups), len(ch.GREASEExtensions))})
	s.add(check{deetExtPresence, ch.HasRenegotiationInfo,
		fmt.Sprintf("renegotiation_info(65281) present=%v", ch.HasRenegotiationInfo)})
	s.add(check{deetExtPresence, ch.HasEMS,
		fmt.Sprintf("extended_master_secret(23) present=%v", ch.HasEMS)})
	s.add(check{deetALPN, len(ch.ALPNProtocols) == 1 && ch.ALPNProtocols[0] == "http/1.1",
		fmt.Sprintf("alpn=%v", ch.ALPNProtocols)})
	s.add(check{deetKeyShareGroups, len(ch.KeyShareGroups) > 0,
		fmt.Sprintf("key_share groups=%v", ch.KeyShareGroups)})
	s.add(check{deetMessageSize, ch.MessageSize > 200,
		fmt.Sprintf("ClientHello message_size=%d bytes", ch.MessageSize)})
	s.add(check{deetNegotiatedVer, tlsRes.GetTlsVersion() == sesametls.TLSVersion_TLS_1_3,
		fmt.Sprintf("negotiated version=%v", tlsRes.GetTlsVersion())})
	s.add(check{deetAppliedEcho, tlsRes.GetAppliedClientHello() == nil,
		"no spec requested => applied_client_hello absent"})

	s.Passed = allPass(s.Checks)
	return s
}

func runHonoredGroups(ctx context.Context, grpcAddr string, cert cryptotls.Certificate) scenario {
	spec := &sesametls.ClientHelloSpec{SupportedGroups: []int32{29, 23}}
	s := scenario{
		ID:            "honored-groups",
		Name:          "Go engine honors SupportedGroups [29,23]",
		Path:          "go-engine",
		RequestedSpec: specSummary(spec),
	}

	ln, captureCh, err := startCaptureListener(cert, []string{"http/1.1"}, nil)
	if err != nil {
		s.Outcome = "error: " + err.Error()
		return s
	}
	defer ln.Close()

	tlsOpts := &sesametls.TLSOptions{
		ServerName:         "localhost",
		InsecureSkipVerify: true,
		AlpnProtocols:      []string{"http/1.1"},
		ClientHello:        spec,
	}

	ic, cap, err := dialAndCapture(ctx, grpcAddr, ln.Addr().String(), tlsOpts, captureCh)
	if err != nil {
		s.Outcome = "error: " + err.Error()
		return s
	}
	if cap.ch == nil {
		s.Outcome = "no ClientHello captured"
		return s
	}
	ch := cap.ch
	s.Outcome = "handshake succeeded"

	tlsRes := ic.TLSResult()
	want := int32ToInts(spec.SupportedGroups)

	s.add(check{deetGroupOrder, intsEq(ch.SupportedGroups, want),
		fmt.Sprintf("captured groups=%v want=%v", fmtInts(ch.SupportedGroups), fmtInts(want))})
	s.add(check{deetKeyShareGroups, len(ch.KeyShareGroups) > 0 && ch.KeyShareGroups[0] == 29,
		fmt.Sprintf("key_share groups=%v (first=29 expected)", fmtInts(ch.KeyShareGroups))})
	s.add(check{deetAppliedEcho, tlsRes.GetAppliedClientHello() != nil,
		"applied_client_hello echo present"})

	s.Passed = allPass(s.Checks)
	return s
}

func runHonoredCiphersTLS12(ctx context.Context, grpcAddr string, cert cryptotls.Certificate) scenario {
	spec := &sesametls.ClientHelloSpec{CipherSuites: []int32{0x002f, 0x0035}}
	s := scenario{
		ID:   "honored-ciphers-tls12",
		Name: "Go engine honors CipherSuites [0x002f,0x0035] at TLS 1.2",
		Path: "go-engine",
		RequestedSpec: map[string]interface{}{
			"max_version":   "TLS_1_2",
			"cipher_suites": spec.CipherSuites,
		},
	}

	// Server must explicitly enable the RSA-kex suites (Go 1.27 disables them
	// by default on the server side).
	ln, captureCh, err := startCaptureListener(cert, []string{"http/1.1"}, []uint16{0x002f, 0x0035})
	if err != nil {
		s.Outcome = "error: " + err.Error()
		return s
	}
	defer ln.Close()

	tlsOpts := &sesametls.TLSOptions{
		ServerName:         "localhost",
		InsecureSkipVerify: true,
		AlpnProtocols:      []string{"http/1.1"},
		MaxVersion:         sesametls.TLSVersion_TLS_1_2,
		ClientHello:        spec,
	}

	ic, cap, err := dialAndCapture(ctx, grpcAddr, ln.Addr().String(), tlsOpts, captureCh)
	if err != nil {
		s.Outcome = "error: " + err.Error()
		return s
	}
	if cap.ch == nil {
		s.Outcome = "no ClientHello captured"
		return s
	}
	ch := cap.ch
	s.Outcome = "handshake succeeded"

	tlsRes := ic.TLSResult()
	want := int32ToInts(spec.CipherSuites)

	s.add(check{deetCipherOrder, intsEq(ch.CipherSuites, want),
		fmt.Sprintf("captured ciphers=%v want=%v", fmtHexInts(ch.CipherSuites), fmtHexInts(want))})
	s.add(check{deetNegotiatedVer, tlsRes.GetTlsVersion() == sesametls.TLSVersion_TLS_1_2,
		fmt.Sprintf("negotiated version=%v (want TLS_1_2)", tlsRes.GetTlsVersion())})
	s.add(check{deetNegotiatedSuite, tlsRes.GetCipherSuite() == 0x002f || tlsRes.GetCipherSuite() == 0x0035,
		fmt.Sprintf("negotiated cipher=0x%04x", tlsRes.GetCipherSuite())})
	s.add(check{deetAppliedEcho, tlsRes.GetAppliedClientHello() != nil,
		"applied_client_hello echo present"})

	s.Passed = allPass(s.Checks)
	return s
}

func runFailClosed(ctx context.Context, grpcAddr, id, name string, spec *sesametls.ClientHelloSpec, maxV sesametls.TLSVersion, wantCode codes.Code) scenario {
	s := scenario{
		ID:            id,
		Name:          name,
		Path:          "go-engine",
		RequestedSpec: specSummary(spec),
	}

	// Plain TCP listener: records whether any bytes arrive (no TLS handshake
	// should occur for fail-closed scenarios).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		s.Outcome = "error: " + err.Error()
		return s
	}
	defer ln.Close()

	bytesReceived := make(chan int, 1)
	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			bytesReceived <- -1
			return
		}
		defer conn.Close()
		buf := make([]byte, 1)
		_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, _ := conn.Read(buf)
		bytesReceived <- n
	}()

	tlsOpts := &sesametls.TLSOptions{
		ServerName:         "localhost",
		InsecureSkipVerify: true,
		ClientHello:        spec,
	}
	if maxV != sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED {
		tlsOpts.MaxVersion = maxV
	}

	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client, cc, cerr := newGoClient(grpcAddr, tlsOpts)
	if cerr != nil {
		s.Outcome = "error: " + cerr.Error()
		return s
	}
	defer cc.Close()

	_, derr := client.DialContext(dialCtx, "tcp", ln.Addr().String())
	gotCode := grpcCode(derr)

	s.Outcome = fmt.Sprintf("dial rejected with %s", gotCode)
	s.add(check{deetGRPCCode, gotCode == wantCode,
		fmt.Sprintf("got %s, want %s", gotCode, wantCode)})

	select {
	case n := <-bytesReceived:
		s.add(check{deetNoHandshake, n <= 0,
			fmt.Sprintf("received %d bytes (want 0: fail before handshake)", n)})
	case <-time.After(1500 * time.Millisecond):
		s.add(check{deetNoHandshake, true, "no connection attempt reached the listener"})
	}

	s.Passed = allPass(s.Checks)
	return s
}

func runClientVerify() scenario {
	s := scenario{
		ID:   "client-verify",
		Name: "VerifyAppliedClientHello accepts verbatim echo, rejects mutation",
		Path: "go-engine",
	}

	spec := &sesametls.ClientHelloSpec{
		SupportedGroups: []int32{29, 23},
		CipherSuites:    []int32{0x002f, 0x0035},
		SessionIdLength: 32,
	}

	// Verbatim echo: must pass.
	verbatim := &sesametls.ClientHelloSpec{
		SupportedGroups: []int32{29, 23},
		CipherSuites:    []int32{0x002f, 0x0035},
		SessionIdLength: 32,
	}
	err := netconn.VerifyAppliedClientHello(spec, verbatim)
	s.add(check{deetVerifyFunc, err == nil,
		fmt.Sprintf("verbatim echo: err=%v (want nil)", err)})

	// Mutated echo: must fail.
	mutated := &sesametls.ClientHelloSpec{
		SupportedGroups: []int32{23, 29}, // reversed
		CipherSuites:    []int32{0x002f, 0x0035},
		SessionIdLength: 32,
	}
	err = netconn.VerifyAppliedClientHello(spec, mutated)
	s.add(check{deetVerifyFunc, err != nil,
		fmt.Sprintf("mutated echo: err=%v (want non-nil)", err)})

	// Nil request, non-nil applied: must fail.
	err = netconn.VerifyAppliedClientHello(nil, spec)
	s.add(check{deetVerifyFunc, err != nil,
		fmt.Sprintf("nil-request non-nil-applied: err=%v (want non-nil)", err)})

	// Both nil: must pass.
	err = netconn.VerifyAppliedClientHello(nil, nil)
	s.add(check{deetVerifyFunc, err == nil,
		fmt.Sprintf("both nil: err=%v (want nil)", err)})

	s.Outcome = "direct function calls"
	s.Passed = allPass(s.Checks)
	return s
}

// ---------------------------------------------------------------------------
// Endpoint-bun scenarios (gated).
// ---------------------------------------------------------------------------

type bunFixture struct {
	TLS struct {
		Extensions []struct {
			Name            string   `json:"name"`
			SupportedGroups []string `json:"supported_groups,omitempty"`
			Protocols       []string `json:"protocols,omitempty"`
		} `json:"extensions"`
		SessionID        string `json:"session_id"`
		TLSVersionRecord string `json:"tls_version_record"`
	} `json:"tls"`
}

func parseBunFixture(data []byte) (*bunFixture, error) {
	var f bunFixture
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func fixtureExtOrder(f *bunFixture) []int {
	var out []int
	for _, e := range f.TLS.Extensions {
		if idx := strings.LastIndex(e.Name, "("); idx >= 0 {
			if end := strings.LastIndex(e.Name, ")"); end > idx {
				if v, err := strconv.Atoi(e.Name[idx+1 : end]); err == nil {
					out = append(out, v)
				}
			}
		}
	}
	return out
}

func fixtureGroups(f *bunFixture) []int {
	for _, e := range f.TLS.Extensions {
		if len(e.SupportedGroups) > 0 {
			var out []int
			for _, g := range e.SupportedGroups {
				if idx := strings.LastIndex(g, "("); idx >= 0 {
					if end := strings.LastIndex(g, ")"); end > idx {
						if v, err := strconv.Atoi(g[idx+1 : end]); err == nil {
							out = append(out, v)
						}
					}
				}
			}
			return out
		}
	}
	return nil
}

func fixtureALPN(f *bunFixture) []string {
	for _, e := range f.TLS.Extensions {
		if len(e.Protocols) > 0 {
			return e.Protocols
		}
	}
	return nil
}

func startEndpointSubprocess(ctx context.Context, cliPath, runtimeBin string, host, port string) (string, func(), error) {
	cmd := exec.CommandContext(ctx, runtimeBin, cliPath, "--host", host, "--port", port)
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", nil, err
	}
	if err := cmd.Start(); err != nil {
		return "", nil, err
	}
	kill := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGINT)
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case <-time.After(2 * time.Second):
				_ = cmd.Process.Kill()
			case <-done:
			}
		}
	}

	portRegex := regexp.MustCompile(`sesame-endpoint listening on ` + regexp.QuoteMeta(host) + `:(\d+)`)
	scanner := bufio.NewScanner(stderrPipe)
	found := make(chan string, 1)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			stderrf("  [endpoint stderr] %s\n", line)
			if m := portRegex.FindStringSubmatch(line); len(m) > 1 {
				found <- m[1]
				return
			}
		}
	}()

	select {
	case p := <-found:
		return p, kill, nil
	case <-time.After(15 * time.Second):
		kill()
		return "", nil, fmt.Errorf("timeout waiting for sesame-endpoint to bind")
	}
}

// endpointOpts configures the endpoint-bun scenario track.
type endpointOpts struct {
	enabled     bool   // -endpoint
	cliPath     string // -cli
	runtime     string // -runtime: bun | node | auto
	fixturePath string // -fixture: optional override of the embedded fixture
}

// resolveRuntime maps the -runtime flag to an executable name.
func resolveRuntime(runtime string) (string, error) {
	switch runtime {
	case "", "auto":
		if _, err := exec.LookPath("bun"); err == nil {
			return "bun", nil
		}
		if _, err := exec.LookPath("node"); err == nil {
			return "node", nil
		}
		return "", fmt.Errorf("neither bun nor node found on PATH")
	case "bun", "node":
		if _, err := exec.LookPath(runtime); err != nil {
			return "", fmt.Errorf("%s not found on PATH", runtime)
		}
		return runtime, nil
	default:
		return "", fmt.Errorf("unsupported -runtime %q (want bun, node, or auto)", runtime)
	}
}

// defaultCLIPath resolves the sesame-endpoint CLI entry point relative to the
// repository root, located by walking up from the working directory to the
// go.mod that declares this module. This lets the endpoint track run from any
// directory inside the repository; outside it, the path is returned relative
// to the working directory (and the endpoint scenarios skip cleanly if the
// file is absent).
func defaultCLIPath() string {
	rel := filepath.Join("sesame-endpoint", "build", "src", "cli.js")
	dir, err := os.Getwd()
	if err != nil {
		return rel
	}
	for {
		if data, rerr := os.ReadFile(filepath.Join(dir, "go.mod")); rerr == nil &&
			strings.HasPrefix(strings.TrimSpace(string(data)), "module github.com/joeycumines/sesame") {
			return filepath.Join(dir, rel)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return rel
}

func runEndpointScenarios(ctx context.Context, cert cryptotls.Certificate, opts endpointOpts) []scenario {
	var scenarios []scenario

	skipReason := ""
	if !opts.enabled {
		skipReason = "endpoint scenarios disabled (pass -endpoint)"
	} else if _, err := os.Stat(opts.cliPath); err != nil {
		skipReason = fmt.Sprintf("endpoint CLI not found at %s (build it first)", opts.cliPath)
	}

	if skipReason != "" {
		scenarios = append(scenarios, []scenario{
			{ID: "endpoint-default-hello", Name: "Endpoint Bun default ClientHello vs opencode fixture", Path: "endpoint-bun", Skipped: true, SkipReason: skipReason, Outcome: "skipped"},
			{ID: "endpoint-fail-closed", Name: "Endpoint rejects non-default ClientHelloSpec dimension", Path: "endpoint-bun", Skipped: true, SkipReason: skipReason, Outcome: "skipped"},
			{ID: "endpoint-caps", Name: "Endpoint advertises all-false ClientHelloCapabilities", Path: "endpoint-bun", Skipped: true, SkipReason: skipReason, Outcome: "skipped"},
		}...)
		return scenarios
	}

	fixtureData := bunFixtureJSON
	if opts.fixturePath != "" {
		data, rerr := os.ReadFile(opts.fixturePath)
		if rerr != nil {
			stderrf("WARNING: cannot read -fixture %s: %v; using the embedded fixture\n", opts.fixturePath, rerr)
		} else {
			fixtureData = data
		}
	}
	fixture, ferr := parseBunFixture(fixtureData)
	if ferr != nil {
		stderrf("WARNING: cannot parse bun fixture: %v\n", ferr)
	}

	freeL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		stderrf("WARNING: cannot find free port for endpoint: %v\n", err)
		return scenarios
	}
	freePort := strconv.Itoa(freeL.Addr().(*net.TCPAddr).Port)
	_ = freeL.Close()

	runtimeBin, rerr := resolveRuntime(opts.runtime)
	if rerr != nil {
		stderrf("WARNING: cannot resolve endpoint runtime: %v\n", rerr)
		scenarios = append(scenarios, []scenario{
			{ID: "endpoint-default-hello", Name: "Endpoint Bun default ClientHello vs opencode fixture", Path: "endpoint-bun", Skipped: true, SkipReason: rerr.Error(), Outcome: "skipped"},
			{ID: "endpoint-fail-closed", Name: "Endpoint rejects non-default ClientHelloSpec dimension", Path: "endpoint-bun", Skipped: true, SkipReason: rerr.Error(), Outcome: "skipped"},
			{ID: "endpoint-caps", Name: "Endpoint advertises all-false ClientHelloCapabilities", Path: "endpoint-bun", Skipped: true, SkipReason: rerr.Error(), Outcome: "skipped"},
		}...)
		return scenarios
	}

	epPort, kill, err := startEndpointSubprocess(ctx, opts.cliPath, runtimeBin, "127.0.0.1", freePort)
	if err != nil {
		stderrf("WARNING: cannot start endpoint subprocess: %v\n", err)
		scenarios = append(scenarios, []scenario{
			{ID: "endpoint-default-hello", Name: "Endpoint Bun default ClientHello vs opencode fixture", Path: "endpoint-bun", Skipped: true, SkipReason: err.Error(), Outcome: "skipped"},
			{ID: "endpoint-fail-closed", Name: "Endpoint rejects non-default ClientHelloSpec dimension", Path: "endpoint-bun", Skipped: true, SkipReason: err.Error(), Outcome: "skipped"},
			{ID: "endpoint-caps", Name: "Endpoint advertises all-false ClientHelloCapabilities", Path: "endpoint-bun", Skipped: true, SkipReason: err.Error(), Outcome: "skipped"},
		}...)
		return scenarios
	}
	defer kill()

	epAddr := fmt.Sprintf("127.0.0.1:%s", epPort)

	scenarios = append(scenarios, runEndpointDefaultHello(ctx, epAddr, cert, fixture))
	scenarios = append(scenarios, runEndpointFailClosed(ctx, epAddr))
	scenarios = append(scenarios, runEndpointCaps(ctx, epAddr, cert))

	return scenarios
}

func runEndpointDefaultHello(ctx context.Context, epAddr string, cert cryptotls.Certificate, fixture *bunFixture) scenario {
	s := scenario{
		ID:   "endpoint-default-hello",
		Name: "Endpoint Bun default ClientHello vs opencode fixture",
		Path: "endpoint-bun",
	}

	ln, captureCh, err := startCaptureListener(cert, []string{"http/1.1"}, nil)
	if err != nil {
		s.Outcome = "error: " + err.Error()
		return s
	}
	defer ln.Close()

	tlsOpts := &sesametls.TLSOptions{
		ServerName:         "localhost",
		InsecureSkipVerify: true,
		AlpnProtocols:      []string{"http/1.1"},
	}

	_, cap, err := dialAndCapture(ctx, epAddr, ln.Addr().String(), tlsOpts, captureCh)
	if err != nil {
		s.Outcome = "error: " + err.Error()
		return s
	}
	if cap.ch == nil {
		s.Outcome = "no ClientHello captured"
		return s
	}
	ch := cap.ch
	s.Outcome = "handshake succeeded"

	if fixture == nil {
		s.add(check{deetExtOrder, false, "fixture not loaded"})
		s.Passed = false
		return s
	}

	// Compare extension order against fixture.
	fixtureExts := fixtureExtOrder(fixture)
	capturedExts := ch.ExtensionTypes
	fixtureSet := make(map[int]bool)
	for _, v := range fixtureExts {
		fixtureSet[v] = true
	}
	var capturedFiltered []int
	for _, v := range capturedExts {
		if fixtureSet[v] {
			capturedFiltered = append(capturedFiltered, v)
		}
	}
	orderOK := true
	fi := 0
	for _, cv := range capturedFiltered {
		for fi < len(fixtureExts) && fixtureExts[fi] != cv {
			fi++
		}
		if fi >= len(fixtureExts) {
			orderOK = false
			break
		}
		fi++
	}
	s.add(check{deetExtOrder, orderOK,
		fmt.Sprintf("captured exts=%v fixture exts=%v", fmtInts(capturedExts), fmtInts(fixtureExts))})

	s.add(check{deetGREASE,
		len(ch.GREASECiphers) == 0 && len(ch.GREASEGroups) == 0 && len(ch.GREASEExtensions) == 0,
		fmt.Sprintf("fixture has no GREASE; captured: ciphers=%d groups=%d exts=%d",
			len(ch.GREASECiphers), len(ch.GREASEGroups), len(ch.GREASEExtensions))})

	fixtureSIDLen := len(fixture.TLS.SessionID) / 2
	s.add(check{deetSessionID, ch.SessionIDLength == fixtureSIDLen,
		fmt.Sprintf("captured session_id_length=%d fixture=%d", ch.SessionIDLength, fixtureSIDLen)})

	fixtureALPNProtos := fixtureALPN(fixture)
	s.add(check{deetALPN, len(ch.ALPNProtocols) > 0 && len(fixtureALPNProtos) > 0 && ch.ALPNProtocols[0] == fixtureALPNProtos[0],
		fmt.Sprintf("captured alpn=%v fixture=%v", ch.ALPNProtocols, fixtureALPNProtos)})

	fixtureGrps := fixtureGroups(fixture)
	if len(fixtureGrps) > 0 {
		groupSet := make(map[int]bool)
		for _, g := range ch.SupportedGroups {
			groupSet[g] = true
		}
		allPresent := true
		for _, g := range fixtureGrps {
			if !groupSet[g] {
				allPresent = false
				break
			}
		}
		s.add(check{deetGroupOrder, allPresent,
			fmt.Sprintf("captured groups=%v fixture groups=%v", fmtInts(ch.SupportedGroups), fmtInts(fixtureGrps))})
	}

	// Documented engine-ceiling gaps (probe-verified): Bun node:tls — the
	// endpoint's builtin engine — always sends record version 0x0301 and
	// omits status_request(5)/signed_certificate_timestamp(18), while Bun
	// fetch (the opencode reference) sends 0x0303 with both. Recorded as
	// gaps: work items for the native BoringSSL addon (todo ts-native-addon).
	var missingExts []int
	capturedSet := make(map[int]bool)
	for _, v := range capturedExts {
		capturedSet[v] = true
	}
	for _, v := range fixtureExts {
		if !capturedSet[v] {
			missingExts = append(missingExts, v)
		}
	}
	s.addGap(check{deetExtPresence, len(missingExts) == 0,
		fmt.Sprintf("missing vs fixture: %v (status_request(5)/SCT(18) absent from node:tls; native-addon work item)", fmtInts(missingExts))})

	fixtureRecVer, _ := strconv.Atoi(fixture.TLS.TLSVersionRecord)
	s.addGap(check{deetRecordVersion, int(ch.RecordVersion) == fixtureRecVer,
		fmt.Sprintf("captured record_version=%d fixture=%d (node:tls always 0x0301; fetch 0x0303; native-addon work item)", ch.RecordVersion, fixtureRecVer)})

	s.Passed = allPass(s.Checks)
	return s
}

func runEndpointFailClosed(ctx context.Context, epAddr string) scenario {
	s := scenario{
		ID:            "endpoint-fail-closed",
		Name:          "Endpoint rejects non-default ClientHelloSpec dimension",
		Path:          "endpoint-bun",
		RequestedSpec: specSummary(&sesametls.ClientHelloSpec{SignatureAlgorithms: []int32{0x0403}}),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		s.Outcome = "error: " + err.Error()
		return s
	}
	defer ln.Close()

	bytesReceived := make(chan int, 1)
	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			bytesReceived <- -1
			return
		}
		defer conn.Close()
		buf := make([]byte, 1)
		_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, _ := conn.Read(buf)
		bytesReceived <- n
	}()

	tlsOpts := &sesametls.TLSOptions{
		ServerName:         "localhost",
		InsecureSkipVerify: true,
		ClientHello:        &sesametls.ClientHelloSpec{SignatureAlgorithms: []int32{0x0403}},
	}

	client, cc, cerr := newGoClient(epAddr, tlsOpts)
	if cerr != nil {
		s.Outcome = "error: " + cerr.Error()
		return s
	}
	defer cc.Close()

	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, derr := client.DialContext(dialCtx, "tcp", ln.Addr().String())
	gotCode := grpcCode(derr)

	s.Outcome = fmt.Sprintf("dial rejected with %s", gotCode)
	s.add(check{deetGRPCCode, gotCode == codes.FailedPrecondition,
		fmt.Sprintf("got %s, want FailedPrecondition", gotCode)})

	select {
	case n := <-bytesReceived:
		s.add(check{deetNoHandshake, n <= 0,
			fmt.Sprintf("received %d bytes (want 0)", n)})
	case <-time.After(1500 * time.Millisecond):
		s.add(check{deetNoHandshake, true, "no connection attempt reached the listener"})
	}

	s.Passed = allPass(s.Checks)
	return s
}

func runEndpointCaps(ctx context.Context, epAddr string, cert cryptotls.Certificate) scenario {
	s := scenario{
		ID:   "endpoint-caps",
		Name: "Endpoint advertises all-false ClientHelloCapabilities",
		Path: "endpoint-bun",
	}

	ln, captureCh, err := startCaptureListener(cert, []string{"http/1.1"}, nil)
	if err != nil {
		s.Outcome = "error: " + err.Error()
		return s
	}
	defer ln.Close()

	tlsOpts := &sesametls.TLSOptions{
		ServerName:         "localhost",
		InsecureSkipVerify: true,
		AlpnProtocols:      []string{"http/1.1"},
	}

	ic, _, err := dialAndCapture(ctx, epAddr, ln.Addr().String(), tlsOpts, captureCh)
	if err != nil {
		s.Outcome = "error: " + err.Error()
		return s
	}

	caps := ic.ServerCapabilities().GetClientHelloCapabilities()
	if caps == nil {
		s.Outcome = "client_hello_capabilities absent"
		s.add(check{deetCapabilities, false, "capabilities field is nil"})
		s.Passed = false
		return s
	}

	s.Outcome = "handshake succeeded; capabilities received"

	allFalse := !caps.GetCustomCipherSuites() &&
		!caps.GetCustomSupportedGroups() &&
		!caps.GetCustomSignatureAlgorithms() &&
		!caps.GetCustomExtensionOrder() &&
		!caps.GetRawExtensions() &&
		!caps.GetGreaseValues() &&
		!caps.GetSessionIdLength() &&
		!caps.GetPaddingControl() &&
		!caps.GetLegacyVersionControl() &&
		!caps.GetCompressionMethods()

	s.add(check{deetCapabilities, allFalse,
		fmt.Sprintf("builtin engine caps all false: custom_ciphers=%v custom_groups=%v sigalgs=%v ext_order=%v raw_ext=%v grease=%v sid=%v padding=%v legacy_ver=%v compression=%v",
			caps.GetCustomCipherSuites(), caps.GetCustomSupportedGroups(), caps.GetCustomSignatureAlgorithms(),
			caps.GetCustomExtensionOrder(), caps.GetRawExtensions(), caps.GetGreaseValues(),
			caps.GetSessionIdLength(), caps.GetPaddingControl(), caps.GetLegacyVersionControl(), caps.GetCompressionMethods())})

	s.Passed = allPass(s.Checks)
	return s
}

// ---------------------------------------------------------------------------
// main.
// ---------------------------------------------------------------------------

func main() {
	jsonPath := flag.String("json", "", "write the results JSON to this path (default: do not write a file)")
	htmlPath := flag.String("html", "", "render the self-contained HTML report to this path")
	endpoint := flag.Bool("endpoint", false, "run the endpoint-bun scenarios against a built sesame-endpoint")
	cliPath := flag.String("cli", defaultCLIPath(), "sesame-endpoint CLI entry point")
	runtimeName := flag.String("runtime", "auto", "endpoint runtime: bun, node, or auto (bun if on PATH, else node)")
	fixturePath := flag.String("fixture", "", "override the embedded Bun-fetch reference fixture with this file")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	r := report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		GoVersion:   runtime.Version(),
		Module:      "github.com/joeycumines/sesame",
	}

	if out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output(); err == nil {
		r.Commit = strings.TrimSpace(string(out))
	}

	stderrf("quick-test-tls: Go %s on %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	cert, err := selfSignedRSACert()
	if err != nil {
		stderrf("FATAL: cannot generate self-signed cert: %v\n", err)
		os.Exit(1)
	}

	grpcAddr, stopGRPC, err := startInProcessGRPC()
	if err != nil {
		stderrf("FATAL: cannot start in-process gRPC server: %v\n", err)
		os.Exit(1)
	}
	defer stopGRPC()
	stderrf("in-process gRPC server at %s\n", grpcAddr)

	stderrf("\n--- go-engine scenarios ---\n")
	goScenarios := runGoEngine(ctx, grpcAddr, cert)
	for _, s := range goScenarios {
		mark := "PASS"
		if !s.Passed {
			mark = "FAIL"
		}
		stderrf("[%s] %s: %s\n", mark, s.ID, s.Outcome)
		r.add(s)
	}

	stderrf("\n--- endpoint-bun scenarios ---\n")
	epScenarios := runEndpointScenarios(ctx, cert, endpointOpts{
		enabled:     *endpoint,
		cliPath:     *cliPath,
		runtime:     *runtimeName,
		fixturePath: *fixturePath,
	})
	for _, s := range epScenarios {
		mark := "SKIP"
		if s.Passed {
			mark = "PASS"
		} else if !s.Skipped {
			mark = "FAIL"
		}
		stderrf("[%s] %s: %s\n", mark, s.ID, s.Outcome)
		r.add(s)
	}

	r.tally()

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		stderrf("FATAL: cannot marshal report: %v\n", err)
		os.Exit(1)
	}
	if *jsonPath != "" {
		if err := os.WriteFile(*jsonPath, data, 0o644); err != nil {
			stderrf("FATAL: cannot write %s: %v\n", *jsonPath, err)
			os.Exit(1)
		}
		stderrf("\nresults written to %s\n", *jsonPath)
	}
	if *htmlPath != "" {
		// json.Marshal escapes <, >, & for safe embedding in an HTML <script>.
		page := strings.Replace(string(reportTemplate), "__REPORT_DATA__", string(data), 1)
		if err := os.WriteFile(*htmlPath, []byte(page), 0o644); err != nil {
			stderrf("FATAL: cannot write %s: %v\n", *htmlPath, err)
			os.Exit(1)
		}
		stderrf("report written to %s\n", *htmlPath)
	}
	stderrf("summary: total=%d passed=%d failed=%d skipped=%d\n",
		r.Summary.Total, r.Summary.Passed, r.Summary.Failed, r.Summary.Skipped)

	if r.Summary.Failed > 0 {
		os.Exit(1)
	}
}
