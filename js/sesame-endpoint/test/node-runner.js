#!/usr/bin/env node
const assert = require('node:assert');
const net = require('node:net');
const tls = require('node:tls');
const http = require('node:http');
const http2 = require('node:http2');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const {execFileSync} = require('node:child_process');
const {create} = require('@bufbuild/protobuf');
const {createGrpcTransport} = require('@connectrpc/connect-node');
const {createClient, Code, ConnectError} = require('@connectrpc/connect');

const {
  parseConfig,
  formatHelp,
  createEndpointServer,
  FlowController,
  RemoteControl,
  NetConnRequestSchema,
  NetConnRequest_DialSchema,
  NetConnRequest_ControlSchema,
  NetConnRequest_Control_UpgradeTlsSchema,
  NetConnRequest_Control_ResetSchema,
  NetConnRequest_Control_HalfCloseSchema,
  NetConnRequest_Control_PingSchema,
  NetConnRequest_CapabilitiesSchema,
  TlsOptionsSchema,
  NetAddrSchema,
  ClientHelloSpecSchema,
  ProxyOptionsSchema,
  ProxyHopSchema,
  ProxyHop_Type,
} = require('../build/src/index.js');
const {StatusSchema} = require('../build/src/gen/google/rpc/status_pb.js');

const FIXTURES_DIR = path.join(__dirname, 'fixtures');
const CERT_PEM = fs.readFileSync(path.join(FIXTURES_DIR, 'cert.pem'));
const KEY_PEM = fs.readFileSync(path.join(FIXTURES_DIR, 'key.pem'));

class RequestStream {
  constructor() {
    this.queue = [];
    this.waiters = [];
    this.closed = false;
  }

  push(item) {
    if (this.closed) return;
    if (this.waiters.length > 0) {
      this.waiters.shift().resolve({value: item, done: false});
    } else {
      this.queue.push(item);
    }
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    while (this.waiters.length > 0) {
      this.waiters.shift().resolve({value: undefined, done: true});
    }
  }

  async *[Symbol.asyncIterator]() {
    while (true) {
      if (this.queue.length > 0) {
        yield this.queue.shift();
      } else if (this.closed) {
        return;
      } else {
        const item = await new Promise((resolve, reject) => {
          this.waiters.push({resolve, reject});
        });
        if (item.done) return;
        yield item.value;
      }
    }
  }
}

async function run() {
  console.log('--- Running sesame-endpoint verification under Node.js ---');

  // Test 1: Config parser
  console.log('1. Testing config parser...');
  const defaultCfg = parseConfig([]);
  assert.strictEqual(defaultCfg.config.port, 50051);
  assert.strictEqual(defaultCfg.config.host, '127.0.0.1');

  const cliCfg = parseConfig(['--host', '0.0.0.0', '--port', '8080']);
  assert.strictEqual(cliCfg.config.host, '0.0.0.0');
  assert.strictEqual(cliCfg.config.port, 8080);

  const envCfg = parseConfig([], {
    SESAME_ENDPOINT_HOST: '10.0.0.1',
    SESAME_ENDPOINT_PORT: '9090',
  });
  assert.strictEqual(envCfg.config.host, '10.0.0.1');
  assert.strictEqual(envCfg.config.port, 9090);
  assert.ok(formatHelp().includes('SESAME_ENDPOINT_'));

  // Test 2: FlowController
  console.log('2. Testing FlowController...');
  const fc = new FlowController(100);
  assert.strictEqual(fc.getCredit(), 100);
  await fc.acquire(50);
  assert.strictEqual(fc.getCredit(), 50);
  fc.addCredit(25);
  assert.strictEqual(fc.getCredit(), 75);

  // Test 3: Plaintext TCP Echo E2E
  console.log('3. Testing Plaintext TCP Echo E2E under Node.js...');
  const tcpEcho = net.createServer(s => s.pipe(s));
  await new Promise(r => tcpEcho.listen(0, '127.0.0.1', r));
  const tcpPort = tcpEcho.address().port;

  const {config} = parseConfig([]);
  const epServer = createEndpointServer(config);
  const bound = await epServer.listen(0, '127.0.0.1');

  const transport = createGrpcTransport({
    baseUrl: `http://127.0.0.1:${bound.port}`,
  });
  const client = createClient(RemoteControl, transport);

  const reqStream1 = new RequestStream();
  reqStream1.push(
    create(NetConnRequestSchema, {
      data: {
        case: 'dial',
        value: create(NetConnRequest_DialSchema, {
          address: create(NetAddrSchema, {
            network: 'tcp',
            address: `127.0.0.1:${tcpPort}`,
          }),
        }),
      },
    }),
  );
  reqStream1.push(
    create(NetConnRequestSchema, {
      data: {
        case: 'bytes',
        value: Buffer.from('Node Plaintext Message'),
      },
    }),
  );

  const respStream1 = client.netConn(reqStream1);
  const it1 = respStream1[Symbol.asyncIterator]();
  const conn1 = await it1.next();
  assert.strictEqual(conn1.value.data.case, 'conn');
  const echo1 = await it1.next();
  assert.strictEqual(echo1.value.data.case, 'bytes');
  assert.strictEqual(
    Buffer.from(echo1.value.data.value).toString('utf-8'),
    'Node Plaintext Message',
  );
  reqStream1.close();

  // Test 4: Endpoint TLS Termination & ALPN under Node.js
  console.log('4. Testing Endpoint TLS Termination with ALPN under Node.js...');
  const tlsEcho = tls.createServer(
    {cert: CERT_PEM, key: KEY_PEM, ALPNProtocols: ['test-node', 'h2']},
    s => s.pipe(s),
  );
  await new Promise(r => tlsEcho.listen(0, '127.0.0.1', r));
  const tlsPort = tlsEcho.address().port;

  const reqStream2 = new RequestStream();
  reqStream2.push(
    create(NetConnRequestSchema, {
      data: {
        case: 'dial',
        value: create(NetConnRequest_DialSchema, {
          address: create(NetAddrSchema, {
            network: 'tcp',
            address: `127.0.0.1:${tlsPort}`,
          }),
          tls: create(TlsOptionsSchema, {
            sni: 'localhost',
            alpnProtocols: ['test-node'],
            insecureSkipVerify: true,
          }),
        }),
      },
    }),
  );
  reqStream2.push(
    create(NetConnRequestSchema, {
      data: {
        case: 'bytes',
        value: Buffer.from('Node TLS Message'),
      },
    }),
  );

  const respStream2 = client.netConn(reqStream2);
  const it2 = respStream2[Symbol.asyncIterator]();
  const conn2 = await it2.next();
  assert.strictEqual(conn2.value.data.case, 'conn');
  assert.strictEqual(
    conn2.value.data.value.tls.negotiatedProtocol,
    'test-node',
  );
  // Wire-contract parity: negotiated cipher suite is reported (non-zero)
  // and the peer chain is present under Node too.
  assert.notStrictEqual(conn2.value.data.value.tls.cipherSuite, 0);
  assert.ok(conn2.value.data.value.tls.peerCertificates.length > 0);
  const echo2 = await it2.next();
  assert.strictEqual(echo2.value.data.case, 'bytes');
  assert.strictEqual(
    Buffer.from(echo2.value.data.value).toString('utf-8'),
    'Node TLS Message',
  );
  reqStream2.close();

  // Test 5: In-Stream Ping/Pong under Node.js
  console.log('5. Testing In-Stream Ping/Pong under Node.js...');
  const reqStream3 = new RequestStream();
  reqStream3.push(
    create(NetConnRequestSchema, {
      data: {
        case: 'dial',
        value: create(NetConnRequest_DialSchema, {
          address: create(NetAddrSchema, {
            network: 'tcp',
            address: `127.0.0.1:${tcpPort}`,
          }),
          capabilities: create(NetConnRequest_CapabilitiesSchema, {
            supportsOpportunisticTls: true,
            supportsFlowControl: true,
          }),
        }),
      },
    }),
  );
  const respStream3 = client.netConn(reqStream3);
  const it3 = respStream3[Symbol.asyncIterator]();
  const conn3 = await it3.next();
  assert.strictEqual(conn3.value.data.case, 'conn');

  reqStream3.push(
    create(NetConnRequestSchema, {
      data: {
        case: 'control',
        value: create(NetConnRequest_ControlSchema, {
          action: {
            case: 'ping',
            value: create(NetConnRequest_Control_PingSchema, {
              id: 777n,
              timestampNanos: 999999n,
            }),
          },
        }),
      },
    }),
  );
  const pong3 = await it3.next();
  assert.strictEqual(pong3.value.data.case, 'control');
  assert.strictEqual(pong3.value.data.value.event.case, 'pong');
  assert.strictEqual(pong3.value.data.value.event.value.id, 777n);
  reqStream3.close();

  // Test 6: Fail-Closed non-default ClientHello dimension under Node.js
  console.log('6. Testing Fail-Closed ClientHello rejection under Node.js...');
  const reqStream4 = new RequestStream();
  reqStream4.push(
    create(NetConnRequestSchema, {
      data: {
        case: 'dial',
        value: create(NetConnRequest_DialSchema, {
          address: create(NetAddrSchema, {
            network: 'tcp',
            address: `127.0.0.1:${tlsPort}`,
          }),
          tls: create(TlsOptionsSchema, {
            sni: 'localhost',
            insecureSkipVerify: true,
            clientHello: create(ClientHelloSpecSchema, {
              signatureAlgorithms: [0x0403],
            }),
          }),
        }),
      },
    }),
  );
  const respStream4 = client.netConn(reqStream4);
  const it4 = respStream4[Symbol.asyncIterator]();
  try {
    await it4.next();
    assert.fail('should have failed closed');
  } catch (err) {
    assert.ok(err instanceof ConnectError);
    assert.strictEqual(err.code, Code.FailedPrecondition);
    assert.ok(err.message.includes('signature_algorithms'));
  }
  reqStream4.close();

  // Test 7: Reject upgrade_tls without options under Node.js
  console.log('7. Testing options-less upgradeTls rejection under Node.js...');
  const reqStream5 = new RequestStream();
  reqStream5.push(
    create(NetConnRequestSchema, {
      data: {
        case: 'dial',
        value: create(NetConnRequest_DialSchema, {
          address: create(NetAddrSchema, {
            network: 'tcp',
            address: `127.0.0.1:${tcpPort}`,
          }),
          capabilities: create(NetConnRequest_CapabilitiesSchema, {
            supportsOpportunisticTls: true,
            supportsFlowControl: true,
          }),
        }),
      },
    }),
  );
  const respStream5 = client.netConn(reqStream5);
  const it5 = respStream5[Symbol.asyncIterator]();
  const conn5 = await it5.next();
  assert.strictEqual(conn5.value.data.case, 'conn');

  reqStream5.push(
    create(NetConnRequestSchema, {
      data: {
        case: 'control',
        value: create(NetConnRequest_ControlSchema, {
          action: {
            case: 'upgradeTls',
            value: create(NetConnRequest_Control_UpgradeTlsSchema, {}),
          },
        }),
      },
    }),
  );

  let sawInvalidArgument = false;
  try {
    while (true) {
      const resp = await it5.next();
      if (resp.done) break;
      if (resp.value.data.case === 'control') {
        const evt = resp.value.data.value.event;
        if (evt.case === 'tlsUpgraded') {
          assert.fail('server emitted tlsUpgraded for an options-less upgrade');
        }
      }
    }
  } catch (err) {
    assert.ok(
      err instanceof ConnectError,
      `expected ConnectError, got: ${err}`,
    );
    assert.strictEqual(err.code, Code.InvalidArgument);
    sawInvalidArgument = true;
  }
  assert.ok(sawInvalidArgument, 'expected InvalidArgument stream termination');
  reqStream5.close();

  // Test 8: Reset surfaces as an error carrying the reason under Node.js
  console.log('8. Testing reset-reason surfacing under Node.js...');
  const resetStream = new RequestStream();
  resetStream.push(
    create(NetConnRequestSchema, {
      data: {
        case: 'dial',
        value: create(NetConnRequest_DialSchema, {
          address: create(NetAddrSchema, {
            network: 'tcp',
            address: `127.0.0.1:${tcpPort}`,
          }),
          capabilities: create(NetConnRequest_CapabilitiesSchema, {
            supportsOpportunisticTls: true,
            supportsFlowControl: true,
          }),
        }),
      },
    }),
  );
  const resetResp = client.netConn(resetStream);
  const resetIt = resetResp[Symbol.asyncIterator]();
  const resetConn = await resetIt.next();
  assert.strictEqual(resetConn.value.data.case, 'conn');

  resetStream.push(
    create(NetConnRequestSchema, {
      data: {
        case: 'control',
        value: create(NetConnRequest_ControlSchema, {
          action: {
            case: 'reset',
            value: create(NetConnRequest_Control_ResetSchema, {
              reason: create(StatusSchema, {
                code: Code.Canceled,
                message: 'node-runner reset probe',
              }),
            }),
          },
        }),
      },
    }),
  );

  let resetErr;
  try {
    while (true) {
      const resp = await resetIt.next();
      if (resp.done) break;
    }
    assert.fail('expected reset to terminate the stream with an error');
  } catch (err) {
    resetErr = err;
  }
  assert.ok(
    resetErr instanceof ConnectError,
    `expected ConnectError, got: ${resetErr}`,
  );
  assert.ok(
    resetErr.message.includes('connection reset by client'),
    resetErr.message,
  );
  assert.ok(
    resetErr.message.includes('node-runner reset probe'),
    resetErr.message,
  );
  assert.strictEqual(resetErr.code, Code.Canceled);
  resetStream.close();

  // Test 9: Request-stream EOF fully closes the upstream under Node.js
  console.log('9. Testing request-EOF full-close semantics under Node.js...');
  let upstreamFullyClosed = false;
  const silentUpstream = net.createServer(sock => {
    sock.on('close', () => {
      upstreamFullyClosed = true;
    });
    sock.on('data', () => {});
  });
  await new Promise(r => silentUpstream.listen(0, '127.0.0.1', () => r()));
  const silentPort = silentUpstream.address().port;

  const eofStream = new RequestStream();
  eofStream.push(
    create(NetConnRequestSchema, {
      data: {
        case: 'dial',
        value: create(NetConnRequest_DialSchema, {
          address: create(NetAddrSchema, {
            network: 'tcp',
            address: `127.0.0.1:${silentPort}`,
          }),
          capabilities: create(NetConnRequest_CapabilitiesSchema, {
            supportsOpportunisticTls: true,
            supportsFlowControl: true,
          }),
        }),
      },
    }),
  );
  const eofResp = client.netConn(eofStream);
  const eofIt = eofResp[Symbol.asyncIterator]();
  const eofConn = await eofIt.next();
  assert.strictEqual(eofConn.value.data.case, 'conn');

  eofStream.close();
  const eofDone = await eofIt.next();
  assert.ok(eofDone.done, 'response stream must complete on request EOF');

  const eofDeadline = Date.now() + 2000;
  while (!upstreamFullyClosed && Date.now() < eofDeadline) {
    await new Promise(r => setTimeout(r, 20));
  }
  assert.ok(
    upstreamFullyClosed,
    'upstream socket must be fully closed on request EOF',
  );
  await new Promise(r => silentUpstream.close(() => r()));

  // Byte-level SOCKS5 mock (RFC 1928): a greeting -> (auth) -> connect
  // state machine that records every frame the client sent and, on
  // success, pipes to the real target the client asked for. The
  // tunnel state is entered at CONNECT-parse time and the pipe to the
  // upstream attaches immediately (net buffers writes to a connecting
  // socket), because the endpoint writes tunnel payload the instant
  // the handshake resolves.
  function makeSocks5Mock(opts = {}) {
    const frames = [];
    const server = net.createServer(raw => {
      let stage = 'greeting';
      let buf = Buffer.alloc(0);
      const frame = {greeting: Buffer.alloc(0), auth: null, connect: null};
      frames.push(frame);
      let upstream = null;

      const dialUpstream = connect => {
        const atyp = connect[3];
        let host;
        let addrOff;
        let addrLen;
        if (atyp === 0x01) {
          host = `${connect[4]}.${connect[5]}.${connect[6]}.${connect[7]}`;
          addrOff = 4;
          addrLen = 4;
        } else {
          const domainLen = connect[4];
          host = connect.subarray(5, 5 + domainLen).toString('utf-8');
          addrOff = 5 + domainLen;
          addrLen = 0;
        }
        const port =
          (connect[addrOff + addrLen] << 8) | connect[addrOff + addrLen + 1];
        upstream = net.connect(port, host, () => {
          upstream.pipe(raw);
        });
        raw.pipe(upstream);
        upstream.on('error', () => raw.destroy());
      };

      raw.on('data', chunk => {
        if (stage === 'tunnel') {
          // raw -> upstream is handled by the pipe.
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

          stage = 'tunnel';
          raw.write(
            Buffer.concat([
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
            ]),
          );
          if ((opts.connectRep ?? 0x00) !== 0x00) {
            raw.destroy();
            return;
          }
          if (!opts.noUpstream) {
            dialUpstream(frame.connect);
          }
        }
      });

      raw.on('close', () => upstream && upstream.destroy());
      raw.on('error', () => upstream && upstream.destroy());
    });
    return {server, frames: () => frames};
  }

  // Reads responses until a bytes payload arrives, skipping flow-control
  // windowUpdate events (control-mode connections interleave them).
  async function nextBytes(it) {
    for (;;) {
      const resp = await it.next();
      if (resp.done) {
        throw new Error('response stream ended before expected bytes');
      }
      if (resp.value.data.case === 'bytes') {
        return resp.value.data.value;
      }
    }
  }

  const proxyDial = (reqStream, target, hop) => {
    reqStream.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {network: 'tcp', address: target}),
            proxy: create(ProxyOptionsSchema, {hops: [hop]}),
          }),
        },
      }),
    );
  };

  // Test 10: SOCKS5 traversal with the framing bytes verified
  console.log('10. Testing SOCKS5 proxy traversal under Node.js...');
  const socksMock = makeSocks5Mock();
  await new Promise(r => socksMock.server.listen(0, '127.0.0.1', r));
  const socksPort = socksMock.server.address().port;
  try {
    const reqStream10 = new RequestStream();
    proxyDial(
      reqStream10,
      `localhost:${tcpPort}`,
      create(ProxyHopSchema, {
        type: ProxyHop_Type.SOCKS5,
        address: create(NetAddrSchema, {
          network: 'tcp',
          address: `127.0.0.1:${socksPort}`,
        }),
      }),
    );
    reqStream10.push(
      create(NetConnRequestSchema, {
        data: {case: 'bytes', value: Buffer.from('NODE SOCKS5 TRAVERSAL')},
      }),
    );

    const resp10 = client.netConn(reqStream10);
    const it10 = resp10[Symbol.asyncIterator]();
    const conn10 = await it10.next();
    assert.strictEqual(conn10.value.data.case, 'conn');
    assert.strictEqual(conn10.value.data.value.proxy.traversedHops.length, 1);
    const echo10 = await it10.next();
    assert.strictEqual(echo10.value.data.case, 'bytes');
    assert.strictEqual(
      Buffer.from(echo10.value.data.value).toString('utf-8'),
      'NODE SOCKS5 TRAVERSAL',
    );

    // The framing is the thing under test: exact greeting and
    // domain-typed CONNECT bytes.
    const frame = socksMock.frames().at(-1);
    assert.ok(
      Buffer.compare(frame.greeting, Buffer.from([0x05, 0x01, 0x00])) === 0,
      `greeting = ${frame.greeting.toString('hex')}`,
    );
    assert.strictEqual(frame.auth, null);
    const wantConnect = Buffer.concat([
      Buffer.from([0x05, 0x01, 0x00, 0x03, 'localhost'.length]),
      Buffer.from('localhost', 'utf-8'),
      Buffer.from([tcpPort >> 8, tcpPort & 0xff]),
    ]);
    assert.ok(
      Buffer.compare(frame.connect, wantConnect) === 0,
      `connect = ${frame.connect.toString('hex')}`,
    );
    reqStream10.close();
  } finally {
    await new Promise(r => socksMock.server.close(() => r()));
  }

  // Test 11: early tunnel bytes coalesced with the SOCKS5 connect reply
  console.log(
    '11. Testing SOCKS5 coalesced connect-reply bytes under Node.js...',
  );
  const earlyPayload = Buffer.from('SOCKS-EARLY');
  const coalesceMock = makeSocks5Mock({
    coalesceWithReply: earlyPayload,
    noUpstream: true,
  });
  await new Promise(r => coalesceMock.server.listen(0, '127.0.0.1', r));
  const coalescePort = coalesceMock.server.address().port;
  try {
    const reqStream11 = new RequestStream();
    proxyDial(
      reqStream11,
      `localhost:${tcpPort}`,
      create(ProxyHopSchema, {
        type: ProxyHop_Type.SOCKS5,
        address: create(NetAddrSchema, {
          network: 'tcp',
          address: `127.0.0.1:${coalescePort}`,
        }),
      }),
    );

    const resp11 = client.netConn(reqStream11);
    const it11 = resp11[Symbol.asyncIterator]();
    const conn11 = await it11.next();
    assert.strictEqual(conn11.value.data.case, 'conn');
    const early11 = await it11.next();
    assert.strictEqual(early11.value.data.case, 'bytes');
    assert.ok(
      Buffer.compare(Buffer.from(early11.value.data.value), earlyPayload) === 0,
      'coalesced early tunnel bytes must arrive in order',
    );
    reqStream11.close();
  } finally {
    await new Promise(r => coalesceMock.server.close(() => r()));
  }

  // Test 12: HTTP CONNECT traversal
  console.log('12. Testing HTTP CONNECT proxy traversal under Node.js...');
  const httpProxy = http.createServer();
  httpProxy.on('connect', (req, clientSocket, head) => {
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
  await new Promise(r => httpProxy.listen(0, '127.0.0.1', r));
  const httpProxyPort = httpProxy.address().port;
  try {
    const reqStream12 = new RequestStream();
    proxyDial(
      reqStream12,
      `127.0.0.1:${tcpPort}`,
      create(ProxyHopSchema, {
        type: ProxyHop_Type.HTTP_CONNECT,
        address: create(NetAddrSchema, {
          network: 'tcp',
          address: `127.0.0.1:${httpProxyPort}`,
        }),
      }),
    );
    reqStream12.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'bytes',
          value: Buffer.from('NODE HTTP CONNECT TRAVERSAL'),
        },
      }),
    );

    const resp12 = client.netConn(reqStream12);
    const it12 = resp12[Symbol.asyncIterator]();
    const conn12 = await it12.next();
    assert.strictEqual(conn12.value.data.case, 'conn');
    assert.strictEqual(conn12.value.data.value.proxy.traversedHops.length, 1);
    const echo12 = await it12.next();
    assert.strictEqual(echo12.value.data.case, 'bytes');
    assert.strictEqual(
      Buffer.from(echo12.value.data.value).toString('utf-8'),
      'NODE HTTP CONNECT TRAVERSAL',
    );
    reqStream12.close();
  } finally {
    await new Promise(r => httpProxy.close(() => r()));
  }

  // Test 13: halfClose delivers a real FIN and the relay continues
  console.log('13. Testing client halfClose under Node.js...');
  let sawFin = false;
  const halfCloseUpstream = net.createServer(rawSocket => {
    rawSocket.on('data', chunk => {
      rawSocket.write(chunk);
    });
    rawSocket.on('end', () => {
      sawFin = true;
      rawSocket.end('FINAL');
    });
  });
  await new Promise(r => halfCloseUpstream.listen(0, '127.0.0.1', r));
  const halfClosePort = halfCloseUpstream.address().port;
  try {
    const reqStream13 = new RequestStream();
    reqStream13.push(
      create(NetConnRequestSchema, {
        data: {
          case: 'dial',
          value: create(NetConnRequest_DialSchema, {
            address: create(NetAddrSchema, {
              network: 'tcp',
              address: `127.0.0.1:${halfClosePort}`,
            }),
            capabilities: create(NetConnRequest_CapabilitiesSchema, {
              supportsOpportunisticTls: true,
              supportsFlowControl: true,
            }),
          }),
        },
      }),
    );

    const resp13 = client.netConn(reqStream13);
    const it13 = resp13[Symbol.asyncIterator]();
    const conn13 = await it13.next();
    assert.strictEqual(conn13.value.data.case, 'conn');

    // Echo roundtrip proves the tunnel before the half-close.
    reqStream13.push(
      create(NetConnRequestSchema, {
        data: {case: 'bytes', value: Buffer.from('PING')},
      }),
    );
    const echo13 = await nextBytes(it13);
    assert.strictEqual(Buffer.from(echo13).toString('utf-8'), 'PING');

    // Client-initiated write-side close: the upstream must see a FIN,
    // keep the relay running for the post-FIN marker, and then end
    // cleanly via the halfClose event.
    reqStream13.push(
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

    const final13 = await nextBytes(it13);
    assert.strictEqual(Buffer.from(final13).toString('utf-8'), 'FINAL');

    let sawHalfCloseEvent = false;
    for (;;) {
      const resp = await it13.next();
      if (resp.done) break;
      if (
        resp.value.data.case === 'control' &&
        resp.value.data.value.event.case === 'halfClose'
      ) {
        sawHalfCloseEvent = true;
      }
    }
    assert.ok(
      sawHalfCloseEvent,
      'expected the halfClose control event before stream end',
    );
    assert.ok(sawFin, 'upstream must have observed the FIN');
    reqStream13.close();
  } finally {
    await new Promise(r => halfCloseUpstream.close(() => r()));
  }

  // Test 14: CLI reports TLS material errors as a friendly one-liner,
  // never a raw stack. Malformed PEM fails at TLS context construction
  // under Node.js (createSecureServer), the path the CLI's
  // construction-time error handling exists to cover.
  console.log('14. Testing CLI TLS error reporting under Node.js...');
  {
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'sesame-cli-node-'));
    try {
      const badCert = path.join(tmp, 'bad-cert.pem');
      const badKey = path.join(tmp, 'bad-key.pem');
      fs.writeFileSync(
        badCert,
        '-----BEGIN CERTIFICATE-----\nnot-base64\n-----END CERTIFICATE-----\n',
      );
      fs.writeFileSync(
        badKey,
        '-----BEGIN PRIVATE KEY-----\nnot-base64\n-----END PRIVATE KEY-----\n',
      );

      const cleanEnv = Object.fromEntries(
        Object.entries(process.env).filter(
          ([k]) => !k.startsWith('SESAME_ENDPOINT_'),
        ),
      );

      let status = 0;
      let stderr = '';
      try {
        execFileSync(
          process.execPath,
          [path.join(__dirname, '..', 'build', 'src', 'cli.js')],
          {
            // Capture stderr (asserted below) instead of letting execFileSync
            // echo it to this runner's stderr: the failure is expected.
            stdio: ['ignore', 'pipe', 'pipe'],
            env: {
              ...cleanEnv,
              SESAME_ENDPOINT_TLS_CERT: badCert,
              SESAME_ENDPOINT_TLS_KEY: badKey,
            },
            encoding: 'utf8',
            timeout: 30000,
          },
        );
      } catch (err) {
        status = err.status;
        stderr = String(err.stderr);
      }
      assert.strictEqual(status, 1, `expected exit 1, got ${status}`);
      assert.ok(
        stderr.startsWith('sesame-endpoint: '),
        `expected a friendly CLI error, got: ${stderr}`,
      );
      assert.ok(
        !stderr.includes('\n    at '),
        `expected no raw stack trace, got: ${stderr}`,
      );
    } finally {
      fs.rmSync(tmp, {recursive: true, force: true});
    }
  }

  // Test 15: Endpoint mTLS Listener Handshake under Node.js runtime
  // Proves that Node.js accepts trusted client certs and rejects missing or untrusted client certs.
  console.log('15. Testing Endpoint mTLS Listener Handshake under Node.js...');
  {
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'sesame-mtls-node-'));
    let epServer;
    try {
      const caExt = path.join(tmp, 'ca.ext');
      fs.writeFileSync(
        caExt,
        '[v3_ca]\nbasicConstraints=critical,CA:TRUE\nkeyUsage=critical,keyCertSign,cRLSign\n',
      );
      const leafExt = path.join(tmp, 'leaf.ext');
      fs.writeFileSync(
        leafExt,
        '[v3_leaf]\nbasicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=clientAuth\n',
      );

      function genPair(prefix, cn) {
        const caKey = path.join(tmp, `${prefix}-ca.key`);
        const caCsr = path.join(tmp, `${prefix}-ca.csr`);
        const caCert = path.join(tmp, `${prefix}-ca.crt`);
        const leafKey = path.join(tmp, `${prefix}.key`);
        const leafCsr = path.join(tmp, `${prefix}.csr`);
        const leafCert = path.join(tmp, `${prefix}.crt`);

        execFileSync(
          'openssl',
          [
            'req',
            '-nodes',
            '-newkey',
            'rsa:2048',
            '-keyout',
            caKey,
            '-out',
            caCsr,
            '-subj',
            `/CN=${cn} CA`,
          ],
          {stdio: 'pipe'},
        );
        execFileSync(
          'openssl',
          [
            'x509',
            '-req',
            '-in',
            caCsr,
            '-signkey',
            caKey,
            '-out',
            caCert,
            '-days',
            '1',
            '-extfile',
            caExt,
            '-extensions',
            'v3_ca',
          ],
          {stdio: 'pipe'},
        );
        execFileSync(
          'openssl',
          [
            'req',
            '-nodes',
            '-newkey',
            'rsa:2048',
            '-keyout',
            leafKey,
            '-out',
            leafCsr,
            '-subj',
            `/CN=${cn}`,
          ],
          {stdio: 'pipe'},
        );
        execFileSync(
          'openssl',
          [
            'x509',
            '-req',
            '-in',
            leafCsr,
            '-CA',
            caCert,
            '-CAkey',
            caKey,
            '-CAcreateserial',
            '-out',
            leafCert,
            '-days',
            '1',
            '-extfile',
            leafExt,
            '-extensions',
            'v3_leaf',
          ],
          {stdio: 'pipe'},
        );

        return {caCert, leafCert, leafKey};
      }

      const trusted = genPair('trusted', 'Node Trusted');
      const untrusted = genPair('untrusted', 'Node Untrusted');

      const {config} = parseConfig([
        '--host=127.0.0.1',
        `--tls-cert=${path.join(FIXTURES_DIR, 'cert.pem')}`,
        `--tls-key=${path.join(FIXTURES_DIR, 'key.pem')}`,
        `--client-ca-cert=${trusted.caCert}`,
      ]);
      epServer = createEndpointServer(config);
      const bound = await epServer.listen(0, '127.0.0.1');

      async function attempt(opts) {
        const client = http2.connect(`https://127.0.0.1:${bound.port}`, opts);
        let timer;
        try {
          return await new Promise(resolve => {
            timer = setTimeout(() => resolve(false), 5000);
            client.on('error', () => resolve(false));
            client.on('close', () => resolve(false));
            client.on('connect', () => {
              const req = client.request({':path': '/'});
              req.on('error', () => resolve(false));
              req.on('close', () => resolve(false));
              req.on('response', () => resolve(true));
              req.end();
            });
          });
        } finally {
          if (timer) clearTimeout(timer);
          client.destroy();
        }
      }

      // Case 1: Trusted client cert is accepted
      const accepted = await attempt({
        ca: fs.readFileSync(path.join(FIXTURES_DIR, 'cert.pem')),
        cert: fs.readFileSync(trusted.leafCert),
        key: fs.readFileSync(trusted.leafKey),
        rejectUnauthorized: false,
      });
      assert.strictEqual(
        accepted,
        true,
        'expected trusted client cert to be accepted under Node.js',
      );

      // Case 2: Missing client cert is rejected
      const rejectedNoCert = await attempt({
        ca: fs.readFileSync(path.join(FIXTURES_DIR, 'cert.pem')),
        rejectUnauthorized: false,
      });
      assert.strictEqual(
        rejectedNoCert,
        false,
        'expected missing client cert to be rejected under Node.js',
      );

      // Case 3: Untrusted client cert is rejected
      const rejectedUntrusted = await attempt({
        ca: fs.readFileSync(path.join(FIXTURES_DIR, 'cert.pem')),
        cert: fs.readFileSync(untrusted.leafCert),
        key: fs.readFileSync(untrusted.leafKey),
        rejectUnauthorized: false,
      });
      assert.strictEqual(
        rejectedUntrusted,
        false,
        'expected untrusted client cert to be rejected under Node.js',
      );
    } finally {
      if (epServer) await epServer.close();
      fs.rmSync(tmp, {recursive: true, force: true});
    }
  }

  // 16. Testing release packaging integrity and CLI version under Node.js
  {
    process.stdout.write(
      '16. Testing release packaging integrity and CLI version under Node.js...\n',
    );
    const pkgJson = JSON.parse(
      fs.readFileSync(path.join(__dirname, '..', 'package.json'), 'utf-8'),
    );
    assert.strictEqual(pkgJson.name, 'sesame-endpoint');
    assert.strictEqual(pkgJson.license, 'MIT');
    assert.deepStrictEqual(pkgJson.files, ['build/src']);
    assert.ok(
      fs.existsSync(path.join(__dirname, '..', 'LICENSE')),
      'LICENSE must exist in package',
    );

    const cleanEnv = Object.fromEntries(
      Object.entries(process.env).filter(
        ([key]) => !key.startsWith('SESAME_ENDPOINT_'),
      ),
    );
    const cliPath = path.join(__dirname, '..', 'build', 'src', 'cli.js');
    const verOut = execFileSync(process.execPath, [cliPath, '--version'], {
      encoding: 'utf-8',
      env: cleanEnv,
    });
    assert.strictEqual(verOut, `sesame-endpoint v${pkgJson.version}\n`);

    const helpOut = execFileSync(process.execPath, [cliPath, '--help'], {
      encoding: 'utf-8',
      env: cleanEnv,
    });
    assert.ok(
      helpOut.includes('Usage: sesame-endpoint'),
      'cli --help must contain usage',
    );
  }

  // 17. Testing CLI version resolution failure under Node.js
  // A broken install with no sesame-endpoint manifest above the CLI must
  // report the friendly one-liner, never an unhandled-rejection stack —
  // the Node.js counterpart of the orphan-CLI test in mtls.test.ts.
  console.log(
    '17. Testing CLI version resolution error reporting under Node.js...',
  );
  {
    const orphanDir = fs.mkdtempSync(
      path.join(os.tmpdir(), 'sesame-cli-orphan-'),
    );
    try {
      fs.cpSync(
        path.join(__dirname, '..', 'build', 'src'),
        path.join(orphanDir, 'build', 'src'),
        {recursive: true},
      );
      fs.symlinkSync(
        path.join(__dirname, '..', 'node_modules'),
        path.join(orphanDir, 'node_modules'),
        process.platform === 'win32' ? 'junction' : 'dir',
      );

      const cleanEnv = Object.fromEntries(
        Object.entries(process.env).filter(
          ([k]) => !k.startsWith('SESAME_ENDPOINT_'),
        ),
      );

      let status = 0;
      let stdout = '';
      let stderr = '';
      try {
        stdout = execFileSync(
          process.execPath,
          [path.join(orphanDir, 'build', 'src', 'cli.js'), '--version'],
          {
            // Capture the expected failure instead of letting execFileSync
            // echo it to this runner's stderr.
            stdio: ['ignore', 'pipe', 'pipe'],
            env: cleanEnv,
            encoding: 'utf8',
            timeout: 30000,
          },
        );
      } catch (err) {
        status = err.status;
        stdout = String(err.stdout);
        stderr = String(err.stderr);
      }
      assert.strictEqual(status, 1, `expected exit 1, got ${status}`);
      assert.ok(
        stderr.startsWith('sesame-endpoint: '),
        `expected a friendly CLI error, got: ${stderr}`,
      );
      assert.ok(
        stderr.includes('unable to resolve sesame-endpoint version'),
        `expected the version resolution message, got: ${stderr}`,
      );
      assert.ok(
        !stderr.includes('\n    at '),
        `expected no raw stack trace, got: ${stderr}`,
      );
      assert.strictEqual(stdout, '', `expected no stdout, got: ${stdout}`);
    } finally {
      fs.rmSync(orphanDir, {recursive: true, force: true});
    }
  }

  console.log('All Node.js runtime tests passed cleanly!');
  process.exit(0);
}

run().catch(err => {
  console.error('Test failure:', err);
  process.exit(1);
});
