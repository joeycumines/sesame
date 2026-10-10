package integration_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	cryptotls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/rc"
	"github.com/joeycumines/sesame/rc/netconn"
	sesameproxy "github.com/joeycumines/sesame/rc/proxy"
	sesametls "github.com/joeycumines/sesame/rc/tls"
	"github.com/joeycumines/sesame/type/netaddr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockPipeDialer struct {
	conn net.Conn
}

func (m *mockPipeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return m.conn, nil
}

func generateCertAndKey(t *testing.T, hosts ...string) ([]byte, []byte, *x509.CertPool) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	notBefore := time.Now().Add(-time.Hour)
	notAfter := notBefore.Add(24 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Sesame Integration Test"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		t.Fatal("failed to append cert to pool")
	}

	return certPEM, keyPEM, pool
}

// TestLifecycle_EndpointTLS_ALPN tests endpoint TLS termination and ALPN negotiation.
func TestLifecycle_EndpointTLS_ALPN(t *testing.T) {
	skipUnlessIntegration(t)

	certPEM, keyPEM, _ := generateCertAndKey(t, "mail.internal")
	cert, err := cryptotls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Upstream TLS echo server
	serverTLSConfig := &cryptotls.Config{
		Certificates: []cryptotls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	}

	go func() {
		tlsServerConn := cryptotls.Server(serverPipe, serverTLSConfig)
		if err := tlsServerConn.Handshake(); err != nil {
			return
		}
		buf := make([]byte, 1024)
		n, err := tlsServerConn.Read(buf)
		if err == nil && n > 0 {
			_, _ = tlsServerConn.Write(buf[:n])
		}
	}()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := testutil.ClientConnFactories["inprocgrpc"](func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		TLS: &sesametls.TLSOptions{
			ServerName:         "mail.internal",
			AlpnProtocols:      []string{"h2", "http/1.1"},
			CaCertificates:     certPEM,
			InsecureSkipVerify: true,
		},
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsOpportunisticTls: true,
			SupportsFlowControl:      true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", "mail.internal:443")
	if err != nil {
		t.Fatalf("DialContext error: %v", err)
	}
	defer conn.Close()

	inStreamConn, ok := conn.(netconn.InStreamConn)
	if !ok {
		t.Fatalf("expected InStreamConn, got %T", conn)
	}

	tlsRes := inStreamConn.TLSResult()
	if tlsRes == nil {
		t.Fatalf("missing TLSHandshakeResult")
	}
	if tlsRes.GetNegotiatedProtocol() != "h2" {
		t.Errorf("got negotiated protocol %q, want h2", tlsRes.GetNegotiatedProtocol())
	}

	// Echo verification over TLS stream
	msg := []byte("hello tls lifecycle")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	reply := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if !bytes.Equal(reply, msg) {
		t.Errorf("got %q, want %q", reply, msg)
	}
}

// TestLifecycle_ALPN_EmptySuppressionRule verifies empty alpn_protocols sends no ALPN.
func TestLifecycle_ALPN_EmptySuppressionRule(t *testing.T) {
	skipUnlessIntegration(t)

	certPEM, keyPEM, _ := generateCertAndKey(t, "mail.internal")
	cert, err := cryptotls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}

	var serverNegotiatedProtocol string
	serverTLSConfig := &cryptotls.Config{
		Certificates: []cryptotls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	}

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	go func() {
		tlsServerConn := cryptotls.Server(serverPipe, serverTLSConfig)
		_ = tlsServerConn.Handshake()
		serverNegotiatedProtocol = tlsServerConn.ConnectionState().NegotiatedProtocol
		buf := make([]byte, 64)
		n, _ := tlsServerConn.Read(buf)
		if n > 0 {
			_, _ = tlsServerConn.Write(buf[:n])
		}
	}()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := testutil.ClientConnFactories["inprocgrpc"](func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		TLS: &sesametls.TLSOptions{
			ServerName:         "mail.internal",
			CaCertificates:     certPEM,
			InsecureSkipVerify: true,
			// empty alpn_protocols triggers empty suppression rule
		},
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsOpportunisticTls: true,
			SupportsFlowControl:      true,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", "mail.internal:443")
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}

	if serverNegotiatedProtocol != "" {
		t.Errorf("expected empty ALPN protocol negotiation, got %q", serverNegotiatedProtocol)
	}
}

// TestLifecycle_HTTPConnectProxyHop tests HTTP CONNECT proxy hop chaining.
func TestLifecycle_HTTPConnectProxyHop(t *testing.T) {
	skipUnlessIntegration(t)

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	targetAddr := "target.internal:80"

	// Mock proxy server
	go func() {
		buf := make([]byte, 1024)
		n, err := serverPipe.Read(buf)
		if err != nil {
			return
		}
		reqStr := string(buf[:n])
		if !strings.HasPrefix(reqStr, "CONNECT target.internal:80 HTTP/1.1\r\n") {
			_, _ = serverPipe.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			return
		}
		if !strings.Contains(reqStr, "Proxy-Authorization: Basic c2VjcmV0LXVzZXI6c2VjcmV0LXBhc3M=\r\n") {
			_, _ = serverPipe.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\n"))
			return
		}
		_, _ = serverPipe.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

		// Echo through established tunnel
		echoBuf := make([]byte, 1024)
		en, err := serverPipe.Read(echoBuf)
		if err == nil && en > 0 {
			_, _ = serverPipe.Write(echoBuf[:en])
		}
	}()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := testutil.ClientConnFactories["inprocgrpc"](func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		Proxy: &sesameproxy.ProxyOptions{
			Hops: []*sesameproxy.ProxyHop{
				{
					Type: sesameproxy.ProxyHop_HTTP_CONNECT,
					Address: &netaddr.NetAddr{
						Network: "tcp",
						Address: "proxy.internal:8080",
					},
					Username: "secret-user",
					Password: "secret-pass",
				},
			},
		},
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsOpportunisticTls: true,
			SupportsFlowControl:      true,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", targetAddr)
	if err != nil {
		t.Fatalf("DialContext through proxy: %v", err)
	}
	defer conn.Close()

	testMsg := []byte("proxied payload echo")
	if _, err := conn.Write(testMsg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	reply := make([]byte, len(testMsg))
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if !bytes.Equal(reply, testMsg) {
		t.Errorf("got %q, want %q", reply, testMsg)
	}
}

// TestLifecycle_InStream_STARTTLS tests dynamic in-stream UpgradeTLS.
func TestLifecycle_InStream_STARTTLS(t *testing.T) {
	skipUnlessIntegration(t)

	certPEM, keyPEM, _ := generateCertAndKey(t, "mail.internal")
	cert, err := cryptotls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Upstream echo server that transitions from plaintext to TLS upon seeing "STARTTLS\n"
	go func() {
		buf := make([]byte, 128)
		n, err := serverPipe.Read(buf)
		if err != nil {
			return
		}
		if string(buf[:n]) != "STARTTLS\n" {
			_, _ = serverPipe.Write([]byte("500 Syntax error\n"))
			return
		}
		_, _ = serverPipe.Write([]byte("220 Ready for TLS\n"))

		// Wrap in TLS server
		tlsServer := cryptotls.Server(serverPipe, &cryptotls.Config{
			Certificates: []cryptotls.Certificate{cert},
			NextProtos:   []string{"test-proto"},
		})
		if err := tlsServer.Handshake(); err != nil {
			return
		}

		echoBuf := make([]byte, 256)
		en, err := tlsServer.Read(echoBuf)
		if err == nil && en > 0 {
			_, _ = tlsServer.Write(echoBuf[:en])
		}
	}()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := testutil.ClientConnFactories["inprocgrpc"](func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsOpportunisticTls: true,
			SupportsFlowControl:      true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", "mail.internal:25")
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	defer conn.Close()

	inStreamConn, ok := conn.(netconn.InStreamConn)
	if !ok {
		t.Fatalf("expected InStreamConn, got %T", conn)
	}

	// 1. Cleartext exchange
	if _, err := inStreamConn.Write([]byte("STARTTLS\n")); err != nil {
		t.Fatalf("Write STARTTLS: %v", err)
	}
	replyBuf := make([]byte, 128)
	rn, err := inStreamConn.Read(replyBuf)
	if err != nil {
		t.Fatalf("Read reply: %v", err)
	}
	if string(replyBuf[:rn]) != "220 Ready for TLS\n" {
		t.Fatalf("unexpected cleartext reply: %s", string(replyBuf[:rn]))
	}

	// 2. Trigger in-stream UpgradeTLS
	upgradeOpts := &sesametls.TLSOptions{
		ServerName:         "mail.internal",
		AlpnProtocols:      []string{"test-proto"},
		CaCertificates:     certPEM,
		InsecureSkipVerify: true,
	}

	tlsRes, err := inStreamConn.UpgradeTLS(ctx, upgradeOpts)
	if err != nil {
		t.Fatalf("UpgradeTLS failed: %v", err)
	}
	if tlsRes == nil {
		t.Fatal("expected non-nil UpgradeTLS result")
	}
	if tlsRes.GetNegotiatedProtocol() != "test-proto" {
		t.Errorf("got negotiated protocol %q, want test-proto", tlsRes.GetNegotiatedProtocol())
	}

	// 3. Post-upgrade encrypted data exchange
	encMsg := []byte("hello encrypted world")
	if _, err := inStreamConn.Write(encMsg); err != nil {
		t.Fatalf("Write encrypted: %v", err)
	}
	encReply := make([]byte, len(encMsg))
	if _, err := io.ReadFull(inStreamConn, encReply); err != nil {
		t.Fatalf("ReadFull encrypted: %v", err)
	}
	if !bytes.Equal(encReply, encMsg) {
		t.Errorf("got %q, want %q", encReply, encMsg)
	}
}

// TestLifecycle_InStream_PingPong tests in-stream ping/pong liveness.
func TestLifecycle_InStream_PingPong(t *testing.T) {
	skipUnlessIntegration(t)

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := testutil.ClientConnFactories["inprocgrpc"](func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsOpportunisticTls: true,
			SupportsFlowControl:      true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	defer conn.Close()

	inStreamConn, ok := conn.(netconn.InStreamConn)
	if !ok {
		t.Fatalf("expected InStreamConn, got %T", conn)
	}

	rtt, err := inStreamConn.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if rtt < 0 {
		t.Errorf("negative RTT: %v", rtt)
	}
}

// TestLifecycle_FailClosed_Security tests fail-closed security invariants.
func TestLifecycle_FailClosed_Security(t *testing.T) {
	skipUnlessIntegration(t)

	t.Run("Server rejects un-honorable client_hello with FailedPrecondition", func(t *testing.T) {
		clientPipe, serverPipe := net.Pipe()
		defer clientPipe.Close()
		defer serverPipe.Close()

		server := netconn.Server{
			Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
				return &mockPipeDialer{conn: clientPipe}, nil
			},
		}

		gc := testutil.ClientConnFactories["inprocgrpc"](func(h testutil.GRPCServer) {
			rc.RegisterRemoteControlServer(h, &server)
		})
		defer gc.Close()

		client := netconn.Client{
			API: rc.NewRemoteControlClient(gc),
			TLS: &sesametls.TLSOptions{
				ClientHello: &sesametls.ClientHelloSpec{
					SignatureAlgorithms: []int32{0x0403},
				},
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := client.DialContext(ctx, "tcp", "target:443")
		if err == nil {
			t.Fatal("expected error for un-honorable client_hello dimension, got nil")
		}
		stat, _ := status.FromError(err)
		if stat.Code() != codes.FailedPrecondition {
			t.Errorf("expected FailedPrecondition, got code %v: %v", stat.Code(), err)
		}
	})

	t.Run("Client aborts on unconfirmed TLS", func(t *testing.T) {
		clientPipe, serverPipe := net.Pipe()
		defer clientPipe.Close()
		defer serverPipe.Close()

		// Server returns Conn without TLS confirmation
		gc := testutil.ClientConnFactories["inprocgrpc"](func(h testutil.GRPCServer) {
			rc.RegisterRemoteControlServer(h, &mockUnconfirmedServer{
				dialer: &mockPipeDialer{conn: clientPipe},
			})
		})
		defer gc.Close()

		client := netconn.Client{
			API: rc.NewRemoteControlClient(gc),
			TLS: &sesametls.TLSOptions{ServerName: "target"},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := client.DialContext(ctx, "tcp", "target:443")
		if err == nil {
			t.Fatal("expected client to fail closed on unconfirmed TLS, got nil")
		}
		if !strings.Contains(err.Error(), "security violation: server returned cleartext") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

type mockUnconfirmedServer struct {
	dialer netconn.Dialer
	rc.UnimplementedRemoteControlServer
}

func (m *mockUnconfirmedServer) NetConn(stream rc.RemoteControl_NetConnServer) error {
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	conn, err := m.dialer.DialContext(stream.Context(), msg.GetDial().GetAddress().GetNetwork(), msg.GetDial().GetAddress().GetAddress())
	if err != nil {
		return err
	}
	defer conn.Close()

	// Deliberately omit TLS and Proxy results in Conn response to test fail-closed invariants
	return stream.Send(&rc.NetConnResponse{
		Data: &rc.NetConnResponse_Conn_{
			Conn: &rc.NetConnResponse_Conn{
				Local:  netaddr.New(conn.LocalAddr()),
				Remote: netaddr.New(conn.RemoteAddr()),
			},
		},
	})
}
