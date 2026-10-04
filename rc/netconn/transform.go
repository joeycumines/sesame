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

	"github.com/joeycumines/sesame/type/netaddr"
	sesameproxy "github.com/joeycumines/sesame/type/proxy"
	sesametls "github.com/joeycumines/sesame/type/tls"
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

	if len(opts.GetCipherSuites()) > 0 {
		cfg.CipherSuites = make([]uint16, len(opts.GetCipherSuites()))
		for i, cs := range opts.GetCipherSuites() {
			cfg.CipherSuites[i] = uint16(cs)
		}
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
func ExtractTLSHandshakeResult(state cryptotls.ConnectionState, appliedPreset sesametls.FingerprintPreset) *sesametls.TLSHandshakeResult {
	res := &sesametls.TLSHandshakeResult{
		NegotiatedProtocol: state.NegotiatedProtocol,
		CipherSuite:        uint32(state.CipherSuite),
		TlsVersion:         TLSVersionToProto(state.Version),
		ServerName:         state.ServerName,
		AppliedPreset:      appliedPreset,
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

	preset := opts.GetFingerprintPreset()
	if preset != sesametls.FingerprintPreset_FINGERPRINT_PRESET_UNSPECIFIED &&
		preset != sesametls.FingerprintPreset_RUNTIME_DEFAULT {
		if provider == nil {
			// RFC Section 4.1: Server MUST return FAILED_PRECONDITION if it cannot satisfy requested preset
			return nil, nil, status.Errorf(codes.FailedPrecondition,
				"sesame/rc/netconn: requested fingerprint preset %v is not supported by standard runtime; custom TLSProvider required",
				preset)
		}
		return provider.Handshake(ctx, rawConn, opts)
	}

	if provider != nil {
		return provider.Handshake(ctx, rawConn, opts)
	}

	cfg, err := BuildTLSConfig(opts, defaultServerName)
	if err != nil {
		return nil, nil, err
	}

	tlsConn := cryptotls.Client(rawConn, cfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, nil, status.Errorf(codes.Unavailable, "sesame/rc/netconn: TLS handshake failed: %v", err)
	}

	result := ExtractTLSHandshakeResult(tlsConn.ConnectionState(), sesametls.FingerprintPreset_RUNTIME_DEFAULT)
	return tlsConn, result, nil
}

// ExecuteProxyHops chains egress proxies across one or more hops to establish a tunnel to targetAddress.
func ExecuteProxyHops(ctx context.Context, baseDialer Dialer, proxyOpts *sesameproxy.ProxyOptions, targetNetwork, targetAddress string) (net.Conn, *sesameproxy.ProxyResult, error) {
	if proxyOpts == nil || len(proxyOpts.GetHops()) == 0 {
		conn, err := baseDialer.DialContext(ctx, targetNetwork, targetAddress)
		return conn, nil, err
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

func httpConnectHandshake(ctx context.Context, conn net.Conn, hop *sesameproxy.ProxyHop, target string) (net.Conn, error) {
	reqStr := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
	if h := hop.GetAuthHeader(); h != "" {
		reqStr += fmt.Sprintf("Proxy-Authorization: %s\r\n", h)
	} else if u := hop.GetUsername(); u != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(u + ":" + hop.GetPassword()))
		reqStr += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", auth)
	}
	reqStr += "\r\n"

	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
		defer func() { _ = conn.SetDeadline(timeZero) }()
	} else {
		// Bound the handshake even without a caller deadline so a proxy
		// that stalls mid-headers cannot hang ExecuteProxyHops forever.
		// A CONNECT response is headers-only; 30s is generous.
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
		defer func() { _ = conn.SetDeadline(timeZero) }()
	}

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

	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
		defer func() { _ = conn.SetDeadline(timeZero) }()
	}

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

var timeZero = time.Time{}
