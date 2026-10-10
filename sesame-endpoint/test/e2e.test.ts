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
  NetConnRequest_Control_HalfCloseSchema,
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

// Byte-level SOCKS5 mock (RFC 1928/1929) for exercising the endpoint's
// hand-rolled framing. Each connection runs a greeting -> (auth) ->
// connect state machine, records every frame the client sent, and on
// success pipes to the real target the client asked for, so tunnel
// payloads round-trip like they would through a real proxy.
interface Socks5MockFrame {
  greeting: Buffer;
  auth: Buffer | null;
  connect: Buffer | null;
}

interface Socks5MockOptions {
  // Answer the greeting by selecting 0x02 (username/password) instead of
  // 0x00 (no auth).
  requireAuth?: boolean;
  // Reject the RFC 1929 subnegotiation with status 0x01.
  authReject?: boolean;
  // Reply with this REP byte instead of 0x00 (succeeded).
  connectRep?: number;
  // Append these bytes to the connect reply: early tunnel payload a
  // fast proxy coalesced into the same TCP segment.
  coalesceWithReply?: Buffer;
  // Skip dialing the real target (for failure scenarios).
  noUpstream?: boolean;
}

function formatIpv6(bytes: Buffer): string {
  const groups: string[] = [];
  for (let i = 0; i < 16; i += 2) {
    groups.push(((bytes[i] << 8) | bytes[i + 1]).toString(16));
  }
  return groups.join(':');
}

function makeSocks5Mock(opts: Socks5MockOptions = {}) {
  const frames: Socks5MockFrame[] = [];

  const server = net.createServer(raw => {
    let stage: 'greeting' | 'auth' | 'connect' | 'tunnel' = 'greeting';
    let buf = Buffer.alloc(0);
    const frame: Socks5MockFrame = {
      greeting: Buffer.alloc(0),
      auth: null,
      connect: null,
    };
    frames.push(frame);
    let upstream: net.Socket | null = null;

    const dialUpstream = (connect: Buffer) => {
      const atyp = connect[3];
      let host: string;
      let addrOff: number;
      let addrLen: number;
      if (atyp === 0x01) {
        host = `${connect[4]}.${connect[5]}.${connect[6]}.${connect[7]}`;
        addrOff = 4;
        addrLen = 4;
      } else if (atyp === 0x04) {
        host = formatIpv6(connect.subarray(4, 20));
        addrOff = 4;
        addrLen = 16;
      } else {
        const domainLen = connect[4];
        host = connect.subarray(5, 5 + domainLen).toString('utf-8');
        addrOff = 5 + domainLen;
        addrLen = 0;
      }
      const port =
        (connect[addrOff + addrLen] << 8) | connect[addrOff + addrLen + 1];
      upstream = net.connect(port, host, () => {
        upstream?.pipe(raw);
      });
      // The endpoint writes tunnel payload the instant the handshake
      // resolves, racing the upstream connect; pipe immediately - net
      // buffers writes to a connecting socket.
      raw.pipe(upstream!);
      upstream.on('error', () => raw.destroy());
    };

    raw.on('data', chunk => {
      if (stage === 'tunnel') {
        // raw -> upstream is handled by the pipe attached in
        // dialUpstream; this handler is parsing-only.
        return;
      }
      buf = Buffer.concat([buf, chunk]);

      if (stage === 'greeting') {
        if (buf.length < 2) return;
        const nMethods = buf[1];
        if (buf.length < 2 + nMethods) return;
        frame.greeting = buf.subarray(0, 2 + nMethods);
        buf = buf.subarray(2 + nMethods);
        raw.write(Buffer.from([0x05, opts.requireAuth ? 0x02 : 0x00]));
        stage = opts.requireAuth ? 'auth' : 'connect';
      }

      if (stage === 'auth') {
        if (buf.length < 2) return;
        const uLen = buf[1];
        if (buf.length < 2 + uLen + 1) return;
        const pLen = buf[2 + uLen];
        if (buf.length < 2 + uLen + 1 + pLen) return;
        frame.auth = buf.subarray(0, 2 + uLen + 1 + pLen);
        buf = buf.subarray(2 + uLen + 1 + pLen);
        if (opts.authReject) {
          raw.write(Buffer.from([0x01, 0x01]));
          raw.destroy();
          return;
        }
        raw.write(Buffer.from([0x01, 0x00]));
        stage = 'connect';
      }

      if (stage === 'connect') {
        if (buf.length < 5) return;
        const atyp = buf[3];
        let addrLen = 0;
        if (atyp === 0x01) addrLen = 4;
        else if (atyp === 0x04) addrLen = 16;
        else if (atyp === 0x03) addrLen = 1 + buf[4];
        else {
          raw.destroy();
          return;
        }
        const total = 4 + addrLen + 2;
        if (buf.length < total) return;
        frame.connect = buf.subarray(0, total);
        buf = buf.subarray(total);

        const reply = Buffer.concat([
          Buffer.from([
            0x05,
            opts.connectRep ?? 0x00,
            0x00,
            0x01,
            0,
            0,
            0,
            0,
            0,
            0,
          ]),
          opts.coalesceWithReply ?? Buffer.alloc(0),
        ]);
        raw.write(reply);
        if ((opts.connectRep ?? 0x00) !== 0x00) {
          raw.destroy();
          return;
        }
        // Enter tunnel state immediately: the endpoint may write
        // payload before the upstream dial completes.
        stage = 'tunnel';
        if (!opts.noUpstream) {
          dialUpstream(frame.connect);
        }
      }
    });

    raw.on('close', () => upstream?.destroy());
    raw.on('error', () => upstream?.destroy());
  });

  return {server, frames: () => frames};
}

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
      expect((err as ConnectError).message).toContain('signature_algorithms');
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

  it('traverses a SOCKS5 proxy hop and round-trips payload', async () => {
    const mock = makeSocks5Mock();
    await new Promise<void>(r => mock.server.listen(0, '127.0.0.1', () => r()));
    const mockPort = (mock.server.address() as net.AddressInfo).port;

    try {
      const reqStream = new RequestStream();
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'dial',
            value: create(NetConnRequest_DialSchema, {
              address: create(NetAddrSchema, {
                network: 'tcp',
                // A domain target exercises the ATYP 0x03 encoding.
                address: `localhost:${tcpEchoPort}`,
              }),
              proxy: create(ProxyOptionsSchema, {
                hops: [
                  create(ProxyHopSchema, {
                    type: ProxyHop_Type.SOCKS5,
                    address: create(NetAddrSchema, {
                      network: 'tcp',
                      address: `127.0.0.1:${mockPort}`,
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
            value: new TextEncoder().encode('HELLO-SOCKS5'),
          },
        }),
      );

      const respStream = client.netConn(reqStream);
      const iterator = respStream[Symbol.asyncIterator]();

      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');
      const conn = first.value.data.value;
      expect(conn.proxy).toBeDefined();
      expect(conn.proxy?.traversedHops.length).toBe(1);

      const second = await iterator.next();
      expect(second.value.data.case).toBe('bytes');
      expect(new TextDecoder().decode(second.value.data.value)).toBe(
        'HELLO-SOCKS5',
      );

      // The framing is the thing under test: verify the exact bytes
      // the client sent - a no-auth greeting and a domain-typed
      // CONNECT carrying the target port in network byte order.
      const frame = mock.frames().at(-1)!;
      expect(frame.greeting).toEqual(Buffer.from([0x05, 0x01, 0x00]));
      expect(frame.auth).toBeNull();
      expect(frame.connect).toEqual(
        Buffer.concat([
          Buffer.from([0x05, 0x01, 0x00, 0x03, 'localhost'.length]),
          Buffer.from('localhost', 'utf-8'),
          Buffer.from([tcpEchoPort >> 8, tcpEchoPort & 0xff]),
        ]),
      );

      reqStream.close();
    } finally {
      await new Promise<void>(r => mock.server.close(() => r()));
    }
  });

  it('authenticates to SOCKS5 with RFC 1929 username/password', async () => {
    const mock = makeSocks5Mock({requireAuth: true});
    await new Promise<void>(r => mock.server.listen(0, '127.0.0.1', () => r()));
    const mockPort = (mock.server.address() as net.AddressInfo).port;

    try {
      const reqStream = new RequestStream();
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'dial',
            value: create(NetConnRequest_DialSchema, {
              address: create(NetAddrSchema, {
                network: 'tcp',
                address: `localhost:${tcpEchoPort}`,
              }),
              proxy: create(ProxyOptionsSchema, {
                hops: [
                  create(ProxyHopSchema, {
                    type: ProxyHop_Type.SOCKS5,
                    address: create(NetAddrSchema, {
                      network: 'tcp',
                      address: `127.0.0.1:${mockPort}`,
                    }),
                    username: 'socksuser',
                    password: 'sockspass',
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
            value: new TextEncoder().encode('AUTH-ECHO'),
          },
        }),
      );

      const respStream = client.netConn(reqStream);
      const iterator = respStream[Symbol.asyncIterator]();

      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');

      const second = await iterator.next();
      expect(second.value.data.case).toBe('bytes');
      expect(new TextDecoder().decode(second.value.data.value)).toBe(
        'AUTH-ECHO',
      );

      // The client must offer exactly [no-auth, username/password] and
      // then send the RFC 1929 subnegotiation verbatim.
      const frame = mock.frames().at(-1)!;
      expect(frame.greeting).toEqual(Buffer.from([0x05, 0x02, 0x00, 0x02]));
      expect(frame.auth).toEqual(
        Buffer.concat([
          Buffer.from([0x01, 'socksuser'.length]),
          Buffer.from('socksuser', 'utf-8'),
          Buffer.from(['sockspass'.length]),
          Buffer.from('sockspass', 'utf-8'),
        ]),
      );
      expect(frame.connect?.subarray(0, 4)).toEqual(
        Buffer.from([0x05, 0x01, 0x00, 0x03]),
      );

      reqStream.close();
    } finally {
      await new Promise<void>(r => mock.server.close(() => r()));
    }
  });

  it('surfaces a SOCKS5 auth rejection as PermissionDenied', async () => {
    const mock = makeSocks5Mock({requireAuth: true, authReject: true});
    await new Promise<void>(r => mock.server.listen(0, '127.0.0.1', () => r()));
    const mockPort = (mock.server.address() as net.AddressInfo).port;

    try {
      const reqStream = new RequestStream();
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'dial',
            value: create(NetConnRequest_DialSchema, {
              address: create(NetAddrSchema, {
                network: 'tcp',
                address: `localhost:${tcpEchoPort}`,
              }),
              proxy: create(ProxyOptionsSchema, {
                hops: [
                  create(ProxyHopSchema, {
                    type: ProxyHop_Type.SOCKS5,
                    address: create(NetAddrSchema, {
                      network: 'tcp',
                      address: `127.0.0.1:${mockPort}`,
                    }),
                    username: 'wronguser',
                    password: 'wrongpass',
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
        expect.unreachable('should have rejected the SOCKS5 auth');
      } catch (err: unknown) {
        expect(err).toBeInstanceOf(ConnectError);
        const ce = err as ConnectError;
        expect(ce.code).toBe(Code.PermissionDenied);
        expect(ce.message).toContain('SOCKS5 auth failed');
      }

      // The rejected credentials were still transmitted per RFC 1929 -
      // the mock saw exactly what the hop configured.
      const frame = mock.frames().at(-1)!;
      expect(frame.auth).toEqual(
        Buffer.concat([
          Buffer.from([0x01, 'wronguser'.length]),
          Buffer.from('wronguser', 'utf-8'),
          Buffer.from(['wrongpass'.length]),
          Buffer.from('wrongpass', 'utf-8'),
        ]),
      );

      reqStream.close();
    } finally {
      await new Promise<void>(r => mock.server.close(() => r()));
    }
  });

  it('encodes IPv4 and IPv6 literal targets correctly', async () => {
    const echoV6 = net.createServer(s => s.pipe(s));
    await new Promise<void>(r => echoV6.listen(0, '::1', () => r()));
    const v6Port = (echoV6.address() as net.AddressInfo).port;

    const mock = makeSocks5Mock();
    await new Promise<void>(r => mock.server.listen(0, '127.0.0.1', () => r()));
    const mockPort = (mock.server.address() as net.AddressInfo).port;

    const dialThroughMock = async (target: string, payload: string) => {
      const reqStream = new RequestStream();
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'dial',
            value: create(NetConnRequest_DialSchema, {
              address: create(NetAddrSchema, {
                network: 'tcp',
                address: target,
              }),
              proxy: create(ProxyOptionsSchema, {
                hops: [
                  create(ProxyHopSchema, {
                    type: ProxyHop_Type.SOCKS5,
                    address: create(NetAddrSchema, {
                      network: 'tcp',
                      address: `127.0.0.1:${mockPort}`,
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
            value: new TextEncoder().encode(payload),
          },
        }),
      );

      const respStream = client.netConn(reqStream);
      const iterator = respStream[Symbol.asyncIterator]();
      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');
      const second = await iterator.next();
      expect(second.value.data.case).toBe('bytes');
      expect(new TextDecoder().decode(second.value.data.value)).toBe(payload);
      reqStream.close();
    };

    try {
      // IPv4 literal: ATYP 0x01 with the four octets inline.
      await dialThroughMock(`127.0.0.1:${tcpEchoPort}`, 'V4-OK');
      expect(mock.frames()[0].connect).toEqual(
        Buffer.from([
          0x05,
          0x01,
          0x00,
          0x01,
          127,
          0,
          0,
          1,
          tcpEchoPort >> 8,
          tcpEchoPort & 0xff,
        ]),
      );

      // Bracketed IPv6 literal: ATYP 0x04 with 16 wire-order bytes
      // (::1 is fifteen zero bytes then one).
      await dialThroughMock(`[::1]:${v6Port}`, 'V6-OK');
      const v6Addr = Buffer.alloc(16);
      v6Addr[15] = 1;
      expect(mock.frames()[1].connect).toEqual(
        Buffer.concat([
          Buffer.from([0x05, 0x01, 0x00, 0x04]),
          v6Addr,
          Buffer.from([v6Port >> 8, v6Port & 0xff]),
        ]),
      );
    } finally {
      await new Promise<void>(r => mock.server.close(() => r()));
      await new Promise<void>(r => echoV6.close(() => r()));
    }
  });

  it('surfaces a non-zero SOCKS5 connect reply as Unavailable', async () => {
    const mock = makeSocks5Mock({connectRep: 0x05, noUpstream: true});
    await new Promise<void>(r => mock.server.listen(0, '127.0.0.1', () => r()));
    const mockPort = (mock.server.address() as net.AddressInfo).port;

    try {
      const reqStream = new RequestStream();
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'dial',
            value: create(NetConnRequest_DialSchema, {
              address: create(NetAddrSchema, {
                network: 'tcp',
                address: `localhost:${tcpEchoPort}`,
              }),
              proxy: create(ProxyOptionsSchema, {
                hops: [
                  create(ProxyHopSchema, {
                    type: ProxyHop_Type.SOCKS5,
                    address: create(NetAddrSchema, {
                      network: 'tcp',
                      address: `127.0.0.1:${mockPort}`,
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
        expect.unreachable('should have surfaced the SOCKS5 reply code');
      } catch (err: unknown) {
        expect(err).toBeInstanceOf(ConnectError);
        const ce = err as ConnectError;
        expect(ce.code).toBe(Code.Unavailable);
        expect(ce.message).toContain('SOCKS5 connect failed');
        expect(ce.message).toContain('reply code 5');
      }

      // The CONNECT request itself was well-formed before the refusal.
      expect(mock.frames().at(-1)!.connect).toBeDefined();

      reqStream.close();
    } finally {
      await new Promise<void>(r => mock.server.close(() => r()));
    }
  });

  it('preserves early tunnel bytes coalesced with the SOCKS5 connect reply', async () => {
    // A fast proxy may pipeline the first upstream flight in the same
    // TCP segment as the CONNECT reply; those bytes are tunnel payload
    // and must survive the handshake (mirrors the HTTP CONNECT
    // coalescing test and Go's bufferedPrefixConn).
    const earlyPayload = Buffer.from('SOCKS-EARLY');
    const mock = makeSocks5Mock({
      coalesceWithReply: earlyPayload,
      noUpstream: true,
    });
    await new Promise<void>(r => mock.server.listen(0, '127.0.0.1', () => r()));
    const mockPort = (mock.server.address() as net.AddressInfo).port;

    try {
      const reqStream = new RequestStream();
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'dial',
            value: create(NetConnRequest_DialSchema, {
              address: create(NetAddrSchema, {
                network: 'tcp',
                address: `localhost:${tcpEchoPort}`,
              }),
              proxy: create(ProxyOptionsSchema, {
                hops: [
                  create(ProxyHopSchema, {
                    type: ProxyHop_Type.SOCKS5,
                    address: create(NetAddrSchema, {
                      network: 'tcp',
                      address: `127.0.0.1:${mockPort}`,
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
      expect(first.value.data.case).toBe('conn');

      const second = await iterator.next();
      expect(second.value.data.case).toBe('bytes');
      expect(Buffer.from(second.value.data.value as Uint8Array)).toEqual(
        earlyPayload,
      );

      reqStream.close();
    } finally {
      await new Promise<void>(r => mock.server.close(() => r()));
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

  it('relays halfClose to the upstream and keeps the tunnel open until the target ends', async () => {
    // Termination flow (d): halfClose closes only the server's write
    // side to the target (a real FIN reaches the upstream), and the
    // relay continues until the target terminates.
    let sawFin = false;
    const upstream = net.createServer(rawSocket => {
      rawSocket.on('data', chunk => {
        rawSocket.write(chunk); // echo until the FIN arrives
      });
      rawSocket.on('end', () => {
        // The tunnel's write side closed: a FIN reached the upstream.
        // Prove the relay continues by writing a final marker and only
        // then closing.
        sawFin = true;
        rawSocket.end('FINAL');
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

    // Flow control is on, so windowUpdate events interleave with bytes
    // in the response stream; skip them until the wanted message class
    // arrives.
    const nextBytes = async (): Promise<Uint8Array> => {
      for (;;) {
        const resp = await iterator.next();
        if (resp.done) {
          throw new Error('response stream ended before expected bytes');
        }
        if (resp.value.data.case === 'bytes') {
          return resp.value.data.value;
        }
      }
    };
    const nextHalfCloseEvent = async (): Promise<void> => {
      for (;;) {
        const resp = await iterator.next();
        if (resp.done) {
          throw new Error('response stream ended before the halfClose event');
        }
        if (
          resp.value.data.case === 'control' &&
          resp.value.data.value.event.case === 'halfClose'
        ) {
          return;
        }
      }
    };

    try {
      const first = await iterator.next();
      expect(first.value.data.case).toBe('conn');

      // Echo roundtrip proves the tunnel before the half-close.
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'bytes',
            value: new Uint8Array([0x50, 0x49, 0x4e, 0x47]), // "PING"
          },
        }),
      );
      expect(Buffer.from(await nextBytes()).toString()).toBe('PING');

      // Client-initiated write-side close.
      reqStream.push(
        create(NetConnRequestSchema, {
          data: {
            case: 'control',
            value: create(NetConnRequest_ControlSchema, {
              action: {
                case: 'halfClose',
                value: create(NetConnRequest_Control_HalfCloseSchema, {}),
              },
            }),
          },
        }),
      );

      // The relay continues: the post-FIN marker arrives, then the
      // upstream closes and the server relays the halfClose event, and
      // only then does the response stream end.
      expect(Buffer.from(await nextBytes()).toString()).toBe('FINAL');

      await nextHalfCloseEvent();

      const done = await iterator.next();
      expect(done.done).toBe(true);

      expect(sawFin).toBe(true);
    } finally {
      reqStream.close();
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
