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
	"time"

	sesameproxy "github.com/joeycumines/sesame/rc/proxy"
	sesametls "github.com/joeycumines/sesame/rc/tls"
	"github.com/joeycumines/sesame/type/netaddr"
	xproxy "golang.org/x/net/proxy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type (
	// TLSProvider allows plugging in an alternative TLS implementation, such as uTLS
	// for client cryptographic fingerprint impersonation or custom handshake engines.
	TLSProvider interface {
		Handshake(ctx context.Context, rawConn net.Conn, opts *sesametls.TLSOptions) (net.Conn, *sesametls.TLSHandshakeResult, error)
	}

	// ConnTransformResult provides metadata about negotiated TLS and proxy state.
	ConnTransformResult interface {
		TLSHandshakeResult() *sesametls.TLSHandshakeResult
		ProxyResult() *sesameproxy.ProxyResult
	}

	transformedConn struct {
		net.Conn
		tlsResult   *sesametls.TLSHandshakeResult
		proxyResult *sesameproxy.ProxyResult
	}
)

var (
	_ ConnTransformResult = (*transformedConn)(nil)
)

func (c *transformedConn) TLSHandshakeResult() *sesametls.TLSHandshakeResult {
	return c.tlsResult
}

func (c *transformedConn) ProxyResult() *sesameproxy.ProxyResult {
	return c.proxyResult
}

// ProtoToTLSVersion maps a protobuf TLSVersion to the crypto/tls version constant.
func ProtoToTLSVersion(v sesametls.TLSVersion) uint16 {
	switch v {
	case sesametls.TLSVersion_TLS_1_0:
		return cryptotls.VersionTLS10
	case sesametls.TLSVersion_TLS_1_1:
		return cryptotls.VersionTLS11
	case sesametls.TLSVersion_TLS_1_2:
		return cryptotls.VersionTLS12
	case sesametls.TLSVersion_TLS_1_3:
		return cryptotls.VersionTLS13
	default:
		return 0
	}
}

// TLSVersionToProto maps a crypto/tls version constant to the protobuf TLSVersion.
func TLSVersionToProto(v uint16) sesametls.TLSVersion {
	switch v {
	case cryptotls.VersionTLS10:
		return sesametls.TLSVersion_TLS_1_0
	case cryptotls.VersionTLS11:
		return sesametls.TLSVersion_TLS_1_1
	case cryptotls.VersionTLS12:
		return sesametls.TLSVersion_TLS_1_2
	case cryptotls.VersionTLS13:
		return sesametls.TLSVersion_TLS_1_3
	default:
		return sesametls.TLSVersion_TLS_VERSION_UNSPECIFIED
	}
}

// BuildTLSConfig constructs a crypto/tls.Config from a sesametls.TLSOptions message.
func BuildTLSConfig(opts *sesametls.TLSOptions, defaultServerName string) (*cryptotls.Config, error) {
	if opts == nil {
		return nil, nil
	}

	serverName := opts.GetServerName()
	if serverName == "" {
		serverName = defaultServerName
	}
	// Strip port if present in serverName
	if host, _, err := net.SplitHostPort(serverName); err == nil {
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
func ExtractTLSHandshakeResult(state cryptotls.ConnectionState, appliedSpec *sesametls.ClientHelloSpec) *sesametls.TLSHandshakeResult {
	res := &sesametls.TLSHandshakeResult{
		NegotiatedProtocol: state.NegotiatedProtocol,
		CipherSuite:        int32(state.CipherSuite),
		TlsVersion:         TLSVersionToProto(state.Version),
		ServerName:         state.ServerName,
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
func ExecuteTLSHandshake(ctx context.Context, rawConn net.Conn, opts *sesametls.TLSOptions, defaultServerName string, provider TLSProvider) (net.Conn, *sesametls.TLSHandshakeResult, error) {
	if opts == nil {
		return rawConn, nil, nil
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
	var appliedSpec *sesametls.ClientHelloSpec
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

	tlsConn := cryptotls.Client(rawConn, cfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, nil, status.Errorf(codes.Unavailable, "sesame/rc/netconn: TLS handshake failed: %v", err)
	}

	result := ExtractTLSHandshakeResult(tlsConn.ConnectionState(), appliedSpec)
	return tlsConn, result, nil
}

// MaxProxyHops is the reference upper bound on proxy chain length. Each hop
// is a sequential server-side dial and handshake, so the count is a server
// resource commitment; requests exceeding the bound are rejected.
const MaxProxyHops = 8

// ExecuteProxyHops chains egress proxies across one or more hops to establish a tunnel to targetAddress.
func ExecuteProxyHops(ctx context.Context, baseDialer Dialer, proxyOpts *sesameproxy.ProxyOptions, targetNetwork, targetAddress string) (net.Conn, *sesameproxy.ProxyResult, error) {
	if proxyOpts == nil || len(proxyOpts.GetHops()) == 0 {
		conn, err := baseDialer.DialContext(ctx, targetNetwork, targetAddress)
		return conn, nil, err
	}

	if len(proxyOpts.GetHops()) > MaxProxyHops {
		return nil, nil, status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: too many proxy hops: %d (max %d)", len(proxyOpts.GetHops()), MaxProxyHops)
	}

	hops := proxyOpts.GetHops()
	traversed := make([]*netaddr.NetAddr, 0, len(hops))

	// Dial the first hop using the base dialer
	firstHop := hops[0]
	firstAddr := firstHop.GetAddress()
	if firstAddr == nil {
		return nil, nil, status.Error(codes.InvalidArgument, "sesame/rc/netconn: first proxy hop missing address")
	}

	currentConn, err := baseDialer.DialContext(ctx, firstAddr.GetNetwork(), firstAddr.GetAddress())
	if err != nil {
		return nil, nil, status.Errorf(codes.Unavailable, "sesame/rc/netconn: dialing first proxy hop %s failed: %v", firstAddr.GetAddress(), err)
	}

	traversed = append(traversed, firstAddr)

	// Chain through subsequent hops
	for i := 0; i < len(hops); i++ {
		hop := hops[i]
		var nextAddr string
		if i+1 < len(hops) {
			nextHopAddr := hops[i+1].GetAddress()
			if nextHopAddr == nil {
				_ = currentConn.Close()
				return nil, nil, status.Errorf(codes.InvalidArgument, "sesame/rc/netconn: proxy hop %d missing address", i+1)
			}
			nextAddr = nextHopAddr.GetAddress()
		} else {
			nextAddr = targetAddress
		}

		switch hop.GetType() {
		case sesameproxy.ProxyHop_HTTP_CONNECT:
			upgradedConn, err := httpConnectHandshake(ctx, currentConn, hop, nextAddr)
			if err != nil {
				_ = currentConn.Close()
				return nil, nil, err
			}
			currentConn = upgradedConn

		case sesameproxy.ProxyHop_SOCKS5:
			upgradedConn, err := socks5Handshake(ctx, currentConn, hop, nextAddr)
			if err != nil {
				_ = currentConn.Close()
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

	res := &sesameproxy.ProxyResult{
		TraversedHops: traversed,
		EgressAddress: netaddr.New(currentConn.RemoteAddr()),
	}

	return currentConn, res, nil
}

// proxyHandshakeTimeout bounds proxy protocol handshakes (HTTP CONNECT,
// SOCKS5) when the caller context carries no deadline, so a proxy that
// stalls mid-handshake cannot hang ExecuteProxyHops forever.
const proxyHandshakeTimeout = 30 * time.Second

// setProxyHandshakeDeadline arms conn with either the caller's context
// deadline or the proxyHandshakeTimeout fallback, returning a restore func.
func setProxyHandshakeDeadline(ctx context.Context, conn net.Conn) func() {
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	} else {
		_ = conn.SetDeadline(time.Now().Add(proxyHandshakeTimeout))
	}
	return func() { _ = conn.SetDeadline(timeZero) }
}

func httpConnectHandshake(ctx context.Context, conn net.Conn, hop *sesameproxy.ProxyHop, target string) (net.Conn, error) {
	reqStr := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
	if h := hop.GetAuthHeader(); h != "" {
		reqStr += fmt.Sprintf("Proxy-Authorization: %s\r\n", h)
	} else if u := hop.GetUsername(); u != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(u + ":" + hop.GetPassword()))
		reqStr += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", auth)
	}
	reqStr += "\r\n"

	defer setProxyHandshakeDeadline(ctx, conn)()

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

func socks5Handshake(ctx context.Context, conn net.Conn, hop *sesameproxy.ProxyHop, target string) (net.Conn, error) {
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

	defer setProxyHandshakeDeadline(ctx, conn)()

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
