package netconn_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	cryptotls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/rc"
	"github.com/joeycumines/sesame/rc/netconn"
	sesameproxy "github.com/joeycumines/sesame/rc/proxy"
	sesametls "github.com/joeycumines/sesame/rc/tls"
	"github.com/joeycumines/sesame/type/netaddr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFlowController(t *testing.T) {
	fc := netconn.NewFlowController(100)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Should acquire 60 bytes immediately (remaining: 40)
	if err := fc.Acquire(ctx, 60); err != nil {
		t.Fatalf("unexpected acquire error: %v", err)
	}

	// Should acquire 40 bytes immediately (remaining: 0)
	if err := fc.Acquire(ctx, 40); err != nil {
		t.Fatalf("unexpected acquire error: %v", err)
	}

	// Attempting to acquire 10 bytes now should block until AddCredit
	var wg sync.WaitGroup
	wg.Add(1)
	acquired := false
	go func() {
		defer wg.Done()
		if err := fc.Acquire(ctx, 10); err == nil {
			acquired = true
		}
	}()

	time.Sleep(50 * time.Millisecond)
	if acquired {
		t.Fatal("should have blocked on empty window")
	}

	fc.AddCredit(20)
	wg.Wait()

	if !acquired {
		t.Fatal("expected acquire to succeed after AddCredit")
	}

	// Test Close unblocking
	wg.Add(1)
	closeAcquiredErr := false
	go func() {
		defer wg.Done()
		if err := fc.Acquire(ctx, 100); err != nil {
			closeAcquiredErr = true
		}
	}()

	time.Sleep(50 * time.Millisecond)
	fc.Close()
	wg.Wait()

	if !closeAcquiredErr {
		t.Fatal("expected error after Close on flow controller")
	}
}

func TestClientServer_InStream_Ping(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
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
		t.Fatalf("DialContext failed: %v", err)
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
	if rtt <= 0 {
		t.Errorf("expected positive RTT, got %v", rtt)
	}

	if caps := inStreamConn.ServerCapabilities(); caps == nil || !caps.GetSupportsOpportunisticTls() {
		t.Errorf("unexpected server capabilities: %v", caps)
	}
}

func TestClientServer_FailClosed_CleartextViolation(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Server that omits TLS in Conn response
	mockServer := &mockServerWithoutTLS{
		dialer: &mockPipeDialer{conn: clientPipe},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, mockServer)
	})
	defer gc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		TLS: &sesametls.TLSOptions{
			ServerName: "secure.internal",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := client.DialContext(ctx, "tcp", "secure.internal:443")
	if err == nil {
		t.Fatal("expected fail-closed error when TLS was requested but server returned cleartext")
	}
	if !strings.Contains(err.Error(), "security violation: server returned cleartext") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestClientServer_FailClosed_PresetUnsupported(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	// Request an unsupported preset without configuring custom TLSProvider
	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		TLS: &sesametls.TLSOptions{
			ServerName:        "secure.internal",
			FingerprintPreset: sesametls.FingerprintPreset_CHROME_131,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := client.DialContext(ctx, "tcp", "secure.internal:443")
	if err == nil {
		t.Fatal("expected FAILED_PRECONDITION error for unsupported preset")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.FailedPrecondition {
		t.Errorf("expected codes.FailedPrecondition, got error: %v", err)
	}
}

func TestClientServer_FailClosed_ProxyViolation(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Server that omits ProxyResult
	mockServer := &mockServerWithoutTLS{
		dialer: &mockPipeDialer{conn: clientPipe},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, mockServer)
	})
	defer gc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		Proxy: &sesameproxy.ProxyOptions{
			Hops: []*sesameproxy.ProxyHop{
				{
					Type:    sesameproxy.ProxyHop_HTTP_CONNECT,
					Address: &netaddr.NetAddr{Network: "tcp", Address: "proxy.internal:8080"},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := client.DialContext(ctx, "tcp", "target.internal:443")
	if err == nil {
		t.Fatal("expected fail-closed error when proxy requested but server returned unproxied")
	}
	if !strings.Contains(err.Error(), "security violation: server returned unproxied") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestClientServer_InStream_UpgradeTLS(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	// Generate a self-signed TLS cert for testing
	tlsCert, caCertPEM := generateSelfSignedCert(t)

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	// Upstream mock server goroutine reading cleartext then performing TLS handshake
	go func() {
		// 1. Cleartext phase
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

		// 2. TLS phase: wrap serverPipe in TLS server
		tlsServer := cryptotls.Server(serverPipe, &cryptotls.Config{
			Certificates: []cryptotls.Certificate{tlsCert},
			NextProtos:   []string{"test-proto"},
		})
		if err := tlsServer.Handshake(); err != nil {
			return
		}

		// 3. Encrypted echo phase
		echoBuf := make([]byte, 256)
		en, err := tlsServer.Read(echoBuf)
		if err == nil && en > 0 {
			_, _ = tlsServer.Write(echoBuf[:en])
		}
	}()

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
		t.Fatalf("DialContext failed: %v", err)
	}
	defer conn.Close()

	inStreamConn, ok := conn.(netconn.InStreamConn)
	if !ok {
		t.Fatalf("expected InStreamConn, got %T", conn)
	}

	// 1. Cleartext exchange
	if _, err := inStreamConn.Write([]byte("STARTTLS\n")); err != nil {
		t.Fatalf("write STARTTLS failed: %v", err)
	}

	replyBuf := make([]byte, 128)
	rn, err := inStreamConn.Read(replyBuf)
	if err != nil {
		t.Fatalf("read reply failed: %v", err)
	}
	if string(replyBuf[:rn]) != "220 Ready for TLS\n" {
		t.Fatalf("unexpected cleartext reply: %s", string(replyBuf[:rn]))
	}

	// 2. In-stream TLS Upgrade
	upgradeOpts := &sesametls.TLSOptions{
		ServerName:         "mail.internal",
		AlpnProtocols:      []string{"test-proto"},
		CaCertificates:     caCertPEM,
		InsecureSkipVerify: true,
	}

	tlsRes, err := inStreamConn.UpgradeTLS(ctx, upgradeOpts)
	if err != nil {
		t.Fatalf("UpgradeTLS failed: %v", err)
	}
	if tlsRes == nil {
		t.Fatal("expected non-nil TLSHandshakeResult")
	}
	if tlsRes.GetNegotiatedProtocol() != "test-proto" {
		t.Errorf("expected negotiated protocol test-proto, got %q", tlsRes.GetNegotiatedProtocol())
	}

	// 3. Post-upgrade encrypted data exchange
	testPayload := []byte("hello encrypted world")
	if _, err := inStreamConn.Write(testPayload); err != nil {
		t.Fatalf("write post-upgrade failed: %v", err)
	}

	echoBack := make([]byte, len(testPayload))
	if _, err := inStreamConn.Read(echoBack); err != nil {
		t.Fatalf("read echo failed: %v", err)
	}
	if string(echoBack) != string(testPayload) {
		t.Errorf("echo got %q, want %q", string(echoBack), string(testPayload))
	}
}

func TestClientServer_Dial_TLSTermination(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	tlsCert, caCertPEM := generateSelfSignedCert(t)

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Upstream mock TLS server that negotiates "h2" via ALPN
	go func() {
		tlsServer := cryptotls.Server(serverPipe, &cryptotls.Config{
			Certificates: []cryptotls.Certificate{tlsCert},
			NextProtos:   []string{"h2", "http/1.1"},
		})
		if err := tlsServer.Handshake(); err != nil {
			return
		}

		buf := make([]byte, 128)
		n, err := tlsServer.Read(buf)
		if err == nil && n > 0 {
			_, _ = tlsServer.Write(buf[:n])
		}
	}()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		TLS: &sesametls.TLSOptions{
			ServerName:         "mail.internal",
			AlpnProtocols:      []string{"h2", "http/1.1"},
			CaCertificates:     caCertPEM,
			InsecureSkipVerify: true,
		},
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsOpportunisticTls: true,
			SupportsFlowControl:      true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", "mail.internal:443")
	if err != nil {
		t.Fatalf("DialContext with TLS termination failed: %v", err)
	}
	defer conn.Close()

	inStreamConn, ok := conn.(netconn.InStreamConn)
	if !ok {
		t.Fatalf("expected InStreamConn, got %T", conn)
	}

	tlsRes := inStreamConn.TLSResult()
	if tlsRes == nil {
		t.Fatal("expected non-nil TLSResult from Dial with TLS")
	}
	if tlsRes.GetNegotiatedProtocol() != "h2" {
		t.Errorf("negotiated protocol got %q, want h2", tlsRes.GetNegotiatedProtocol())
	}
	if tlsRes.GetServerName() != "mail.internal" {
		t.Errorf("server name got %q, want mail.internal", tlsRes.GetServerName())
	}

	// Application data exchange
	payload := []byte("ping payload")
	if _, err := inStreamConn.Write(payload); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	echo := make([]byte, len(payload))
	if _, err := inStreamConn.Read(echo); err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(echo) != string(payload) {
		t.Errorf("echo got %q, want %q", string(echo), string(payload))
	}
}

// recordingNetConnClient delegates to a real NetConn stream while recording
// the length of every NetConnRequest_Bytes payload sent through it.
type recordingNetConnClient struct {
	rc.RemoteControl_NetConnClient

	mu      sync.Mutex
	lengths []int
}

func (r *recordingNetConnClient) Send(req *rc.NetConnRequest) error {
	if b, ok := req.GetData().(*rc.NetConnRequest_Bytes); ok {
		r.mu.Lock()
		r.lengths = append(r.lengths, len(b.Bytes))
		r.mu.Unlock()
	}
	return r.RemoteControl_NetConnClient.Send(req)
}

func TestClientControl_WriteRespectsMaxChunkSize(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Concurrently drain the mock upstream so client writes cannot stall
	// on the net.Pipe rendezvous; count received bytes.
	var received atomic.Int64
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := serverPipe.Read(buf)
			received.Add(int64(n))
			if err != nil {
				return
			}
		}
	}()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
		Capabilities: &rc.NetConnResponse_Capabilities{
			SupportsFlowControl:      true,
			SupportsOpportunisticTls: true,
			MaxChunkSize:             4096,
			InitialWindowSize:        1 << 20,
		},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	recording := &recordingNetConnClient{}
	client := netconn.Client{
		API: recordingNetConnClientFactory{
			inner:    rc.NewRemoteControlClient(gc),
			recorder: recording,
		},
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsFlowControl: true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("DialContext failed: %v", err)
	}
	defer conn.Close()

	payload := make([]byte, 64*1024)
	for i := range payload {
		payload[i] = byte(i)
	}
	if n, err := conn.Write(payload); err != nil || n != len(payload) {
		t.Fatalf("Write failed: n=%d err=%v", n, err)
	}

	// Wait for the drained bytes to arrive at the mock upstream.
	deadline := time.Now().Add(3 * time.Second)
	for received.Load() < int64(len(payload)) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := received.Load(); got != int64(len(payload)) {
		t.Fatalf("upstream received %d bytes, want %d", got, len(payload))
	}

	recording.mu.Lock()
	lengths := append([]int(nil), recording.lengths...)
	recording.mu.Unlock()

	if len(lengths) == 0 {
		t.Fatal("no byte chunks were recorded")
	}
	for _, l := range lengths {
		if l > 4096 {
			t.Fatalf("chunk of %d bytes exceeds advertised MaxChunkSize 4096 (chunks: %v)", l, lengths)
		}
	}
	if len(lengths) < 2 {
		t.Fatalf("expected chunking for a 64KB write, got %d chunk(s)", len(lengths))
	}
}

// TestClientControl_WriteChunksAtDefaultWithoutAdvertisement verifies the
// fallback granularity: no advertised MaxChunkSize means DefaultChunkSize
// chunks even with a huge flow-control window.
func TestClientControl_WriteChunksAtDefaultWithoutAdvertisement(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	var received atomic.Int64
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := serverPipe.Read(buf)
			received.Add(int64(n))
			if err != nil {
				return
			}
		}
	}()

	// Capabilities deliberately absent -> server advertises its defaults
	// (MaxChunkSize normalized to DefaultChunkSize by Server.NetConn).
	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	recording := &recordingNetConnClient{}
	client := netconn.Client{
		API: recordingNetConnClientFactory{
			inner:    rc.NewRemoteControlClient(gc),
			recorder: recording,
		},
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsFlowControl: true,
			InitialWindowSize:   1 << 20,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("DialContext failed: %v", err)
	}
	defer conn.Close()

	payload := make([]byte, 100*1024)
	if n, err := conn.Write(payload); err != nil || n != len(payload) {
		t.Fatalf("Write failed: n=%d err=%v", n, err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for received.Load() < int64(len(payload)) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := received.Load(); got != int64(len(payload)) {
		t.Fatalf("upstream received %d bytes, want %d", got, len(payload))
	}

	recording.mu.Lock()
	lengths := append([]int(nil), recording.lengths...)
	recording.mu.Unlock()

	for _, l := range lengths {
		if l > netconn.DefaultChunkSize {
			t.Fatalf("chunk of %d bytes exceeds DefaultChunkSize %d (chunks: %v)", l, netconn.DefaultChunkSize, lengths)
		}
	}
	if len(lengths) < 3 {
		t.Fatalf("expected chunking for a 100KB write, got %d chunk(s)", len(lengths))
	}
}

// recordingNetConnClientFactory adapts a recordingNetConnClient to the
// ClientAPI interface.
type recordingNetConnClientFactory struct {
	inner    rc.RemoteControlClient
	recorder *recordingNetConnClient
}

func (f recordingNetConnClientFactory) NetConn(ctx context.Context, opts ...grpc.CallOption) (rc.RemoteControl_NetConnClient, error) {
	stream, err := f.inner.NetConn(ctx, opts...)
	if err != nil {
		return nil, err
	}
	f.recorder.RemoteControl_NetConnClient = stream
	return f.recorder, nil
}

func TestClientServer_InStream_UpgradeTLS_NilOptions(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsOpportunisticTls: true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("DialContext failed: %v", err)
	}
	defer conn.Close()

	inStreamConn, ok := conn.(netconn.InStreamConn)
	if !ok {
		t.Fatalf("expected InStreamConn, got %T", conn)
	}

	// The client must reject a nil-options upgrade locally, before
	// anything touches the wire, with InvalidArgument.
	_, err = inStreamConn.UpgradeTLS(ctx, nil)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument from nil-options UpgradeTLS, got %v", err)
	}

	// Fail closed, not fail broken: the connection must remain usable
	// for data and control traffic after the rejected upgrade.
	if _, err := inStreamConn.Write([]byte("still-alive\n")); err != nil {
		t.Fatalf("Write after rejected upgrade failed: %v", err)
	}
	buf := make([]byte, 32)
	if _, err := serverPipe.Read(buf); err != nil {
		t.Fatalf("upstream read after rejected upgrade failed: %v", err)
	}

	if _, err := inStreamConn.Ping(ctx); err != nil {
		t.Fatalf("Ping after rejected upgrade failed: %v", err)
	}
}

func TestClientServer_InStream_UpgradeTLS_NilOptions_ServerRejects(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, _ := net.Pipe()
	defer clientPipe.Close()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Hand-crafted stream: a protocol-level upgrade_tls with absent
	// options must be rejected by the server itself, not silently
	// "succeed" with a nil handshake result.
	stream, err := rc.NewRemoteControlClient(gc).NetConn(ctx)
	if err != nil {
		t.Fatalf("failed opening NetConn stream: %v", err)
	}

	if err := stream.Send(&rc.NetConnRequest{
		Data: &rc.NetConnRequest_Dial_{
			Dial: &rc.NetConnRequest_Dial{
				Address: &netaddr.NetAddr{Network: "tcp", Address: "example.com:80"},
				Capabilities: &rc.NetConnRequest_Capabilities{
					SupportsOpportunisticTls: true,
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed sending dial: %v", err)
	}

	res, err := stream.Recv()
	if err != nil {
		t.Fatalf("failed receiving conn response: %v", err)
	}
	if res.GetConn() == nil {
		t.Fatalf("expected conn response, got %T", res.GetData())
	}

	if err := stream.Send(&rc.NetConnRequest{
		Data: &rc.NetConnRequest_Control_{
			Control: &rc.NetConnRequest_Control{
				Action: &rc.NetConnRequest_Control_UpgradeTls{
					UpgradeTls: &rc.NetConnRequest_Control_UpgradeTLS{},
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed sending nil-options upgrade_tls: %v", err)
	}

	// The stream must terminate with InvalidArgument (the terminal Recv
	// surfaces the server error).
	for {
		_, err = stream.Recv()
		if err != nil {
			break
		}
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument stream termination, got %v", err)
	}
}

func TestClientServer_InStream_UpgradeTLS_DisabledByServerPolicy(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, _ := net.Pipe()
	defer clientPipe.Close()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
		Capabilities: &rc.NetConnResponse_Capabilities{
			SupportsFlowControl:      true,
			SupportsOpportunisticTls: false,
			MaxChunkSize:             4096,
			InitialWindowSize:        1 << 20,
		},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Hand-crafted stream: the upgrade carries valid options, but the
	// server advertised SupportsOpportunisticTls=false, so the server
	// must reject with FailedPrecondition before any handshake or
	// socket mutation.
	stream, err := rc.NewRemoteControlClient(gc).NetConn(ctx)
	if err != nil {
		t.Fatalf("failed opening NetConn stream: %v", err)
	}

	if err := stream.Send(&rc.NetConnRequest{
		Data: &rc.NetConnRequest_Dial_{
			Dial: &rc.NetConnRequest_Dial{
				Address: &netaddr.NetAddr{Network: "tcp", Address: "example.com:80"},
				Capabilities: &rc.NetConnRequest_Capabilities{
					SupportsOpportunisticTls: true,
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed sending dial: %v", err)
	}

	res, err := stream.Recv()
	if err != nil {
		t.Fatalf("failed receiving conn response: %v", err)
	}
	if res.GetConn() == nil {
		t.Fatalf("expected conn response, got %T", res.GetData())
	}
	if res.GetConn().GetCapabilities().GetSupportsOpportunisticTls() {
		t.Fatalf("expected server caps to disable opportunistic TLS")
	}

	if err := stream.Send(&rc.NetConnRequest{
		Data: &rc.NetConnRequest_Control_{
			Control: &rc.NetConnRequest_Control{
				Action: &rc.NetConnRequest_Control_UpgradeTls{
					UpgradeTls: &rc.NetConnRequest_Control_UpgradeTLS{
						Options: &sesametls.TLSOptions{
							ServerName: "example.com",
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed sending upgrade_tls: %v", err)
	}

	// The stream must terminate with FailedPrecondition (the terminal
	// Recv surfaces the server error).
	for {
		_, err = stream.Recv()
		if err != nil {
			break
		}
	}
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition stream termination, got %v", err)
	}
}

func TestClientServer_InStream_UpgradeTLS_Failure(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Upstream mock server that immediately closes on TLS handshake attempt
	go func() {
		buf := make([]byte, 128)
		n, err := serverPipe.Read(buf)
		if err != nil {
			return
		}
		if string(buf[:n]) == "STARTTLS\n" {
			_, _ = serverPipe.Write([]byte("220 Ready\n"))
		}
		// Close abruptly when TLS client hello arrives
		_ = serverPipe.Close()
	}()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsOpportunisticTls: true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", "mail.internal:25")
	if err != nil {
		t.Fatalf("DialContext failed: %v", err)
	}
	defer conn.Close()

	inStreamConn, ok := conn.(netconn.InStreamConn)
	if !ok {
		t.Fatalf("expected InStreamConn, got %T", conn)
	}

	// Read greeting
	if _, err := inStreamConn.Write([]byte("STARTTLS\n")); err != nil {
		t.Fatalf("write STARTTLS failed: %v", err)
	}
	greeting := make([]byte, 128)
	if _, err := inStreamConn.Read(greeting); err != nil {
		t.Fatalf("read greeting failed: %v", err)
	}

	// In-stream TLS upgrade should fail
	_, err = inStreamConn.UpgradeTLS(ctx, &sesametls.TLSOptions{
		ServerName:         "mail.internal",
		InsecureSkipVerify: true,
	})
	if err == nil {
		t.Fatal("expected UpgradeTLS error when upstream abruptly drops, got nil")
	}

	// Verify fail-closed: connection must not be usable in cleartext
	buf := make([]byte, 32)
	_, readErr := inStreamConn.Read(buf)
	if readErr == nil {
		t.Fatal("expected read error on failed connection, got nil")
	}
}

func TestClientServer_InStream_FlowControl(t *testing.T) {
	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()

	// Upstream echo server
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := serverPipe.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				_, _ = serverPipe.Write(buf[:n])
			}
		}
	}()

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	defer gc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsFlowControl: true,
			InitialWindowSize:   100, // Small window to test window updates
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", "echo.internal:80")
	if err != nil {
		t.Fatalf("DialContext failed: %v", err)
	}
	defer conn.Close()

	// Send data larger than initial window (e.g. 250 bytes) in chunks
	data := make([]byte, 250)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if _, err := conn.Write(data); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	received := make([]byte, len(data))
	var totalRead int
	for totalRead < len(data) {
		n, err := conn.Read(received[totalRead:])
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		totalRead += n
	}

	for i := range data {
		if received[i] != data[i] {
			t.Fatalf("byte %d mismatch: got %v want %v", i, received[i], data[i])
		}
	}
}

type mockPipeDialer struct {
	conn net.Conn
}

func (m *mockPipeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return m.conn, nil
}

type mockServerWithoutTLS struct {
	dialer netconn.Dialer
	rc.UnimplementedRemoteControlServer
}

func (m *mockServerWithoutTLS) NetConn(stream rc.RemoteControl_NetConnServer) error {
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

func generateSelfSignedCert(t *testing.T) (cryptotls.Certificate, []byte) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed generating key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "mail.internal",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"mail.internal", "example.com"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed creating cert: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed marshaling key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	tlsCert, err := cryptotls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed X509KeyPair: %v", err)
	}

	return tlsCert, certPEM
}

// newDeadlineTestConn dials a control-mode conn against an echo upstream and
// returns it alongside the raw mock-upstream side.
func newDeadlineTestConn(t *testing.T) (netconn.InStreamConn, net.Conn) {
	t.Helper()

	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, serverPipe := net.Pipe()
	t.Cleanup(func() {
		_ = clientPipe.Close()
		_ = serverPipe.Close()
	})

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	t.Cleanup(func() { _ = gc.Close() })

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsFlowControl: true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	conn, err := client.DialContext(ctx, "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("DialContext failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return conn.(netconn.InStreamConn), serverPipe
}

// TestClientControl_ReadDeadline_Expires verifies a blocked Read returns
// os.ErrDeadlineExceeded promptly when the read deadline expires.
func TestClientControl_ReadDeadline_Expires(t *testing.T) {
	conn, _ := newDeadlineTestConn(t)

	if err := conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}

	start := time.Now()
	buf := make([]byte, 32)
	_, err := conn.Read(buf)
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("expected os.ErrDeadlineExceeded from blocked Read, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Read unblocked too slowly: %v", elapsed)
	}
}

// TestClientControl_ReadDeadline_ClearRestoresBlocking verifies clearing the
// deadline restores blocking semantics: bytes pushed after the would-be
// deadline are still readable.
func TestClientControl_ReadDeadline_ClearRestoresBlocking(t *testing.T) {
	conn, upstream := newDeadlineTestConn(t)

	if err := conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}
	buf := make([]byte, 32)
	if _, err := conn.Read(buf); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}

	// Clear the deadline and push bytes AFTER the would-be deadline.
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		t.Fatalf("clearing SetReadDeadline failed: %v", err)
	}
	time.Sleep(80 * time.Millisecond) // past the original deadline

	if _, err := upstream.Write([]byte("post-deadline\n")); err != nil {
		t.Fatalf("upstream write failed: %v", err)
	}

	got := make([]byte, len("post-deadline\n"))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("Read after deadline clear failed: %v", err)
	}
	if string(got) != "post-deadline\n" {
		t.Fatalf("unexpected data: %q", got)
	}
}

// TestClientControl_WriteDeadline_ExpiresOnExhaustedWindow verifies Write
// blocked on an exhausted flow-control window returns os.ErrDeadlineExceeded
// when the write deadline expires.
func TestClientControl_WriteDeadline_ExpiresOnExhaustedWindow(t *testing.T) {
	conn, upstream := newDeadlineTestConn(t)

	// Drain the whole outbound window without letting the upstream
	// consume anything: window is 65535 by default, so a larger single
	// Write cannot complete... but Write sends chunks as it acquires
	// credit, which drains the window onto the wire. Block the upstream
	// read side so the server's own inbound path (and window updates in
	// the reverse direction) cannot advance.
	//
	// Simpler deterministic exhaustion: advertise a small window, write
	// exactly window bytes (succeeds), then write more - the second
	// Write blocks with zero credit and no reader on the upstream side
	// to generate window updates.

	// Exhaust: default window 65535; first Write of 65535 succeeds
	// (chunks flow to the server's inbound FC, which refunds via
	// windowUpdate only after c.Write to the mock upstream succeeds -
	// and the mock upstream IS read by nobody here, but server-side
	// refund happens after writing to activeConn, which is the blocked
	// net.Pipe... so the server refund is blocked too).
	first := make([]byte, 65535)
	if n, err := conn.Write(first); err != nil || n != len(first) {
		t.Fatalf("window-exhausting Write failed: n=%d err=%v", n, err)
	}
	_ = upstream

	if err := conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("SetWriteDeadline failed: %v", err)
	}

	start := time.Now()
	_, err := conn.Write([]byte("blocked-on-zero-credit"))
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("expected os.ErrDeadlineExceeded from blocked Write, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Write unblocked too slowly: %v", elapsed)
	}
}

// TestClientControl_WriteDeadline_PastFailsImmediately verifies a write
// deadline in the past makes the next Write fail immediately with
// os.ErrDeadlineExceeded (net.Conn testPastTimeout semantics).
func TestClientControl_WriteDeadline_PastFailsImmediately(t *testing.T) {
	conn, _ := newDeadlineTestConn(t)

	if err := conn.SetWriteDeadline(time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("SetWriteDeadline failed: %v", err)
	}

	start := time.Now()
	_, err := conn.Write([]byte("should not go anywhere"))
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("expected os.ErrDeadlineExceeded from past-deadline Write, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("past-deadline Write blocked for %v, expected immediate failure", elapsed)
	}
}

// blockingTLSProvider is a Server.TLSProvider whose handshake blocks until
// released, letting tests control exactly when TlsUpgraded/TlsUpgradeFailed
// is emitted.
type blockingTLSProvider struct {
	release chan struct{}
	once    sync.Once
}

func (p *blockingTLSProvider) Handshake(ctx context.Context, rawConn net.Conn, opts *sesametls.TLSOptions) (net.Conn, *sesametls.TLSHandshakeResult, error) {
	select {
	case <-p.release:
		return rawConn, &sesametls.TLSHandshakeResult{ServerName: opts.GetServerName()}, nil
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

func (p *blockingTLSProvider) unblock() { p.once.Do(func() { close(p.release) }) }

// dialControlConnWithProvider is newDeadlineTestConn's harness plus a
// Server.TLSProvider and a longer-lived dial ctx.
func dialControlConnWithProvider(t *testing.T, provider netconn.TLSProvider) (netconn.InStreamConn, net.Conn) {
	t.Helper()

	ccFactory := testutil.ClientConnFactories["inprocgrpc"]
	if ccFactory == nil {
		t.Skip("inprocgrpc factory unavailable")
	}

	clientPipe, serverPipe := net.Pipe()
	t.Cleanup(func() {
		_ = clientPipe.Close()
		_ = serverPipe.Close()
	})

	server := netconn.Server{
		Dialer: func(req *rc.NetConnRequest_Dial) (netconn.Dialer, error) {
			return &mockPipeDialer{conn: clientPipe}, nil
		},
		TLSProvider: provider,
	}

	gc := ccFactory(func(h testutil.GRPCServer) {
		rc.RegisterRemoteControlServer(h, &server)
	})
	t.Cleanup(func() { _ = gc.Close() })

	client := netconn.Client{
		API: rc.NewRemoteControlClient(gc),
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsFlowControl:      true,
			SupportsOpportunisticTls: true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	conn, err := client.DialContext(ctx, "tcp", "example.com:80")
	if err != nil {
		t.Fatalf("DialContext failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return conn.(netconn.InStreamConn), serverPipe
}

// TestClientServer_UpgradeTLS_TimeoutClearsPending: a timed-out UpgradeTLS
// must clear the pending slot so a subsequent UpgradeTLS does not return
// AlreadyExists for a dead request.
func TestClientServer_UpgradeTLS_TimeoutClearsPending(t *testing.T) {
	provider := &blockingTLSProvider{release: make(chan struct{})}
	conn, _ := dialControlConnWithProvider(t, provider)

	upgradeCtx, upgradeCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer upgradeCancel()

	_, err := conn.UpgradeTLS(upgradeCtx, &sesametls.TLSOptions{ServerName: "example.com"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded from blocked upgrade, got %v", err)
	}

	// The pending slot must be clear: a second UpgradeTLS must not fail
	// with AlreadyExists (it gets a fresh pending registration and then
	// hits its own short timeout).
	secondCtx, secondCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer secondCancel()
	_, err = conn.UpgradeTLS(secondCtx, &sesametls.TLSOptions{ServerName: "example.com"})
	if status.Code(err) == codes.AlreadyExists {
		t.Fatalf("second UpgradeTLS failed with AlreadyExists; pending slot was not cleared: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded from second blocked upgrade, got %v", err)
	}
}

// TestClientServer_UpgradeTLS_LateResultPoisons: after a timed-out upgrade,
// a late TlsUpgraded arriving with no waiter must tear the connection down -
// subsequent Read/Write/UpgradeTLS all fail rather than continuing cleartext
// against a peer that switched to TLS. Teardown must happen promptly: a
// healthy connection only ever yields our own probe deadlines.
func TestClientServer_UpgradeTLS_LateResultPoisons(t *testing.T) {
	provider := &blockingTLSProvider{release: make(chan struct{})}
	conn, _ := dialControlConnWithProvider(t, provider)

	upgradeCtx, upgradeCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer upgradeCancel()

	_, err := conn.UpgradeTLS(upgradeCtx, &sesametls.TLSOptions{ServerName: "example.com"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded from blocked upgrade, got %v", err)
	}

	// Release the provider so the server completes the upgrade and emits
	// TlsUpgraded with nobody waiting for it.
	provider.unblock()

	// Probe with a read deadline: on a torn-down conn, Read fails with a
	// close error; on a healthy conn it can only time out with
	// os.ErrDeadlineExceeded (no inbound data is flowing).
	var tornDown bool
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, rerr := conn.Read(make([]byte, 16))
		_ = conn.SetReadDeadline(time.Time{})
		if rerr != nil && !errors.Is(rerr, os.ErrDeadlineExceeded) {
			tornDown = true
			break
		}
	}
	if !tornDown {
		t.Fatal("connection was not torn down within 2s of the unsolicited TlsUpgraded event")
	}

	// With teardown detected, every subsequent operation must fail.
	if _, werr := conn.Write([]byte("probe")); werr == nil {
		t.Fatal("expected Write to fail on poisoned connection, got nil")
	}
	thirdCtx, thirdCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer thirdCancel()
	if _, uerr := conn.UpgradeTLS(thirdCtx, &sesametls.TLSOptions{ServerName: "example.com"}); uerr == nil {
		t.Fatal("expected UpgradeTLS to fail on poisoned connection, got nil")
	}
}

// TestClientServer_TLSResult_ConcurrentAccessDuringUpgrade: TLSResult reads
// racing a completing upgrade must be race-clean.
func TestClientServer_TLSResult_ConcurrentAccessDuringUpgrade(t *testing.T) {
	provider := &blockingTLSProvider{release: make(chan struct{})}
	conn, _ := dialControlConnWithProvider(t, provider)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Repeatedly read TLSResult from a separate goroutine while
		// the upgrade completes.
		for i := 0; i < 2000; i++ {
			_ = conn.TLSResult()
		}
	}()

	// Release the handshake shortly after the upgrade request is in
	// flight so the completing write to tlsResult races the reader.
	go func() {
		time.Sleep(20 * time.Millisecond)
		provider.unblock()
	}()

	upCtx, upCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer upCancel()
	if _, err := conn.UpgradeTLS(upCtx, &sesametls.TLSOptions{ServerName: "example.com"}); err != nil {
		t.Fatalf("UpgradeTLS failed: %v", err)
	}
	wg.Wait()
}
