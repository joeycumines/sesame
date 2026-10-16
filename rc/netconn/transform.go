package netconn

import (
	"bufio"
	"context"
	cryptotls "crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	sesamepb "github.com/joeycumines/sesame/sesamepb"
	typepb "github.com/joeycumines/sesame/typepb"
	xproxy "golang.org/x/net/proxy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type (
	// TLSProvider allows plugging in an alternative TLS implementation, such as uTLS
	// for client cryptographic fingerprint impersonation or custom handshake engines.
	TLSProvider interface {
		Handshake(ctx context.Context, rawConn net.Conn, opts *sesamepb.TlsOptions) (net.Conn, *sesamepb.TlsHandshakeResult, error)
	}

	// ConnTransformResult provides metadata about negotiated TLS and proxy state.
	ConnTransformResult interface {
		TlsHandshakeResult() *sesamepb.TlsHandshakeResult
		ProxyResult() *sesamepb.ProxyResult
	}
)

// ProtoToTLSVersion maps a protobuf TlsVersion to the crypto/tls version constant.
func ProtoToTLSVersion(v sesamepb.TlsVersion) uint16 {
	switch v {
	case sesamepb.TlsVersion_TLS_VERSION_TLS_1_0:
		return cryptotls.VersionTLS10
	case sesamepb.TlsVersion_TLS_VERSION_TLS_1_1:
		return cryptotls.VersionTLS11
	case sesamepb.TlsVersion_TLS_VERSION_TLS_1_2:
		return cryptotls.VersionTLS12
	case sesamepb.TlsVersion_TLS_VERSION_TLS_1_3:
		return cryptotls.VersionTLS13
	default:
		return 0
	}
}

// TLSVersionToProto maps a crypto/tls version constant to the protobuf TlsVersion.
func TLSVersionToProto(v uint16) sesamepb.TlsVersion {
	switch v {
	case cryptotls.VersionTLS10:
		return sesamepb.TlsVersion_TLS_VERSION_TLS_1_0
	case cryptotls.VersionTLS11:
		return sesamepb.TlsVersion_TLS_VERSION_TLS_1_1
	case cryptotls.VersionTLS12:
		return sesamepb.TlsVersion_TLS_VERSION_TLS_1_2
	case cryptotls.VersionTLS13:
		return sesamepb.TlsVersion_TLS_VERSION_TLS_1_3
	default:
		return sesamepb.TlsVersion_TLS_VERSION_UNSPECIFIED
	}
}

// BuildTLSConfig constructs a crypto/tls.Config from a sesamepb.TlsOptions message.
//
// opts are REQUIRED: nil is an explicit error, for the same reason as
// ExecuteTLSHandshake - a silent (nil, nil) return let callers proceed
// with no TLS configuration and no error.
func BuildTLSConfig(opts *sesamepb.TlsOptions, defaultServerName string) (*cryptotls.Config, error) {
	if opts == nil {
		return nil, status.Error(codes.InvalidArgument, "sesame/rc/netconn: tls options required")
	}

	serverName := opts.GetSni()
	if serverName == "" {
		serverName = defaultServerName
	}
	// A colon-bearing server_name that is not a bare IPv6 literal must
	// be a parseable host:port (SNI is a DNS hostname; the port is
	// stripped). Anything else is malformed client input, rejected with
	// INVALID_ARGUMENT rather than sent on the wire, mirroring the
	// TypeScript reference.
	if strings.Contains(serverName, ":") && net.ParseIP(serverName) == nil {
		if err := validateHostPort(serverName, "server_name"); err != nil {
			return nil, err
		}
		host, _, _ := net.SplitHostPort(serverName)
		serverName = host
	}

	cfg := &cryptotls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: opts.GetInsecureSkipVerify(),
	}

	// Standing Directive: If alpn_protocols is empty, ALPN extension MUST NOT be sent.
	if len(opts.GetAlpnProtocols()) > 0 {
		cfg.NextProtos = opts.GetAlpnProtocols()
	}

	if v := ProtoToTLSVersion(opts.GetMinVersion()); v != 0 {
		cfg.MinVersion = v
	}
	if v := ProtoToTLSVersion(opts.GetMaxVersion()); v != 0 {
		cfg.MaxVersion = v
	}

	// A version floor above its ceiling can never negotiate; reject it at
	// request validation time rather than surfacing an opaque handshake
	// failure. Unspecified (0) means "runtime default" and never conflicts.
	if minV, maxV := opts.GetMinVersion(), opts.GetMaxVersion(); minV != 0 && maxV != 0 && minV > maxV {
		return nil, status.Error(codes.InvalidArgument, "sesame/rc/netconn: min_version exceeds max_version")
	}

	if len(opts.GetCaCertificates()) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(opts.GetCaCertificates()) {
			return nil, status.Error(codes.InvalidArgument, "sesame/rc/netconn: invalid CA certificates PEM data")
		}
		cfg.RootCAs = pool
	}

	if len(opts.GetClientCertificate()) > 0 && len(opts.GetClientPrivateKey()) > 0 {
		cert, err := cryptotls.X509KeyPair(opts.GetClientCertificate(), opts.GetClientPrivateKey())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: invalid client certificate/key pair: %v", err)
		}
		cfg.Certificates = []cryptotls.Certificate{cert}
	}

	return cfg, nil
}

// ExtractTLSHandshakeResult extracts session metadata from a completed TLS connection.
func ExtractTLSHandshakeResult(state cryptotls.ConnectionState, appliedSpec *sesamepb.ClientHelloSpec) *sesamepb.TlsHandshakeResult {
	res := &sesamepb.TlsHandshakeResult{
		NegotiatedProtocol: state.NegotiatedProtocol,
		CipherSuite:        int32(state.CipherSuite),
		TlsVersion:         TLSVersionToProto(state.Version),
		Sni:                state.ServerName,
		AppliedClientHello: appliedSpec,
	}

	if len(state.PeerCertificates) > 0 {
		res.PeerCertificates = make([][]byte, len(state.PeerCertificates))
		for i, cert := range state.PeerCertificates {
			res.PeerCertificates[i] = cert.Raw
		}
	}

	return res
}

// ExecuteTLSHandshake wraps an existing connection with TLS using either a custom TLSProvider or standard crypto/tls.
//
// opts are REQUIRED: nil is an explicit error. The previous silent
// (rawConn, nil, nil) passthrough was a trap for third-party callers -
// the connection stayed cleartext while the caller believed TLS had
// been negotiated. In-repo callers all guard at their call boundary.
func ExecuteTLSHandshake(ctx context.Context, rawConn net.Conn, opts *sesamepb.TlsOptions, defaultServerName string, provider TLSProvider) (net.Conn, *sesamepb.TlsHandshakeResult, error) {
	if opts == nil {
		return nil, nil, status.Error(codes.InvalidArgument, "sesame/rc/netconn: tls options required")
	}

	// Version-range validation applies to every provider, not just the
	// standard runtime: a floor above its ceiling can never negotiate.
	if minV, maxV := opts.GetMinVersion(), opts.GetMaxVersion(); minV != 0 && maxV != 0 && minV > maxV {
		return nil, nil, status.Error(codes.InvalidArgument, "sesame/rc/netconn: min_version exceeds max_version")
	}

	// ClientHelloSpec validation applies to every provider path.
	if spec := opts.GetClientHello(); spec != nil {
		if err := ValidateClientHelloSpec(spec); err != nil {
			return nil, nil, err
		}
	}

	if provider != nil {
		return provider.Handshake(ctx, rawConn, opts)
	}

	// Standard crypto/tls engine path.
	cfg, err := BuildTLSConfig(opts, defaultServerName)
	if err != nil {
		return nil, nil, err
	}

	// Apply the ClientHelloSpec if present, fail closed on unsupported dimensions.
	var appliedSpec *sesamepb.ClientHelloSpec
	if spec := opts.GetClientHello(); spec != nil {
		// Determine effective max version for cipher/group filtering logic.
		effectiveMax := cfg.MaxVersion
		if effectiveMax == 0 {
			effectiveMax = cryptotls.VersionTLS13 // Go default max
		}
		if err := ApplyGoClientHelloSpec(spec, cfg, effectiveMax); err != nil {
			return nil, nil, err
		}
		appliedSpec = spec // Echo verbatim: we validated we can honor it exactly.
	}

	// A peer that accepts TCP but never answers the ClientHello would
	// otherwise hang the handshake until the client disconnects; proxy
	// handshakes take the same fallback for the same reason. The raw
	// conn is armed from the caller's deadline or the fallback and
	// restored afterwards - steady-state deadlines belong to the
	// caller, and custom TLSProviders receive the ctx and own their
	// own handshake bounds.
	defer setHandshakeDeadline(ctx, rawConn)()

	tlsConn := cryptotls.Client(rawConn, cfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		// Map the failure to the canonical code per AIP-193 with the
		// same mapping the dial path uses (dialErrorCode), so a
		// stalled handshake surfaces DeadlineExceeded and a canceled
		// one Canceled - on parity with the TypeScript reference and
		// with Go's own dial errors.
		return nil, nil, status.Errorf(dialErrorCode(err), "sesame/rc/netconn: TLS handshake failed: %v", err)
	}

	result := ExtractTLSHandshakeResult(tlsConn.ConnectionState(), appliedSpec)
	return tlsConn, result, nil
}

// MaxProxyHops is the reference upper bound on proxy chain length. Each hop
// is a sequential server-side dial and handshake, so the count is a server
// resource commitment; requests exceeding the bound are rejected.
const MaxProxyHops = 8

// ExecuteProxyHops chains egress proxies across one or more hops to establish a tunnel to targetAddress.
func ExecuteProxyHops(ctx context.Context, baseDialer Dialer, proxyOpts *sesamepb.ProxyOptions, targetNetwork, targetAddress string) (net.Conn, *sesamepb.ProxyResult, error) {
	if proxyOpts == nil || len(proxyOpts.GetHops()) == 0 {
		if err := validateHostPort(targetAddress, "target address"); err != nil {
			return nil, nil, err
		}
		conn, err := baseDialer.DialContext(ctx, targetNetwork, targetAddress)
		if err != nil {
			return nil, nil, wrapDialError(err, "sesame/rc/netconn: dial %s %s failed", targetNetwork, targetAddress)
		}
		return conn, nil, nil
	}

	if len(proxyOpts.GetHops()) > MaxProxyHops {
		return nil, nil, status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: too many proxy hops: %d (max %d)", len(proxyOpts.GetHops()), MaxProxyHops)
	}

	hops := proxyOpts.GetHops()
	if err := validateProxyWireFields(targetAddress, hops); err != nil {
		return nil, nil, err
	}
	if err := validateSocks5Targets(targetAddress, hops); err != nil {
		return nil, nil, err
	}
	if err := validateProxyCredentials(hops); err != nil {
		return nil, nil, err
	}
	if err := validateHostPort(targetAddress, "target address"); err != nil {
		return nil, nil, err
	}
	// Every hop address is validated before the first dial: a chain
	// with a missing or malformed address is rejected without spending
	// a server-side dial on it.
	for i, hop := range hops {
		if hop.GetAddress() == nil || hop.GetAddress().GetAddress() == "" {
			if i == 0 {
				return nil, nil, status.Error(codes.InvalidArgument, "sesame/rc/netconn: first proxy hop missing address")
			}
			return nil, nil, status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: proxy hop %d missing address", i)
		}
		if err := validateHostPort(hop.GetAddress().GetAddress(), fmt.Sprintf("proxy hop %d address", i)); err != nil {
			return nil, nil, err
		}
	}
	traversed := make([]*typepb.NetAddr, 0, len(hops))

	// Dial the first hop using the base dialer
	firstHop := hops[0]
	firstAddr := firstHop.GetAddress()

	currentConn, err := baseDialer.DialContext(ctx, firstAddr.GetNetwork(), firstAddr.GetAddress())
	if err != nil {
		return nil, nil, wrapDialError(err, "sesame/rc/netconn: dialing first proxy hop %s failed", firstAddr.GetAddress())
	}

	traversed = append(traversed, firstAddr)

	// Chain through subsequent hops
	for i := 0; i < len(hops); i++ {
		hop := hops[i]
		var nextAddr string
		if i+1 < len(hops) {
			nextAddr = hops[i+1].GetAddress().GetAddress()
		} else {
			nextAddr = targetAddress
		}

		switch hop.GetType() {
		case sesamepb.ProxyHop_TYPE_HTTP_CONNECT:
			upgradedConn, err := httpConnectHandshake(ctx, currentConn, hop, nextAddr)
			if err != nil {
				_ = currentConn.Close()
				if ctx.Err() != nil {
					// The client disconnected mid-handshake; the cancel
					// watcher unblocked the stall. Report the
					// cancellation, not the induced timeout.
					return nil, nil, status.Error(codes.Canceled, "sesame/rc/netconn: connection aborted by client")
				}
				return nil, nil, err
			}
			currentConn = upgradedConn

		case sesamepb.ProxyHop_TYPE_SOCKS5:
			upgradedConn, err := socks5Handshake(ctx, currentConn, hop, nextAddr)
			if err != nil {
				_ = currentConn.Close()
				if ctx.Err() != nil {
					return nil, nil, status.Error(codes.Canceled, "sesame/rc/netconn: connection aborted by client")
				}
				return nil, nil, err
			}
			currentConn = upgradedConn

		default:
			_ = currentConn.Close()
			return nil, nil, status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: unsupported proxy type %v on hop %d", hop.GetType(), i)
		}

		if i+1 < len(hops) {
			traversed = append(traversed, hops[i+1].GetAddress())
		}
	}

	res := &sesamepb.ProxyResult{
		TraversedHops: traversed,
		EgressAddress: typepb.New(currentConn.RemoteAddr()),
	}

	return currentConn, res, nil
}

// validateProxyWireFields rejects control bytes in the strings that are
// interpolated verbatim into a raw HTTP CONNECT request line: the
// tunnel target (request line and Host header), each hop's auth_header
// (Proxy-Authorization line), and each subsequent hop address (the
// CONNECT target of the next hop's handshake). CR/LF in any of them
// would inject header or request lines into the proxy request; other
// C0 controls and DEL are garbage in an authority or header value and
// are rejected by the same fail-closed rule. Credentials are outside
// this rule: usernames and passwords travel base64-encoded (CONNECT
// basic auth) or length-prefixed (SOCKS5 RFC 1929), so they cannot
// break request framing.
func validateProxyWireFields(targetAddress string, hops []*sesamepb.ProxyHop) error {
	if err := rejectControlBytes(targetAddress, "proxy target address"); err != nil {
		return err
	}
	for i, hop := range hops {
		if err := rejectControlBytes(hop.GetAddress().GetAddress(), fmt.Sprintf("proxy hop %d address", i)); err != nil {
			return err
		}
		if err := rejectControlBytes(hop.GetAuthHeader(), fmt.Sprintf("proxy hop %d auth_header", i)); err != nil {
			return err
		}
	}
	return nil
}

// rejectControlBytes fails closed on any C0 control or DEL byte. The
// offending value is never echoed: proxy fields carry credentials.
func rejectControlBytes(s, field string) error {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: %s contains a control byte at offset %d; proxy handshake fields must not carry CR/LF or other control characters", field, i)
		}
	}
	return nil
}

// validateSocks5Targets rejects, per SOCKS5 hop, a CONNECT target the
// wire format cannot encode: RFC 1928 carries a domain target behind a
// u8 length field (ATYP 0x03), so a non-IP host longer than 255 bytes
// would silently wrap the length and corrupt the frame - the x/net
// dialer the Go stack delegates to writes the same truncated byte.
// The target of hop i is the next hop's address, or the tunnel target
// for the last hop. HTTP CONNECT has no such limit and is untouched.
func validateSocks5Targets(targetAddress string, hops []*sesamepb.ProxyHop) error {
	for i, hop := range hops {
		if hop.GetType() != sesamepb.ProxyHop_TYPE_SOCKS5 {
			continue
		}
		target := targetAddress
		if i+1 < len(hops) {
			target = hops[i+1].GetAddress().GetAddress()
		}
		if err := validateSocks5TargetEncodable(target); err != nil {
			return err
		}
	}
	return nil
}

func validateSocks5TargetEncodable(target string) error {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		// Malformed targets are the dial path's problem; the
		// encodability check only rejects what cannot be encoded.
		return nil
	}
	if net.ParseIP(host) != nil {
		// IP literals use ATYP 1/4 - no length field involved.
		return nil
	}
	if len(host) > 255 {
		return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: SOCKS5 target domain exceeds the RFC 1928 255-byte limit (%d bytes)", len(host))
	}
	return nil
}

// validateProxyCredentials rejects, per hop, a password with no
// username. Both handshake builders gate authentication on a non-empty
// username (HTTP CONNECT emits Proxy-Authorization only when the
// username is set; SOCKS5 negotiates RFC 1929 auth only then), so a
// password-only hop would silently negotiate NO-AUTH and the configured
// password would never reach the proxy. A misconfiguration this
// concrete fails loudly at the request boundary instead. A username
// with an empty password remains valid. The password value is never
// echoed.
func validateProxyCredentials(hops []*sesamepb.ProxyHop) error {
	for i, hop := range hops {
		if hop.GetPassword() != "" && hop.GetUsername() == "" {
			return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: proxy hop %d has a password but no username; authentication requires a username (an empty password is valid)", i)
		}
	}
	return nil
}

// validateHostPort rejects an address that is not a parseable host:port.
// Dial and proxy-hop targets are client-controlled input, and per
// AIP-193 a malformed value fails with INVALID_ARGUMENT at the
// boundary - before any dialing or handshake side effects - instead of
// surfacing later as an opaque error from the dial path. Mirrors
// parseHostPort in the TypeScript reference: bracketed IPv6 literals
// are accepted, and the port must be pure ASCII digits in [1, 65535]
// (strconv tolerates a sign and JS parseInt tolerates trailing garbage;
// neither leniency belongs in a wire address). field names the address
// in messages.
func validateHostPort(addr, field string) error {
	if addr == "" {
		return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: missing %s", field)
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: invalid %s %q: %v", field, addr, err)
	}
	if !isDigits(port) {
		return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: invalid port in %s %q", field, addr)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: invalid port in %s %q", field, addr)
	}
	return nil
}

// isDigits reports whether s is non-empty and composed solely of ASCII
// digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// wrapDialError converts a raw dial error - one carrying no gRPC
// status; a custom DialerFactory's status errors pass through verbatim,
// details included - into a status error with the canonical code per
// AIP-193: Canceled for an aborted request, DeadlineExceeded for
// timeouts, Unavailable otherwise. Raw errors would otherwise surface
// as Unknown. Mirrors the TypeScript reference's dialTcp mapping.
func wrapDialError(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	return status.Errorf(dialErrorCode(err), format+": %v", append(args, err)...)
}

// dialErrorCode maps a raw dial error to its canonical gRPC code.
func dialErrorCode(err error) codes.Code {
	if errors.Is(err, context.Canceled) {
		return codes.Canceled
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return codes.DeadlineExceeded
	}
	return codes.Unavailable
}

// handshakeFallbackTimeout bounds proxy protocol handshakes (HTTP
// CONNECT, SOCKS5) and the standard-engine TLS handshake when the
// caller context carries no deadline, so a peer that stalls
// mid-handshake cannot hang the dial (or the in-stream upgrade)
// forever.
const handshakeFallbackTimeout = 30 * time.Second

// setHandshakeDeadline arms conn with either the caller's context
// deadline or the handshakeFallbackTimeout fallback, returning a restore
// func. The restore clears BOTH conn deadlines: a caller that armed its
// own steady-state deadline before the handshake must re-arm it
// afterwards (in-repo callers always hand over a fresh conn, or clear
// deadlines themselves around the handshake).
func setHandshakeDeadline(ctx context.Context, conn net.Conn) func() {
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	} else {
		_ = conn.SetDeadline(time.Now().Add(handshakeFallbackTimeout))
	}
	return func() { _ = conn.SetDeadline(timeZero) }
}

// watchHandshakeCancel unblocks a handshake parked on conn I/O when ctx
// is cancelled, by forcing a past deadline, so a client that disconnects
// mid-handshake does not hold the goroutine and socket until the
// fallback deadline. Mirrors the abort wiring the TypeScript reference
// threads through its proxy handshakes. The returned stop func waits for
// the watcher to settle, so a racing cancel-arming cannot fire after the
// handshake returns; the caller's deadline restore (registered before
// this watcher, so it runs after stop) clears any induced deadline.
func watchHandshakeCancel(ctx context.Context, conn net.Conn) func() {
	if ctx.Done() == nil {
		return func() {}
	}
	stopped := make(chan struct{})
	stoppedAck := make(chan struct{})
	go func() {
		defer close(stoppedAck)
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now().Add(-time.Second))
		case <-stopped:
		}
	}()
	return func() {
		close(stopped)
		<-stoppedAck
	}
}

func httpConnectHandshake(ctx context.Context, conn net.Conn, hop *sesamepb.ProxyHop, target string) (net.Conn, error) {
	reqStr := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
	if h := hop.GetAuthHeader(); h != "" {
		reqStr += fmt.Sprintf("Proxy-Authorization: %s\r\n", h)
	} else if u := hop.GetUsername(); u != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(u + ":" + hop.GetPassword()))
		reqStr += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", auth)
	}
	reqStr += "\r\n"

	defer setHandshakeDeadline(ctx, conn)()
	defer watchHandshakeCancel(ctx, conn)()

	if _, err := io.WriteString(conn, reqStr); err != nil {
		return nil, status.Errorf(codes.Unavailable, "sesame/rc/netconn: failed writing HTTP CONNECT to proxy: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "sesame/rc/netconn: failed reading HTTP CONNECT response: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check the status first: a rejecting proxy is entitled to frame its
	// error body, and the status is the more actionable failure.
	if resp.StatusCode != http.StatusOK {
		return nil, status.Errorf(codes.PermissionDenied, "sesame/rc/netconn: HTTP CONNECT to %s failed with status: %s", target, resp.Status)
	}

	// A 200 response to CONNECT must not carry a body at all. A declared
	// Content-Length means the proxy is not actually tunnelling and its
	// bytes would be mistaken for tunnel payload; the same holds for any
	// transfer framing, which http.ReadResponse surfaces in TransferEncoding.
	if len(resp.TransferEncoding) > 0 || resp.ContentLength > 0 {
		return nil, status.Errorf(codes.Unavailable, "sesame/rc/netconn: HTTP CONNECT to %s returned a framed response body", target)
	}

	if br.Buffered() > 0 {
		buf := make([]byte, br.Buffered())
		if _, err := io.ReadFull(br, buf); err != nil {
			return nil, err
		}
		return &bufferedPrefixConn{Conn: conn, prefix: buf}, nil
	}

	return conn, nil
}

func socks5Handshake(ctx context.Context, conn net.Conn, hop *sesamepb.ProxyHop, target string) (net.Conn, error) {
	var auth *xproxy.Auth
	if u := hop.GetUsername(); u != "" {
		auth = &xproxy.Auth{
			User:     u,
			Password: hop.GetPassword(),
		}
	}

	dialer, err := xproxy.SOCKS5("tcp", "", auth, &directConnDialer{conn: conn})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "sesame/rc/netconn: failed initializing SOCKS5 dialer: %v", err)
	}

	defer setHandshakeDeadline(ctx, conn)()
	defer watchHandshakeCancel(ctx, conn)()

	upgradedConn, err := dialer.Dial("tcp", target)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "sesame/rc/netconn: SOCKS5 handshake to %s failed: %v", target, err)
	}

	return upgradedConn, nil
}

type directConnDialer struct {
	conn net.Conn
}

func (d *directConnDialer) Dial(network, addr string) (net.Conn, error) {
	if d.conn == nil {
		return nil, errors.New("sesame/rc/netconn: proxy dialer already consumed")
	}
	c := d.conn
	d.conn = nil
	return c, nil
}

type bufferedPrefixConn struct {
	net.Conn
	prefix []byte
}

func (b *bufferedPrefixConn) Read(p []byte) (int, error) {
	if len(b.prefix) > 0 {
		n := copy(p, b.prefix)
		b.prefix = b.prefix[n:]
		return n, nil
	}
	return b.Conn.Read(p)
}

// CloseWrite half-closes the tunnel by delegating to the wrapped conn.
// The buffered prefix is read-side state, so reads keep draining it
// after the write side closes. A wrapped conn without CloseWrite
// reports that honestly instead of failing the type assertion at the
// caller.
func (b *bufferedPrefixConn) CloseWrite() error {
	cw, ok := b.Conn.(interface{ CloseWrite() error })
	if !ok {
		return errors.New("sesame/rc/netconn: wrapped connection does not support half-close")
	}
	return cw.CloseWrite()
}

var timeZero = time.Time{}
