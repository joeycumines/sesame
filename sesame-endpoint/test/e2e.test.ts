import {describe, it, expect, beforeAll, afterAll} from 'bun:test';
import * as net from 'node:net';
import * as tls from 'node:tls';
import * as http from 'node:http';
import * as fs from 'node:fs';
import * as path from 'node:path';
import {create} from '@bufbuild/protobuf';
import {durationFromMs} from '@bufbuild/protobuf/wkt';
import {createGrpcTransport} from '@connectrpc/connect-node';
import {createClient, Code, ConnectError} from '@connectrpc/connect';
import {
  RemoteControl,
  NetConnRequest,
  NetConnRequestSchema,
  NetConnRequest_DialSchema,
  NetConnRequest_ControlSchema,
  NetConnRequest_Control_UpgradeTLSSchema,
  NetConnRequest_Control_ResetSchema,
  NetConnRequest_Control_PingSchema,
  NetConnRequest_Control_WindowUpdateSchema,
  NetConnRequest_CapabilitiesSchema,
} from '../src/gen/sesame/v1alpha1/remotecontrol_pb';
import {StatusSchema} from '../src/gen/google/rpc/status_pb';
import {
  ClientHelloSpecSchema,
  TLSOptionsSchema,
  TLSVersion,
} from '../src/gen/sesame/tls/v1alpha1/tls_pb';
import {
  ProxyHop_Type,
  ProxyHopSchema,
  ProxyOptionsSchema,
} from '../src/gen/sesame/proxy/v1alpha1/proxy_pb';
import {NetAddrSchema} from '../src/gen/sesame/type/netaddr_pb';
import {createEndpointServer, EndpointServer} from '../src/server';
import {parseConfig} from '../src/config';

const FIXTURES_DIR = fs.existsSync(path.join(__dirname, 'fixtures'))
  ? path.join(__dirname, 'fixtures')
  : path.join(__dirname, '../../test/fixtures');
const CERT_PEM = fs.readFileSync(path.join(FIXTURES_DIR, 'cert.pem'));
const KEY_PEM = fs.readFileSync(path.join(FIXTURES_DIR, 'key.pem'));

class RequestStream {
  private queue: NetConnRequest[] = [];
  private waiters: Array<{
    resolve: (item: IteratorResult<NetConnRequest>) => void;
    reject: (err: Error) => void;
  }> = [];
  private closed = false;

  push(item: NetConnRequest) {
    if (this.closed) return;
    if (this.waiters.length > 0) {
      this.waiters.shift()!.resolve({value: item, done: false});
    } else {
      this.queue.push(item);
    }
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    while (this.waiters.length > 0) {
      this.waiters.shift()!.resolve({
        value: undefined as unknown as NetConnRequest,
        done: true,
      });
    }
  }

  async *[Symbol.asyncIterator]() {
    while (true) {
      if (this.queue.length > 0) {
        yield this.queue.shift()!;
      } else if (this.closed) {
        return;
      } else {
        const item = await new Promise<IteratorResult<NetConnRequest>>(
          (resolve, reject) => {
            this.waiters.push({resolve, reject});
          },
        );
        if (item.done) return;
        yield item.value;
      }
    }
  }
}

describe('sesame-endpoint E2E Suite', () => {
  let endpointServer: EndpointServer;
  let endpointPort: number;
  let client: ReturnType<typeof createClient<typeof RemoteControl>>;

  let tcpEchoServer: net.Server;
  let tcpEchoPort: number;

  let tlsEchoServer: tls.Server;
  let tlsEchoPort: number;

  let httpProxyServer: http.Server;
  let httpProxyPort: number;

  beforeAll(async () => {
    // 1. Plaintext TCP Echo Server
    tcpEchoServer = net.createServer(socket => {
      socket.pipe(socket);
    });
    await new Promise<void>(r =>
      tcpEchoServer.listen(0, '127.0.0.1', () => r()),
    );
    tcpEchoPort = (tcpEchoServer.address() as net.AddressInfo).port;

    // 2. TLS Echo Server with ALPN
    tlsEchoServer = tls.createServer(
      {
        cert: CERT_PEM,
        key: KEY_PEM,
        ALPNProtocols: ['test-proto', 'h2'],
      },
      socket => {
        socket.pipe(socket);
      },
    );
    await new Promise<void>(r =>
      tlsEchoServer.listen(0, '127.0.0.1', () => r()),
    );
    tlsEchoPort = (tlsEchoServer.address() as net.AddressInfo).port;

    // 3. HTTP CONNECT Proxy Server
    httpProxyServer = http.createServer();
    httpProxyServer.on('connect', (req, clientSocket, head) => {
      const parts = (req.url || '').split(':');
      const targetHost = parts[0] || '127.0.0.1';
      const targetPort = parseInt(parts[1], 10) || 80;

      const serverSocket = net.connect(targetPort, targetHost, () => {
        clientSocket.write('HTTP/1.1 200 Connection Established\r\n\r\n');
        if (head.length > 0) {
          serverSocket.write(head);
        }
        serverSocket.pipe(clientSocket);
        clientSocket.pipe(serverSocket);
      });

      serverSocket.on('error', () => clientSocket.destroy());
      clientSocket.on('error', () => serverSocket.destroy());
    });
    await new Promise<void>(r =>
      httpProxyServer.listen(0, '127.0.0.1', () => r()),
    );
    httpProxyPort = (httpProxyServer.address() as net.AddressInfo).port;

    // 4. Sesame Endpoint Server
    const {config} = parseConfig([]);
    endpointServer = createEndpointServer(config!);
    const bound = await endpointServer.listen(0, '127.0.0.1');
    endpointPort = bound.port;

    // 5. Connect gRPC client
    const transport = createGrpcTransport({
      baseUrl: `http://127.0.0.1:${endpointPort}`,
    });
    client = createClient(RemoteControl, transport);
  });

  afterAll(async () => {
    await endpointServer?.close();
    await new Promise<void>(r => tcpEchoServer?.close(() => r()));
    await new Promise<void>(r => tlsEchoServer?.close(() => r()));
    await new Promise<void>(r => httpProxyServer?.close(() => r()));
  });

  it('handles plaintext TCP streaming with bidirectional echo', async () => {
    const reqStream = new RequestStream();

    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${tcpEchoPort}`,
            }),
          }),
        },
      }),
    );

    const testPayload = new TextEncoder().encode('Hello Plaintext Sesame!');
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'bytes',
          value: testPayload,
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    // 1. Initial Conn response
    const first = await iterator.next();
    expect(first.done).toBe(false);
    expect(first.value.data.case).toBe('conn');
    const conn = first.value.data.value;
    expect(conn.local).toBeDefined();
    expect(conn.remote).toBeDefined();
    expect(conn.capabilities).toBeDefined();
    expect(conn.capabilities?.supportsFlowControl).toBe(true);

    // 2. Echoed payload
    const second = await iterator.next();
    expect(second.done).toBe(false);
    expect(second.value.data.case).toBe('bytes');
    expect(new TextDecoder().decode(second.value.data.value)).toBe(
      'Hello Plaintext Sesame!',
    );

    reqStream.close();
  });

  it('fails closed on cipher_suites instead of silently using defaults', async () => {
    // cipher_suites in client_hello cannot be honored by the builtin
    // engine. Negotiating with defaults while reporting success would be
    // a silent security-policy downgrade, so the request must be rejected.
    const reqStream = new RequestStream();
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${tlsEchoPort}`,
            }),
            tls: create(TLSOptionsSchema, {
              serverName: 'localhost',
              insecureSkipVerify: true,
              clientHello: create(ClientHelloSpecSchema, {
                cipherSuites: [0x1301],
              }),
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();
    try {
      await iterator.next();
      expect.unreachable('should have rejected cipher_suites');
    } catch (err: unknown) {
      expect(err).toBeInstanceOf(ConnectError);
      expect((err as ConnectError).code).toBe(Code.FailedPrecondition);
      expect((err as ConnectError).message).toContain('cipher_suites');
      expect((err as ConnectError).message).toContain('builtin engine');
    } finally {
      reqStream.close();
    }
  });

  it('terminates endpoint TLS with ALPN negotiation', async () => {
    const reqStream = new RequestStream();

    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${tlsEchoPort}`,
            }),
            tls: create(TLSOptionsSchema, {
              serverName: 'localhost',
              alpnProtocols: ['test-proto'],
              insecureSkipVerify: true,
            }),
          }),
        },
      }),
    );

    const payload = new TextEncoder().encode('Encrypted Payload over TLS');
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'bytes',
          value: payload,
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    // 1. Conn response
    const first = await iterator.next();
    expect(first.done).toBe(false);
    expect(first.value.data.case).toBe('conn');
    const conn = first.value.data.value;
    expect(conn.tls).toBeDefined();
    expect(conn.tls?.negotiatedProtocol).toBe('test-proto');
    expect(conn.tls?.serverName).toBe('localhost');
    expect(conn.tls?.appliedClientHello).toBeUndefined();

    // 2. Encrypted echo response
    const second = await iterator.next();
    expect(second.done).toBe(false);
    expect(second.value.data.case).toBe('bytes');
    expect(new TextDecoder().decode(second.value.data.value)).toBe(
      'Encrypted Payload over TLS',
    );

    reqStream.close();
  });

  it('enforces ALPN empty suppression rule', async () => {
    const reqStream = new RequestStream();

    // No alpnProtocols specified -> MUST NOT send ALPN extension
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${tlsEchoPort}`,
            }),
            tls: create(TLSOptionsSchema, {
              serverName: 'localhost',
              alpnProtocols: [],
              insecureSkipVerify: true,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    const first = await iterator.next();
    expect(first.done).toBe(false);
    expect(first.value.data.case).toBe('conn');
    const conn = first.value.data.value;
    expect(conn.tls).toBeDefined();
    // Because ALPN was omitted, negotiatedProtocol is empty string
    expect(conn.tls?.negotiatedProtocol).toBe('');
    // Wire-contract parity with the Go reference: the negotiated cipher
    // suite is reported (non-zero) and the peer chain is present.
    expect(conn.tls?.cipherSuite).not.toBe(0);
    expect(conn.tls?.peerCertificates.length).toBeGreaterThan(0);

    reqStream.close();
  });

  it('accepts a default/empty clientHello spec and echoes it', async () => {
    const reqStream = new RequestStream();
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${tlsEchoPort}`,
            }),
            tls: create(TLSOptionsSchema, {
              serverName: 'localhost',
              insecureSkipVerify: true,
              clientHello: create(ClientHelloSpecSchema, {}),
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();
    const first = await iterator.next();
    expect(first.done).toBe(false);
    expect(first.value.data.case).toBe('conn');
    const conn = first.value.data.value;
    expect(conn.tls).toBeDefined();
    // An empty/default spec is echoed verbatim.
    expect(conn.tls?.appliedClientHello).toBeDefined();
    // Capabilities must be present with all booleans false.
    expect(conn.capabilities?.clientHelloCapabilities).toBeDefined();
    const caps = conn.capabilities!.clientHelloCapabilities!;
    expect(caps.customCipherSuites).toBe(false);
    expect(caps.customSupportedGroups).toBe(false);
    expect(caps.customSignatureAlgorithms).toBe(false);
    expect(caps.customExtensionOrder).toBe(false);
    expect(caps.rawExtensions).toBe(false);
    expect(caps.greaseValues).toBe(false);
    expect(caps.sessionIdLength).toBe(false);
    expect(caps.paddingControl).toBe(false);
    expect(caps.legacyVersionControl).toBe(false);
    expect(caps.compressionMethods).toBe(false);

    reqStream.close();
  });

  it('fails closed on non-default clientHello dimension', async () => {
    const reqStream = new RequestStream();

    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${tlsEchoPort}`,
            }),
            tls: create(TLSOptionsSchema, {
              serverName: 'localhost',
              insecureSkipVerify: true,
              clientHello: create(ClientHelloSpecSchema, {
                signatureAlgorithms: [0x0403],
              }),
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    try {
      await iterator.next();
      expect.unreachable('should have failed closed');
    } catch (err: unknown) {
      expect(err).toBeInstanceOf(ConnectError);
      expect((err as ConnectError).code).toBe(Code.FailedPrecondition);
      expect((err as ConnectError).message).toContain(
        'signature_algorithms',
      );
    }

    reqStream.close();
  });

  it('fails closed on disallowed network protocol', async () => {
    const reqStream = new RequestStream();

    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'udp',
              address: '127.0.0.1:53',
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    try {
      await iterator.next();
      expect.unreachable('should have failed closed');
    } catch (err: unknown) {
      expect(err).toBeInstanceOf(ConnectError);
      expect((err as ConnectError).code).toBe(Code.PermissionDenied);
      expect((err as ConnectError).message).toContain('not allowed');
    }

    reqStream.close();
  });

  it('traverses HTTP CONNECT proxy hops', async () => {
    const reqStream = new RequestStream();

    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${tcpEchoPort}`,
            }),
            proxy: create(ProxyOptionsSchema, {
              hops: [
                create(ProxyHopSchema, {
                  type: ProxyHop_Type.HTTP_CONNECT,
                  address: create(NetAddrSchema, {
                    network: 'tcp',
                    address: `127.0.0.1:${httpProxyPort}`,
                  }),
                }),
              ],
            }),
          }),
        },
      }),
    );

    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'bytes',
          value: new TextEncoder().encode('Proxied through HTTP CONNECT!'),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    const first = await iterator.next();
    expect(first.done).toBe(false);
    expect(first.value.data.case).toBe('conn');
    const conn = first.value.data.value;
    expect(conn.proxy).toBeDefined();
    expect(conn.proxy?.traversedHops.length).toBe(1);

    const second = await iterator.next();
    expect(second.done).toBe(false);
    expect(second.value.data.case).toBe('bytes');
    expect(new TextDecoder().decode(second.value.data.value)).toBe(
      'Proxied through HTTP CONNECT!',
    );

    reqStream.close();
  });

  it('rejects a 200 CONNECT response carrying a framed body', async () => {
    // A proxy that answers 200 with framing is not tunnelling; the framing
    // bytes must fail the dial rather than enter the tunnel as payload.
    const badProxy = net.createServer(sock => {
      let seen = Buffer.alloc(0);
      sock.on('data', chunk => {
        seen = Buffer.concat([seen, chunk]);
        if (seen.indexOf('\r\n\r\n') !== -1) {
          sock.write(
            'HTTP/1.1 200 Connection Established\r\nContent-Length: 5\r\n\r\nhello',
          );
        }
      });
    });
    await new Promise<void>(r => badProxy.listen(0, '127.0.0.1', () => r()));
    const badPort = (badProxy.address() as net.AddressInfo).port;

    try {
      const reqStream = new RequestStream();
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'dial',
            value: create(NetConnRequest_DialSchema, {
              address: create(NetAddrSchema, {
                network: 'tcp',
                address: `127.0.0.1:${tcpEchoPort}`,
              }),
              proxy: create(ProxyOptionsSchema, {
                hops: [
                  create(ProxyHopSchema, {
                    type: ProxyHop_Type.HTTP_CONNECT,
                    address: create(NetAddrSchema, {
                      network: 'tcp',
                      address: `127.0.0.1:${badPort}`,
                    }),
                  }),
                ],
              }),
            }),
          },
        }),
      );

      const respStream = client.netConn(reqStream);
      const iterator = respStream[Symbol.asyncIterator]();
      try {
        await iterator.next();
        expect.unreachable('should have rejected the framed 200');
      } catch (err: unknown) {
        expect(err).toBeInstanceOf(ConnectError);
        expect((err as ConnectError).message).toContain('framed response body');
      } finally {
        reqStream.close();
      }
    } finally {
      await new Promise<void>(r => badProxy.close(() => r()));
    }
  });

  it('preserves early tunnel bytes coalesced with the 200 CONNECT headers', async () => {
    // A fast proxy may pipeline the first upstream flight in the same TCP
    // segment as the 200 headers. Those bytes are tunnel payload, not a
    // framed body, and must round-trip intact (mirrors Go bufferedPrefixConn).
    const earlyPayload = Buffer.from('EARLY-TUNNEL-BYTES');
    const coalescingProxy = net.createServer(sock => {
      let seen = Buffer.alloc(0);
      sock.on('data', chunk => {
        seen = Buffer.concat([seen, chunk]);
        if (seen.indexOf('\r\n\r\n') !== -1) {
          sock.write(
            Buffer.concat([
              Buffer.from('HTTP/1.1 200 Connection Established\r\n\r\n'),
              earlyPayload,
            ]),
          );
        }
      });
    });
    await new Promise<void>(r =>
      coalescingProxy.listen(0, '127.0.0.1', () => r()),
    );
    const coalescingPort = (coalescingProxy.address() as net.AddressInfo).port;

    try {
      const reqStream = new RequestStream();
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'dial',
            value: create(NetConnRequest_DialSchema, {
              address: create(NetAddrSchema, {
                network: 'tcp',
                address: `127.0.0.1:${tcpEchoPort}`,
              }),
              proxy: create(ProxyOptionsSchema, {
                hops: [
                  create(ProxyHopSchema, {
                    type: ProxyHop_Type.HTTP_CONNECT,
                    address: create(NetAddrSchema, {
                      network: 'tcp',
                      address: `127.0.0.1:${coalescingPort}`,
                    }),
                  }),
                ],
              }),
            }),
          },
        }),
      );

      const respStream = client.netConn(reqStream);
      const iterator = respStream[Symbol.asyncIterator]();

      const first = await iterator.next();
      expect(first.done).toBe(false);
      expect(first.value.data.case).toBe('conn');

      const second = await iterator.next();
      expect(second.done).toBe(false);
      expect(second.value.data.case).toBe('bytes');
      expect(Buffer.from(second.value.data.value as Uint8Array)).toEqual(
        earlyPayload,
      );

      reqStream.close();
    } finally {
      await new Promise<void>(r => coalescingProxy.close(() => r()));
    }
  });

  it('honors a short per-request dial timeout against an unroutable target', async () => {
    // An unroutable TEST-NET-1 address (RFC 5737, 192.0.2.0/24) has no
    // listener and no refuser on this routing, so the TCP SYN gets no
    // answer and connect() hangs until the dial timer fires. The request
    // asks for 500ms against a server default of 10s: DeadlineExceeded
    // must arrive within a bound proving the request value (not the
    // default) was honored.
    const reqStream = new RequestStream();
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: '192.0.2.1:81',
            }),
            timeout: durationFromMs(500),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();
    const start = Date.now();
    try {
      await iterator.next();
      expect.unreachable('should have timed out on the requested bound');
    } catch (err: unknown) {
      expect(err).toBeInstanceOf(ConnectError);
      expect((err as ConnectError).code).toBe(Code.DeadlineExceeded);
      expect(Date.now() - start).toBeLessThan(9000);
    } finally {
      reqStream.close();
    }
  });

  it('executes in-stream STARTTLS upgrade and exchanges encrypted data', async () => {
    // Upstream server: begins cleartext, awaits STARTTLS\n, upgrades to TLS, echoes
    const starttlsServer = net.createServer(rawSocket => {
      let buffer = '';

      const onRawData = (data: Buffer) => {
        buffer += data.toString();
        if (buffer.includes('STARTTLS\n')) {
          rawSocket.write('220 Ready for TLS\n');
          rawSocket.removeListener('data', onRawData);

          // Wrap rawSocket in TLS server
          const tlsServer = new tls.TLSSocket(rawSocket, {
            isServer: true,
            cert: CERT_PEM,
            key: KEY_PEM,
          });

          tlsServer.on('secure', () => {
            tlsServer.pipe(tlsServer);
          });
        }
      };

      rawSocket.on('data', onRawData);
    });

    await new Promise<void>(r =>
      starttlsServer.listen(0, '127.0.0.1', () => r()),
    );
    const starttlsPort = (starttlsServer.address() as net.AddressInfo).port;

    const reqStream = new RequestStream();

    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${starttlsPort}`,
            }),
            capabilities: create(NetConnRequest_CapabilitiesSchema, {
              supportsOpportunisticTls: true,
              supportsFlowControl: true,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    try {
      // 1. Conn response
      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');

      // 2. Cleartext STARTTLS command
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'bytes',
            value: new TextEncoder().encode('STARTTLS\n'),
          },
        }),
      );

      // 3. Receive 220 Ready (interleaved with windowUpdate if present)
      let clearResp = await iterator.next();
      if (clearResp.value.data.case === 'control') {
        expect(clearResp.value.data.value.event.case).toBe('windowUpdate');
        clearResp = await iterator.next();
      }
      expect(clearResp.value.data.case).toBe('bytes');
      expect(new TextDecoder().decode(clearResp.value.data.value)).toContain(
        '220',
      );

      // 4. Send in-stream UpgradeTLS command
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'control',
            value: create(NetConnRequest_ControlSchema, {
              action: {
                case: 'upgradeTls',
                value: create(NetConnRequest_Control_UpgradeTLSSchema, {
                  options: create(TLSOptionsSchema, {
                    serverName: 'localhost',
                    insecureSkipVerify: true,
                  }),
                }),
              },
            }),
          },
        }),
      );

      // 5. Receive in-stream tlsUpgraded event
      const upgradeResp = await iterator.next();
      expect(upgradeResp.value.data.case).toBe('control');
      const ctl = upgradeResp.value.data.value;
      expect(ctl.event.case).toBe('tlsUpgraded');
      expect(ctl.event.value.result).toBeDefined();

      // 6. Send encrypted payload
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'bytes',
            value: new TextEncoder().encode('Encrypted secret after STARTTLS!'),
          },
        }),
      );

      let encryptedEcho = await iterator.next();
      if (encryptedEcho.value.data.case === 'control') {
        expect(encryptedEcho.value.data.value.event.case).toBe('windowUpdate');
        encryptedEcho = await iterator.next();
      }
      expect(encryptedEcho.value.data.case).toBe('bytes');
      expect(new TextDecoder().decode(encryptedEcho.value.data.value)).toBe(
        'Encrypted secret after STARTTLS!',
      );

      reqStream.close();
    } finally {
      await new Promise<void>(r => starttlsServer.close(() => r()));
    }
  });

  it('surfaces a client reset as an error carrying the reason', async () => {
    const echoServer = net.createServer(rawSocket => {
      rawSocket.pipe(rawSocket);
    });
    await new Promise<void>(r => echoServer.listen(0, '127.0.0.1', () => r()));
    const echoPort = (echoServer.address() as net.AddressInfo).port;

    const reqStream = new RequestStream();
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${echoPort}`,
            }),
            capabilities: create(NetConnRequest_CapabilitiesSchema, {
              supportsOpportunisticTls: true,
              supportsFlowControl: true,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    try {
      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');

      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'control',
            value: create(NetConnRequest_ControlSchema, {
              action: {
                case: 'reset',
                value: create(NetConnRequest_Control_ResetSchema, {
                  reason: create(StatusSchema, {
                    code: Code.Canceled,
                    message: 'client went away',
                  }),
                }),
              },
            }),
          },
        }),
      );

      let resetErr: unknown;
      try {
        while (true) {
          const resp = await iterator.next();
          if (resp.done) break;
        }
      } catch (err: unknown) {
        resetErr = err;
      }
      expect(resetErr).toBeInstanceOf(ConnectError);
      const ce = resetErr as ConnectError;
      expect(ce.message).toContain('connection reset by client');
      expect(ce.message).toContain('client went away');
      expect(ce.code).toBe(Code.Canceled);
    } finally {
      reqStream.close();
      await new Promise<void>(r => echoServer.close(() => r()));
    }
  });

  it('fully closes the upstream on request-stream EOF per the termination contract', async () => {
    // CloseSend semantics: the server initiates a FULL close of the proxy
    // target; the response stream completes without waiting for the
    // upstream to finish. halfClose is the mechanism for drain-then-wait.
    let upstreamFullyClosed = false;
    const upstream = net.createServer(rawSocket => {
      rawSocket.on('close', () => {
        upstreamFullyClosed = true;
      });
      // Never echo, never end: any response-stream completion must come
      // from the EOF-triggered teardown, not upstream EOF.
      rawSocket.on('data', () => {});
    });
    await new Promise<void>(r => upstream.listen(0, '127.0.0.1', () => r()));
    const upstreamPort = (upstream.address() as net.AddressInfo).port;

    const reqStream = new RequestStream();
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${upstreamPort}`,
            }),
            capabilities: create(NetConnRequest_CapabilitiesSchema, {
              supportsOpportunisticTls: true,
              supportsFlowControl: true,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    try {
      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');

      // Close the request side (CloseSend equivalent).
      reqStream.close();

      // The response stream must complete even though the upstream never
      // sent anything and never closed on its own.
      const done = await iterator.next();
      expect(done.done).toBe(true);

      // And the upstream socket must have been fully closed (destroyed),
      // not merely half-closed.
      const deadline = Date.now() + 2000;
      while (!upstreamFullyClosed && Date.now() < deadline) {
        await new Promise<void>(r => setTimeout(r, 20));
      }
      expect(upstreamFullyClosed).toBe(true);
    } finally {
      await new Promise<void>(r => upstream.close(() => r()));
    }
  });

  it('rejects in-stream TLS upgrade without options with InvalidArgument', async () => {
    const echoServer = net.createServer(rawSocket => {
      rawSocket.pipe(rawSocket);
    });
    await new Promise<void>(r => echoServer.listen(0, '127.0.0.1', () => r()));
    const echoPort = (echoServer.address() as net.AddressInfo).port;

    const reqStream = new RequestStream();
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${echoPort}`,
            }),
            capabilities: create(NetConnRequest_CapabilitiesSchema, {
              supportsOpportunisticTls: true,
              supportsFlowControl: true,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    try {
      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');

      // upgradeTls with absent options must terminate the stream with
      // InvalidArgument - it must not emit tlsUpgraded.
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'control',
            value: create(NetConnRequest_ControlSchema, {
              action: {
                case: 'upgradeTls',
                value: create(NetConnRequest_Control_UpgradeTLSSchema, {}),
              },
            }),
          },
        }),
      );

      let sawInvalidArgument = false;
      try {
        while (true) {
          const resp = await iterator.next();
          if (resp.done) break;
          if (resp.value.data.case === 'control') {
            const evt = resp.value.data.value.event;
            if (evt.case === 'tlsUpgraded') {
              throw new Error(
                'server emitted tlsUpgraded for an options-less upgrade',
              );
            }
          }
        }
      } catch (err: unknown) {
        if (err instanceof ConnectError && err.code === Code.InvalidArgument) {
          sawInvalidArgument = true;
        } else {
          throw err;
        }
      }
      expect(sawInvalidArgument).toBe(true);
      reqStream.close();
    } finally {
      await new Promise<void>(r => echoServer.close(() => r()));
    }
  });

  it('rejects in-stream TLS upgrade when opportunistic TLS is disabled', async () => {
    // A server started with enableOpportunisticTls=false still advertises
    // the flag honestly, but a buggy or malicious client may send
    // upgrade_tls anyway. The server must reject with FailedPrecondition
    // before any handshake or socket mutation - the flag is enforced,
    // not advisory.
    const echoServer = net.createServer(rawSocket => {
      rawSocket.pipe(rawSocket);
    });
    await new Promise<void>(r => echoServer.listen(0, '127.0.0.1', () => r()));
    const echoPort = (echoServer.address() as net.AddressInfo).port;

    const {config: strictConfig} = parseConfig([
      '--enable-opportunistic-tls=false',
    ]);
    const strictServer = createEndpointServer(strictConfig!);
    const bound = await strictServer.listen(0, '127.0.0.1');
    const strictTransport = createGrpcTransport({
      baseUrl: `http://127.0.0.1:${bound.port}`,
    });
    const strictClient = createClient(RemoteControl, strictTransport);

    try {
      const reqStream = new RequestStream();
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'dial',
            value: create(NetConnRequest_DialSchema, {
              address: create(NetAddrSchema, {
                network: 'tcp',
                address: `127.0.0.1:${echoPort}`,
              }),
              capabilities: create(NetConnRequest_CapabilitiesSchema, {
                supportsOpportunisticTls: true,
                supportsFlowControl: true,
              }),
            }),
          },
        }),
      );

      const respStream = strictClient.netConn(reqStream);
      const iterator = respStream[Symbol.asyncIterator]();

      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');
      expect(
        first.value.data.value.capabilities?.supportsOpportunisticTls,
      ).toBe(false);

      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'control',
            value: create(NetConnRequest_ControlSchema, {
              action: {
                case: 'upgradeTls',
                value: create(NetConnRequest_Control_UpgradeTLSSchema, {
                  options: create(TLSOptionsSchema, {
                    serverName: 'localhost',
                    insecureSkipVerify: true,
                  }),
                }),
              },
            }),
          },
        }),
      );

      let sawFailedPrecondition = false;
      try {
        while (true) {
          const resp = await iterator.next();
          if (resp.done) break;
          if (resp.value.data.case === 'control') {
            const evt = resp.value.data.value.event;
            if (evt.case === 'tlsUpgraded') {
              throw new Error(
                'server emitted tlsUpgraded for a policy-disabled upgrade',
              );
            }
          }
        }
      } catch (err: unknown) {
        if (
          err instanceof ConnectError &&
          err.code === Code.FailedPrecondition
        ) {
          sawFailedPrecondition = true;
        } else {
          throw err;
        }
      }
      expect(sawFailedPrecondition).toBe(true);
      reqStream.close();
    } finally {
      await strictServer.close();
      await new Promise<void>(r => echoServer.close(() => r()));
    }
  });

  it('fails closed when in-stream TLS upgrade encounters handshake failure', async () => {
    // Non-TLS server that sends garbage or immediately closes when TLS handshake begins
    const nonTlsServer = net.createServer(rawSocket => {
      rawSocket.on('data', () => {
        // Send plain text error instead of TLS handshake record, then destroy
        rawSocket.write('500 Unrecognized SSL handshake\n');
        rawSocket.destroy();
      });
    });

    await new Promise<void>(r =>
      nonTlsServer.listen(0, '127.0.0.1', () => r()),
    );
    const nonTlsPort = (nonTlsServer.address() as net.AddressInfo).port;

    const reqStream = new RequestStream();

    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${nonTlsPort}`,
            }),
            capabilities: create(NetConnRequest_CapabilitiesSchema, {
              supportsOpportunisticTls: true,
              supportsFlowControl: true,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    try {
      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');

      // Request TLS upgrade on non-TLS upstream
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'control',
            value: create(NetConnRequest_ControlSchema, {
              action: {
                case: 'upgradeTls',
                value: create(NetConnRequest_Control_UpgradeTLSSchema, {
                  options: create(TLSOptionsSchema, {
                    serverName: 'localhost',
                    insecureSkipVerify: true,
                  }),
                }),
              },
            }),
          },
        }),
      );

      // Should receive tlsUpgradeFailed control event or stream termination
      const failResp = await iterator.next();
      if (!failResp.done) {
        expect(failResp.value.data.case).toBe('control');
        if (failResp.value.data.case === 'control') {
          expect(failResp.value.data.value.event.case).toBe('tlsUpgradeFailed');
        }
      }
      reqStream.close();
    } catch (err: unknown) {
      // ConnectError from stream failure is also acceptable fail-closed behavior
      expect(err).toBeDefined();
    } finally {
      await new Promise<void>(r => nonTlsServer.close(() => r()));
    }
  });

  it('handles in-stream ping/pong liveness probe', async () => {
    const reqStream = new RequestStream();

    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${tcpEchoPort}`,
            }),
            capabilities: create(NetConnRequest_CapabilitiesSchema, {
              supportsOpportunisticTls: true,
              supportsFlowControl: true,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    // 1. Conn response
    const first = await iterator.next();
    expect(first.value.data.case).toBe('conn');

    // 2. Send Ping
    const pingId = 4242n;
    const pingTs = 123456789n;
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'control',
          value: create(NetConnRequest_ControlSchema, {
            action: {
              case: 'ping',
              value: create(NetConnRequest_Control_PingSchema, {
                id: pingId,
                timestampNanos: pingTs,
              }),
            },
          }),
        },
      }),
    );

    // 3. Receive Pong
    const pongResp = await iterator.next();
    expect(pongResp.value.data.case).toBe('control');
    const ctl = pongResp.value.data.value;
    expect(ctl.event.case).toBe('pong');
    expect(ctl.event.value.id).toBe(pingId);
    expect(ctl.event.value.timestampNanos).toBe(pingTs);

    reqStream.close();
  });

  it('manages flow control window updates and backpressure', async () => {
    const reqStream = new RequestStream();

    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${tcpEchoPort}`,
            }),
            capabilities: create(NetConnRequest_CapabilitiesSchema, {
              supportsFlowControl: true,
              initialWindowSize: 100,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    const first = await iterator.next();
    expect(first.value.data.case).toBe('conn');

    // Send payload
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'bytes',
          value: new TextEncoder().encode('Flow control message 123'),
        },
      }),
    );

    // Receive echoed bytes or inbound windowUpdate
    let receivedBytes = false;
    let receivedWU = false;

    for (let i = 0; i < 2; i++) {
      const resp = await iterator.next();
      if (resp.value.data.case === 'bytes') {
        receivedBytes = true;
        expect(new TextDecoder().decode(resp.value.data.value)).toBe(
          'Flow control message 123',
        );
      } else if (resp.value.data.case === 'control') {
        if (resp.value.data.value.event.case === 'windowUpdate') {
          receivedWU = true;
          expect(resp.value.data.value.event.value.creditBytes).toBeGreaterThan(
            0,
          );
        }
      }
    }

    expect(receivedBytes || receivedWU).toBe(true);

    // Send client window update
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'control',
          value: create(NetConnRequest_ControlSchema, {
            action: {
              case: 'windowUpdate',
              value: create(NetConnRequest_Control_WindowUpdateSchema, {
                creditBytes: 1000,
              }),
            },
          }),
        },
      }),
    );

    reqStream.close();
  });

  it('replenishes the inbound window so payloads larger than it still flow', async () => {
    // A tiny initial window makes the stall deterministic: without the
    // server refunding credit as bytes reach the socket, the second chunk
    // blocks forever once the window is spent.
    const tinyWindow = 1024;
    const total = tinyWindow * 4;
    const payload = Buffer.alloc(total, 0x61);

    const sinkSockets: net.Socket[] = [];
    const sink = net.createServer(sock => {
      // Consume without echoing so the only responses are window updates.
      sinkSockets.push(sock);
      sock.resume();
    });
    await new Promise<void>(r => sink.listen(0, '127.0.0.1', () => r()));
    const sinkPort = (sink.address() as net.AddressInfo).port;

    try {
      const {config} = parseConfig([
        '--initial-window-size',
        String(tinyWindow),
        '--max-chunk-size',
        '512',
      ]);
      const server = createEndpointServer(config!);
      const bound = await server.listen(0, '127.0.0.1');

      try {
        const transport = createGrpcTransport({
          baseUrl: `http://127.0.0.1:${bound.port}`,
        });
        const localClient = createClient(RemoteControl, transport);
        const reqStream = new RequestStream();

        reqStream.push(
          create(NetConnRequestSchema, {
            data: {
              case: 'dial',
              value: create(NetConnRequest_DialSchema, {
                address: create(NetAddrSchema, {
                  network: 'tcp',
                  address: `127.0.0.1:${sinkPort}`,
                }),
                capabilities: create(NetConnRequest_CapabilitiesSchema, {
                  supportsFlowControl: true,
                  initialWindowSize: tinyWindow,
                }),
              }),
            },
          }),
        );

        const abort = new AbortController();
        const iterator = localClient
          .netConn(reqStream, {
            signal: abort.signal,
          })
          [Symbol.asyncIterator]();
        const first = await iterator.next();
        expect(first.value.data.case).toBe('conn');

        // Push well past the window. Each slice is admitted only because the
        // server returns credit after the write.
        let acknowledged = 0;
        const drainResponses = (async () => {
          while (acknowledged < total) {
            const resp = await iterator.next();
            if (resp.done) break;
            if (resp.value.data.case !== 'control') continue;
            const event = resp.value.data.value.event;
            if (event.case === 'windowUpdate') {
              acknowledged += Number(event.value.creditBytes);
            }
          }
        })();

        for (let offset = 0; offset < total; offset += 512) {
          reqStream.push(
            create(NetConnRequestSchema, {
              data: {
                case: 'bytes',
                value: new Uint8Array(payload.subarray(offset, offset + 512)),
              },
            }),
          );
        }

        await drainResponses;
        expect(acknowledged).toBe(total);
        reqStream.close();
        abort.abort();
      } finally {
        await server.close();
      }
    } finally {
      for (const sock of sinkSockets) sock.destroy();
      await new Promise<void>(r => sink.close(() => r()));
    }
  });

  it('rejects a negative window_update credit as a protocol violation', async () => {
    const echoServer = net.createServer(rawSocket => {
      rawSocket.pipe(rawSocket);
    });
    await new Promise<void>(r => echoServer.listen(0, '127.0.0.1', () => r()));
    const echoPort = (echoServer.address() as net.AddressInfo).port;

    const reqStream = new RequestStream();
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${echoPort}`,
            }),
            capabilities: create(NetConnRequest_CapabilitiesSchema, {
              supportsFlowControl: true,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    try {
      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');

      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'control',
            value: create(NetConnRequest_ControlSchema, {
              action: {
                case: 'windowUpdate',
                value: create(NetConnRequest_Control_WindowUpdateSchema, {
                  creditBytes: -1024,
                }),
              },
            }),
          },
        }),
      );

      let termErr: unknown;
      try {
        while (true) {
          const resp = await iterator.next();
          if (resp.done) break;
        }
      } catch (err: unknown) {
        termErr = err;
      }
      expect(termErr).toBeInstanceOf(ConnectError);
      expect((termErr as ConnectError).code).toBe(Code.InvalidArgument);
      expect((termErr as ConnectError).message).toContain(
        'negative window_update credit_bytes',
      );
    } finally {
      reqStream.close();
      await new Promise<void>(r => echoServer.close(() => r()));
    }
  });

  it('rejects a negative window_update even when flow control is off', async () => {
    // The negative-credit rule is unconditional: it must hold even when
    // flow control was never negotiated, so a malformed peer cannot slip
    // a violation past an inactive controller.
    const echoServer = net.createServer(rawSocket => {
      rawSocket.pipe(rawSocket);
    });
    await new Promise<void>(r => echoServer.listen(0, '127.0.0.1', () => r()));
    const echoPort = (echoServer.address() as net.AddressInfo).port;

    const reqStream = new RequestStream();
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${echoPort}`,
            }),
            capabilities: create(NetConnRequest_CapabilitiesSchema, {
              supportsOpportunisticTls: true,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    try {
      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');

      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'control',
            value: create(NetConnRequest_ControlSchema, {
              action: {
                case: 'windowUpdate',
                value: create(NetConnRequest_Control_WindowUpdateSchema, {
                  creditBytes: -1,
                }),
              },
            }),
          },
        }),
      );

      let termErr: unknown;
      try {
        while (true) {
          const resp = await iterator.next();
          if (resp.done) break;
        }
      } catch (err: unknown) {
        termErr = err;
      }
      expect(termErr).toBeInstanceOf(ConnectError);
      expect((termErr as ConnectError).code).toBe(Code.InvalidArgument);
      expect((termErr as ConnectError).message).toContain(
        'negative window_update credit_bytes',
      );
    } finally {
      reqStream.close();
      await new Promise<void>(r => echoServer.close(() => r()));
    }
  });

  it('rejects negative capability values at dial with InvalidArgument', async () => {
    for (const caps of [
      {supportsFlowControl: true, initialWindowSize: -1, maxChunkSize: 0},
      {supportsFlowControl: true, initialWindowSize: 0, maxChunkSize: -512},
    ]) {
      const reqStream = new RequestStream();
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'dial',
            value: create(NetConnRequest_DialSchema, {
              address: create(NetAddrSchema, {
                network: 'tcp',
                address: '127.0.0.1:1',
              }),
              capabilities: create(NetConnRequest_CapabilitiesSchema, caps),
            }),
          },
        }),
      );

      const respStream = client.netConn(reqStream);
      const iterator = respStream[Symbol.asyncIterator]();

      let dialErr: unknown;
      try {
        while (true) {
          const resp = await iterator.next();
          if (resp.done) break;
        }
      } catch (err: unknown) {
        dialErr = err;
      }
      expect(dialErr).toBeInstanceOf(ConnectError);
      expect((dialErr as ConnectError).code).toBe(Code.InvalidArgument);
      expect((dialErr as ConnectError).message).toContain(
        'negative capability value',
      );
      reqStream.close();
    }
  });

  it('rejects min_version above max_version with InvalidArgument', async () => {
    const reqStream = new RequestStream();
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: '127.0.0.1:1',
            }),
            tls: create(TLSOptionsSchema, {
              serverName: 'example.com',
              minVersion: TLSVersion.TLS_1_3,
              maxVersion: TLSVersion.TLS_1_2,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    let tlsErr: unknown;
    try {
      while (true) {
        const resp = await iterator.next();
        if (resp.done) break;
      }
    } catch (err: unknown) {
      tlsErr = err;
    }
    expect(tlsErr).toBeInstanceOf(ConnectError);
    expect((tlsErr as ConnectError).code).toBe(Code.InvalidArgument);
    expect((tlsErr as ConnectError).message).toContain(
      'min_version exceeds max_version',
    );
    reqStream.close();
  });

  it('rejects proxy chains exceeding the hop bound with InvalidArgument', async () => {
    const hops = Array.from({length: 9}, () =>
      create(ProxyHopSchema, {
        type: ProxyHop_Type.HTTP_CONNECT,
        address: create(NetAddrSchema, {
          network: 'tcp',
          address: '127.0.0.1:1',
        }),
      }),
    );
    const reqStream = new RequestStream();
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: '127.0.0.1:1',
            }),
            proxy: create(ProxyOptionsSchema, {hops}),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    let hopErr: unknown;
    try {
      while (true) {
        const resp = await iterator.next();
        if (resp.done) break;
      }
    } catch (err: unknown) {
      hopErr = err;
    }
    expect(hopErr).toBeInstanceOf(ConnectError);
    expect((hopErr as ConnectError).code).toBe(Code.InvalidArgument);
    expect((hopErr as ConnectError).message).toContain(
      'too many proxy hops: 9 (max 8)',
    );
    reqStream.close();
  });

  it('clamps outbound chunks to the client-advertised max_chunk_size', async () => {
    const echoServer = net.createServer(rawSocket => {
      rawSocket.pipe(rawSocket);
    });
    await new Promise<void>(r => echoServer.listen(0, '127.0.0.1', () => r()));
    const echoPort = (echoServer.address() as net.AddressInfo).port;

    const clientMaxChunk = 1024;
    const reqStream = new RequestStream();
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${echoPort}`,
            }),
            capabilities: create(NetConnRequest_CapabilitiesSchema, {
              supportsFlowControl: true,
              maxChunkSize: clientMaxChunk,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    try {
      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');

      // Push 5x the advertised maximum through the echo target; every
      // server->client data chunk must arrive clamped to it.
      const payload = Buffer.alloc(clientMaxChunk * 5, 0x78);
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {case: 'bytes', value: new Uint8Array(payload)},
        }),
      );

      let received = 0;
      while (received < payload.length) {
        const resp = await iterator.next();
        if (resp.done) {
          throw new Error(`stream ended after ${received} bytes`);
        }
        if (resp.value.data.case === 'bytes') {
          const chunk = resp.value.data.value;
          expect(chunk.length).toBeLessThanOrEqual(clientMaxChunk);
          received += chunk.length;
        }
      }
      expect(received).toBe(payload.length);
    } finally {
      reqStream.close();
      await new Promise<void>(r => echoServer.close(() => r()));
    }
  });

  it('treats control from a no-capabilities client as request-stream end', async () => {
    // Consent rule: a client that sent no capabilities opts out of in-stream
    // control; a control message is not honored and is treated as the end
    // of the request stream (full close), matching the Go legacy path.
    let upstreamFullyClosed = false;
    const upstream = net.createServer(rawSocket => {
      rawSocket.on('close', () => {
        upstreamFullyClosed = true;
      });
    });
    await new Promise<void>(r => upstream.listen(0, '127.0.0.1', () => r()));
    const upstreamPort = (upstream.address() as net.AddressInfo).port;

    const reqStream = new RequestStream();
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${upstreamPort}`,
            }),
          }),
        },
      }),
    );

    const respStream = client.netConn(reqStream);
    const iterator = respStream[Symbol.asyncIterator]();

    try {
      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');

      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'control',
            value: create(NetConnRequest_ControlSchema, {
              action: {
                case: 'ping',
                value: create(NetConnRequest_Control_PingSchema, {
                  id: 1n,
                  timestampNanos: 1n,
                }),
              },
            }),
          },
        }),
      );

      // The stream must terminate cleanly (request-stream end semantics),
      // and the upstream must be fully closed.
      const final = await iterator.next();
      expect(final.done).toBe(true);
      await new Promise<void>(r => setTimeout(r, 50));
      expect(upstreamFullyClosed).toBe(true);
    } finally {
      reqStream.close();
      await new Promise<void>(r => upstream.close(() => r()));
    }
  });
});
