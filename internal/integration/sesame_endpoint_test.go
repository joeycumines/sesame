package integration_test

import (
	"bufio"
	"context"
	cryptotls "crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/joeycumines/sesame/rc"
	"github.com/joeycumines/sesame/rc/netconn"
	sesametls "github.com/joeycumines/sesame/type/tls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func findSesameEndpointCLI(t *testing.T) string {
	t.Helper()
	cliPath, err := filepath.Abs("../../sesame-endpoint/build/src/cli.js")
	if err != nil {
		t.Fatalf("failed resolving cli path: %v", err)
	}
	if _, err := os.Stat(cliPath); os.IsNotExist(err) {
		t.Skipf("sesame-endpoint CLI not built at %s; run 'bun run compile' first", cliPath)
	}
	return cliPath
}

// TestSubprocess_SesameEndpoint_Node runs full integration flows against sesame-endpoint running under Node.js.
func TestSubprocess_SesameEndpoint_Node(t *testing.T) {
	skipUnlessIntegration(t)
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found in PATH")
	}
	runSesameEndpointSubprocessSuite(t, "node")
}

// TestSubprocess_SesameEndpoint_Bun runs full integration flows against sesame-endpoint running under Bun.
func TestSubprocess_SesameEndpoint_Bun(t *testing.T) {
	skipUnlessIntegration(t)
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not found in PATH")
	}
	runSesameEndpointSubprocessSuite(t, "bun")
}

func runSesameEndpointSubprocessSuite(t *testing.T, runtimeBin string) {
	cliPath := findSesameEndpointCLI(t)

	// 1. Start Go TCP echo server
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

	certPEM, keyPEM, _ := generateCertAndKey(t, "localhost")
	tlsCert, err := cryptotls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed creating key pair: %v", err)
	}
	tlsListener, err := cryptotls.Listen("tcp", "127.0.0.1:0", &cryptotls.Config{
		Certificates: []cryptotls.Certificate{tlsCert},
		NextProtos:   []string{"h2", "integration-alpn"},
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

	// 3. Find free port and spawn sesame-endpoint subprocess
	freeL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed finding free port: %v", err)
	}
	freePort := freeL.Addr().(*net.TCPAddr).Port
	_ = freeL.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, runtimeBin, cliPath, "--host", "127.0.0.1", "--port", fmt.Sprintf("%d", freePort))
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("failed getting stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed starting %s %s: %v", runtimeBin, cliPath, err)
	}

	// 4. Scan stderr for bound port confirmation
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
		_ = cmd.Process.Kill()
		t.Fatalf("failed finding bound port from %s sesame-endpoint output", runtimeBin)
	}

	// Cleanup hook to test graceful shutdown
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGINT)
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case <-time.After(2 * time.Second):
				_ = cmd.Process.Kill()
			case err := <-done:
				if err != nil {
					t.Logf("subprocess %s exited with: %v", runtimeBin, err)
				}
			}
		}
	}()

	// 5. Connect gRPC client to sesame-endpoint HTTP/2 server
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

	// Test A: Plaintext TCP echo
	t.Run("Plaintext_Echo", func(t *testing.T) {
		dialCtx, dialCancel := context.WithTimeout(ctx, 3*time.Second)
		defer dialCancel()

		conn, err := client.DialContext(dialCtx, "tcp", echoListener.Addr().String())
		if err != nil {
			t.Fatalf("DialContext failed: %v", err)
		}
		defer conn.Close()

		msg := []byte(fmt.Sprintf("Hello from Go via %s sesame-endpoint!\n", runtimeBin))
		if _, err := conn.Write(msg); err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		reply := make([]byte, len(msg))
		if _, err := io.ReadFull(conn, reply); err != nil {
			t.Fatalf("ReadFull failed: %v", err)
		}
		if string(reply) != string(msg) {
			t.Errorf("echo mismatch: got %q, want %q", string(reply), string(msg))
		}
	})

	// Test B: Endpoint TLS termination with ALPN
	t.Run("TLS_Termination_ALPN", func(t *testing.T) {
		dialCtx, dialCancel := context.WithTimeout(ctx, 3*time.Second)
		defer dialCancel()

		tlsClient := netconn.Client{
			API: rc.NewRemoteControlClient(cc),
			TLS: &sesametls.TLSOptions{
				ServerName:         "localhost",
				AlpnProtocols:      []string{"integration-alpn"},
				InsecureSkipVerify: true,
			},
			Capabilities: &rc.NetConnRequest_Capabilities{
				SupportsOpportunisticTls: true,
				SupportsFlowControl:      true,
			},
		}

		conn, err := tlsClient.DialContext(dialCtx, "tcp", tlsListener.Addr().String())
		if err != nil {
			t.Fatalf("DialContext with TLS failed: %v", err)
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
		if tlsRes.GetNegotiatedProtocol() != "integration-alpn" {
			t.Errorf("got negotiated protocol %q, want integration-alpn", tlsRes.GetNegotiatedProtocol())
		}

		msg := []byte("Encrypted payload roundtrip over endpoint TLS!\n")
		if _, err := inStreamConn.Write(msg); err != nil {
			t.Fatalf("Write: %v", err)
		}
		reply := make([]byte, len(msg))
		if _, err := io.ReadFull(inStreamConn, reply); err != nil {
			t.Fatalf("ReadFull: %v", err)
		}
		if string(reply) != string(msg) {
			t.Errorf("echo mismatch: got %q, want %q", string(reply), string(msg))
		}
	})

	// Test C: In-Stream STARTTLS Upgrade
	t.Run("InStream_STARTTLS", func(t *testing.T) {
		starttlsListener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed starting starttls listener: %v", err)
		}
		defer starttlsListener.Close()

		go func() {
			for {
				c, err := starttlsListener.Accept()
				if err != nil {
					return
				}
				go func(raw net.Conn) {
					defer raw.Close()
					buf := make([]byte, 128)
					n, err := raw.Read(buf)
					if err != nil || string(buf[:n]) != "STARTTLS\n" {
						return
					}
					if _, err := raw.Write([]byte("220 Ready for TLS\n")); err != nil {
						return
					}
					tlsConn := cryptotls.Server(raw, &cryptotls.Config{
						Certificates: []cryptotls.Certificate{tlsCert},
						NextProtos:   []string{"starttls-subprocess"},
					})
					if err := tlsConn.Handshake(); err != nil {
						return
					}
					_, _ = io.Copy(tlsConn, tlsConn)
				}(c)
			}
		}()

		dialCtx, dialCancel := context.WithTimeout(ctx, 3*time.Second)
		defer dialCancel()

		conn, err := client.DialContext(dialCtx, "tcp", starttlsListener.Addr().String())
		if err != nil {
			t.Fatalf("DialContext failed: %v", err)
		}
		defer conn.Close()

		inStreamConn, ok := conn.(netconn.InStreamConn)
		if !ok {
			t.Fatalf("expected InStreamConn, got %T", conn)
		}

		// Cleartext exchange
		if _, err := inStreamConn.Write([]byte("STARTTLS\n")); err != nil {
			t.Fatalf("Write STARTTLS: %v", err)
		}
		clearReply := make([]byte, 18)
		if _, err := io.ReadFull(inStreamConn, clearReply); err != nil {
			t.Fatalf("Read cleartext: %v", err)
		}
		if string(clearReply) != "220 Ready for TLS\n" {
			t.Fatalf("unexpected cleartext reply: %q", string(clearReply))
		}

		// Perform in-stream UpgradeTLS
		res, err := inStreamConn.UpgradeTLS(dialCtx, &sesametls.TLSOptions{
			ServerName:         "localhost",
			AlpnProtocols:      []string{"starttls-subprocess"},
			InsecureSkipVerify: true,
		})
		if err != nil {
			t.Fatalf("UpgradeTLS failed: %v", err)
		}
		if res == nil || res.GetNegotiatedProtocol() != "starttls-subprocess" {
			t.Fatalf("unexpected upgrade result: %+v", res)
		}

		// Encrypted payload exchange
		encMsg := []byte("Encrypted data over upgraded subprocess stream!\n")
		if _, err := inStreamConn.Write(encMsg); err != nil {
			t.Fatalf("Write encrypted: %v", err)
		}
		encReply := make([]byte, len(encMsg))
		if _, err := io.ReadFull(inStreamConn, encReply); err != nil {
			t.Fatalf("ReadFull encrypted: %v", err)
		}
		if string(encReply) != string(encMsg) {
			t.Fatalf("echo mismatch: got %q, want %q", string(encReply), string(encMsg))
		}
	})

	// Test C: In-Stream Ping/Pong
	t.Run("InStream_PingPong", func(t *testing.T) {
		dialCtx, dialCancel := context.WithTimeout(ctx, 3*time.Second)
		defer dialCancel()

		conn, err := client.DialContext(dialCtx, "tcp", echoListener.Addr().String())
		if err != nil {
			t.Fatalf("DialContext failed: %v", err)
		}
		defer conn.Close()

		inStreamConn, ok := conn.(netconn.InStreamConn)
		if !ok {
			t.Fatalf("expected InStreamConn, got %T", conn)
		}

		pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
		defer pingCancel()

		rtt, err := inStreamConn.Ping(pingCtx)
		if err != nil {
			t.Fatalf("in-stream Ping failed: %v", err)
		}
		if rtt <= 0 {
			t.Errorf("expected positive RTT, got %v", rtt)
		}
	})

	// Test D: Fail-Closed unsupported fingerprint preset rejection
	t.Run("FailClosed_PresetRejection", func(t *testing.T) {
		dialCtx, dialCancel := context.WithTimeout(ctx, 2*time.Second)
		defer dialCancel()

		badClient := netconn.Client{
			API: rc.NewRemoteControlClient(cc),
			TLS: &sesametls.TLSOptions{
				FingerprintPreset: sesametls.FingerprintPreset_CHROME_131,
			},
		}

		_, err := badClient.DialContext(dialCtx, "tcp", tlsListener.Addr().String())
		if err == nil {
			t.Fatal("expected FAILED_PRECONDITION error for unsupported preset, got nil")
		}
		st, ok := status.FromError(err)
		if !ok || st.Code() != codes.FailedPrecondition {
			t.Errorf("expected codes.FailedPrecondition, got %v (code %v)", err, st.Code())
		}
	})
}
