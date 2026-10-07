package netconn_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	cryptotls "crypto/tls"
	"github.com/joeycumines/sesame/rc"
	"github.com/joeycumines/sesame/rc/netconn"
	sesametls "github.com/joeycumines/sesame/rc/tls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestClientServer_SesameEndpoint_Interop(t *testing.T) {
	// 1. Locate sesame-endpoint built CLI
	cliPath, err := filepath.Abs("../../sesame-endpoint/build/src/cli.js")
	if err != nil {
		t.Fatalf("failed resolving cli path: %v", err)
	}
	if _, err := os.Stat(cliPath); os.IsNotExist(err) {
		t.Skipf("sesame-endpoint CLI not built at %s; skipping interop test", cliPath)
	}

	// 2. Start a TCP Echo Server in Go
	echoListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed starting echo listener: %v", err)
	}
	defer echoListener.Close()

	go func() {
		for {
			c, err := echoListener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(c)
		}
	}()

	echoAddr := echoListener.Addr().String()

	// 3. Launch sesame-endpoint server on available port
	freeL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed finding free port: %v", err)
	}
	freePort := freeL.Addr().(*net.TCPAddr).Port
	_ = freeL.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", cliPath, "--host", "127.0.0.1", "--port", fmt.Sprintf("%d", freePort))
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("failed getting stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed starting sesame-endpoint: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// 4. Scan stderr for bound port
	portRegex := regexp.MustCompile(`sesame-endpoint listening on 127\.0\.0\.1:(\d+)`)
	scanner := bufio.NewScanner(stderrPipe)
	var endpointPort string
	for scanner.Scan() {
		line := scanner.Text()
		if matches := portRegex.FindStringSubmatch(line); len(matches) > 1 {
			endpointPort = matches[1]
			break
		}
	}

	if endpointPort == "" {
		t.Fatalf("failed finding bound port from sesame-endpoint output")
	}

	// 5. Connect Go gRPC client to sesame-endpoint HTTP/2 server
	endpointAddr := fmt.Sprintf("127.0.0.1:%s", endpointPort)
	cc, err := grpc.NewClient(endpointAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed creating gRPC client to %s: %v", endpointAddr, err)
	}
	defer cc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(cc),
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsOpportunisticTls: true,
			SupportsFlowControl:      true,
		},
	}

	// 6. Dial through sesame-endpoint to the echo server
	dialCtx, dialCancel := context.WithTimeout(ctx, 3*time.Second)
	defer dialCancel()

	conn, err := client.DialContext(dialCtx, "tcp", echoAddr)
	if err != nil {
		t.Fatalf("DialContext failed through sesame-endpoint: %v", err)
	}
	defer conn.Close()

	// 7. Verify InStreamConn interface
	inStreamConn, ok := conn.(netconn.InStreamConn)
	if !ok {
		t.Fatalf("expected InStreamConn, got %T", conn)
	}

	// 8. Test Data Echo Roundtrip
	msg := []byte("Hello sesame-endpoint from Go client!\n")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("ReadFull failed: %v", err)
	}

	if string(buf) != string(msg) {
		t.Errorf("echo mismatch: got %q, want %q", string(buf), string(msg))
	}

	// 9. Test In-Stream Ping / Pong RTT
	pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pingCancel()

	rtt, err := inStreamConn.Ping(pingCtx)
	if err != nil {
		t.Fatalf("in-stream Ping failed: %v", err)
	}
	if rtt <= 0 {
		t.Errorf("expected positive RTT, got %v", rtt)
	}

	// 10. Verify Server Capabilities reported
	caps := inStreamConn.ServerCapabilities()
	if caps == nil {
		t.Fatal("expected non-nil server capabilities")
	}
	if !caps.GetSupportsFlowControl() || !caps.GetSupportsOpportunisticTls() {
		t.Errorf("expected flow control and opportunistic tls supported, got: %+v", caps)
	}
}

func TestClientServer_SesameEndpoint_TLS_Interop(t *testing.T) {
	cliPath, err := filepath.Abs("../../sesame-endpoint/build/src/cli.js")
	if err != nil {
		t.Fatalf("failed resolving cli path: %v", err)
	}
	if _, err := os.Stat(cliPath); os.IsNotExist(err) {
		t.Skipf("sesame-endpoint CLI not built at %s; skipping interop test", cliPath)
	}

	tlsCert, _ := generateSelfSignedCert(t)
	tlsListener, err := cryptotls.Listen("tcp", "127.0.0.1:0", &cryptotls.Config{
		Certificates: []cryptotls.Certificate{tlsCert},
		NextProtos:   []string{"test-interop", "h2"},
	})
	if err != nil {
		t.Fatalf("failed starting TLS listener: %v", err)
	}
	defer tlsListener.Close()

	go func() {
		for {
			c, err := tlsListener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(c)
		}
	}()

	tlsAddr := tlsListener.Addr().String()

	freeL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed finding free port: %v", err)
	}
	freePort := freeL.Addr().(*net.TCPAddr).Port
	_ = freeL.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", cliPath, "--host", "127.0.0.1", "--port", fmt.Sprintf("%d", freePort))
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("failed getting stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed starting sesame-endpoint: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	portRegex := regexp.MustCompile(`sesame-endpoint listening on 127\.0\.0\.1:(\d+)`)
	scanner := bufio.NewScanner(stderrPipe)
	var endpointPort string
	for scanner.Scan() {
		line := scanner.Text()
		if matches := portRegex.FindStringSubmatch(line); len(matches) > 1 {
			endpointPort = matches[1]
			break
		}
	}

	if endpointPort == "" {
		t.Fatalf("failed finding bound port from sesame-endpoint output")
	}

	endpointAddr := fmt.Sprintf("127.0.0.1:%s", endpointPort)
	cc, err := grpc.NewClient(endpointAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed creating gRPC client to %s: %v", endpointAddr, err)
	}
	defer cc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(cc),
		TLS: &sesametls.TLSOptions{
			ServerName:         "localhost",
			AlpnProtocols:      []string{"test-interop"},
			InsecureSkipVerify: true,
		},
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsOpportunisticTls: true,
			SupportsFlowControl:      true,
		},
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, 3*time.Second)
	defer dialCancel()

	conn, err := client.DialContext(dialCtx, "tcp", tlsAddr)
	if err != nil {
		t.Fatalf("DialContext with TLS failed: %v", err)
	}
	defer conn.Close()

	if _, ok := conn.(netconn.InStreamConn); !ok {
		t.Fatalf("expected InStreamConn, got %T", conn)
	}

	transformRes, ok := conn.(netconn.ConnTransformResult)
	if !ok {
		t.Fatalf("expected ConnTransformResult, got %T", conn)
	}

	tlsResult := transformRes.TLSHandshakeResult()
	if tlsResult == nil {
		t.Fatal("expected non-nil TLS handshake result in Conn")
	}
	if tlsResult.GetNegotiatedProtocol() != "test-interop" {
		t.Errorf("negotiated protocol = %q, want test-interop", tlsResult.GetNegotiatedProtocol())
	}

	msg := []byte("Secret encrypted payload over endpoint-terminated TLS!\n")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("ReadFull failed: %v", err)
	}

	if string(buf) != string(msg) {
		t.Errorf("echo mismatch: got %q, want %q", string(buf), string(msg))
	}
}

func TestClientServer_SesameEndpoint_STARTTLS_Interop(t *testing.T) {
	cliPath, err := filepath.Abs("../../sesame-endpoint/build/src/cli.js")
	if err != nil {
		t.Fatalf("failed resolving cli path: %v", err)
	}
	if _, err := os.Stat(cliPath); os.IsNotExist(err) {
		t.Skipf("sesame-endpoint CLI not built at %s; skipping interop test", cliPath)
	}

	tlsCert, _ := generateSelfSignedCert(t)
	starttlsListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed starting TCP listener: %v", err)
	}
	defer starttlsListener.Close()

	go func() {
		for {
			c, err := starttlsListener.Accept()
			if err != nil {
				return
			}
			go func(rawConn net.Conn) {
				defer rawConn.Close()
				buf := make([]byte, 128)
				n, err := rawConn.Read(buf)
				if err != nil || string(buf[:n]) != "STARTTLS\n" {
					return
				}
				if _, err := rawConn.Write([]byte("220 Ready for TLS\n")); err != nil {
					return
				}
				tlsConn := cryptotls.Server(rawConn, &cryptotls.Config{
					Certificates: []cryptotls.Certificate{tlsCert},
					NextProtos:   []string{"starttls-interop"},
				})
				if err := tlsConn.Handshake(); err != nil {
					return
				}
				_, _ = io.Copy(tlsConn, tlsConn)
			}(c)
		}
	}()

	starttlsAddr := starttlsListener.Addr().String()

	freeL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed finding free port: %v", err)
	}
	freePort := freeL.Addr().(*net.TCPAddr).Port
	_ = freeL.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", cliPath, "--host", "127.0.0.1", "--port", fmt.Sprintf("%d", freePort))
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("failed getting stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed starting sesame-endpoint: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	portRegex := regexp.MustCompile(`sesame-endpoint listening on 127\.0\.0\.1:(\d+)`)
	scanner := bufio.NewScanner(stderrPipe)
	var endpointPort string
	for scanner.Scan() {
		line := scanner.Text()
		if matches := portRegex.FindStringSubmatch(line); len(matches) > 1 {
			endpointPort = matches[1]
			break
		}
	}

	if endpointPort == "" {
		t.Fatalf("failed finding bound port from sesame-endpoint output")
	}

	endpointAddr := fmt.Sprintf("127.0.0.1:%s", endpointPort)
	cc, err := grpc.NewClient(endpointAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed creating gRPC client to %s: %v", endpointAddr, err)
	}
	defer cc.Close()

	client := netconn.Client{
		API: rc.NewRemoteControlClient(cc),
		Capabilities: &rc.NetConnRequest_Capabilities{
			SupportsOpportunisticTls: true,
			SupportsFlowControl:      true,
		},
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, 3*time.Second)
	defer dialCancel()

	conn, err := client.DialContext(dialCtx, "tcp", starttlsAddr)
	if err != nil {
		t.Fatalf("DialContext failed: %v", err)
	}
	defer conn.Close()

	inStreamConn, ok := conn.(netconn.InStreamConn)
	if !ok {
		t.Fatalf("expected InStreamConn, got %T", conn)
	}

	// 1. Cleartext phase
	if _, err := inStreamConn.Write([]byte("STARTTLS\n")); err != nil {
		t.Fatalf("Write STARTTLS failed: %v", err)
	}

	clearBuf := make([]byte, 18)
	if _, err := io.ReadFull(inStreamConn, clearBuf); err != nil {
		t.Fatalf("Read cleartext response failed: %v", err)
	}
	if string(clearBuf) != "220 Ready for TLS\n" {
		t.Fatalf("unexpected cleartext response: %q", string(clearBuf))
	}

	// 2. Perform in-stream UpgradeTLS
	upgradeCtx, upgradeCancel := context.WithTimeout(ctx, 3*time.Second)
	defer upgradeCancel()

	res, err := inStreamConn.UpgradeTLS(upgradeCtx, &sesametls.TLSOptions{
		ServerName:         "localhost",
		AlpnProtocols:      []string{"starttls-interop"},
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatalf("UpgradeTLS failed: %v", err)
	}
	if res == nil || res.GetNegotiatedProtocol() != "starttls-interop" {
		t.Fatalf("unexpected TLS result after upgrade: %+v", res)
	}

	// 3. Encrypted phase
	secretMsg := []byte("Encrypted data exchanged post-STARTTLS!\n")
	if _, err := inStreamConn.Write(secretMsg); err != nil {
		t.Fatalf("Write encrypted failed: %v", err)
	}

	encBuf := make([]byte, len(secretMsg))
	if _, err := io.ReadFull(inStreamConn, encBuf); err != nil {
		t.Fatalf("Read encrypted failed: %v", err)
	}
	if string(encBuf) != string(secretMsg) {
		t.Fatalf("echo mismatch: got %q, want %q", string(encBuf), string(secretMsg))
	}
}
