#!/usr/bin/env node
const assert = require('node:assert');
const net = require('node:net');
const tls = require('node:tls');
const fs = require('node:fs');
const path = require('node:path');
const { create } = require('@bufbuild/protobuf');
const { createGrpcTransport } = require('@connectrpc/connect-node');
const { createClient, Code, ConnectError } = require('@connectrpc/connect');

const {
  parseConfig,
  formatHelp,
  createEndpointServer,
  FlowController,
  RemoteControl,
  NetConnRequestSchema,
  NetConnRequest_DialSchema,
  NetConnRequest_ControlSchema,
  NetConnRequest_Control_UpgradeTLSSchema,
  NetConnRequest_Control_PingSchema,
  NetConnRequest_CapabilitiesSchema,
  TLSOptionsSchema,
  NetAddrSchema,
  FingerprintPreset,
} = require('../build/src/index.js');

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
      this.waiters.shift().resolve({ value: item, done: false });
    } else {
      this.queue.push(item);
    }
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    while (this.waiters.length > 0) {
      this.waiters.shift().resolve({ value: undefined, done: true });
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
          this.waiters.push({ resolve, reject });
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

  const envCfg = parseConfig([], { SESAME_ENDPOINT_HOST: '10.0.0.1', SESAME_ENDPOINT_PORT: '9090' });
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

  const { config } = parseConfig([]);
  const epServer = createEndpointServer(config);
  const bound = await epServer.listen(0, '127.0.0.1');

  const transport = createGrpcTransport({ baseUrl: `http://127.0.0.1:${bound.port}` });
  const client = createClient(RemoteControl, transport);

  const reqStream1 = new RequestStream();
  reqStream1.push(create(NetConnRequestSchema, {
    data: {
      case: 'dial',
      value: create(NetConnRequest_DialSchema, {
        address: create(NetAddrSchema, { network: 'tcp', address: `127.0.0.1:${tcpPort}` }),
      }),
    },
  }));
  reqStream1.push(create(NetConnRequestSchema, {
    data: {
      case: 'bytes',
      value: Buffer.from('Node Plaintext Message'),
    },
  }));

  const respStream1 = client.netConn(reqStream1);
  const it1 = respStream1[Symbol.asyncIterator]();
  const conn1 = await it1.next();
  assert.strictEqual(conn1.value.data.case, 'conn');
  const echo1 = await it1.next();
  assert.strictEqual(echo1.value.data.case, 'bytes');
  assert.strictEqual(Buffer.from(echo1.value.data.value).toString('utf-8'), 'Node Plaintext Message');
  reqStream1.close();

  // Test 4: Endpoint TLS Termination & ALPN under Node.js
  console.log('4. Testing Endpoint TLS Termination with ALPN under Node.js...');
  const tlsEcho = tls.createServer({ cert: CERT_PEM, key: KEY_PEM, ALPNProtocols: ['test-node', 'h2'] }, s => s.pipe(s));
  await new Promise(r => tlsEcho.listen(0, '127.0.0.1', r));
  const tlsPort = tlsEcho.address().port;

  const reqStream2 = new RequestStream();
  reqStream2.push(create(NetConnRequestSchema, {
    data: {
      case: 'dial',
      value: create(NetConnRequest_DialSchema, {
        address: create(NetAddrSchema, { network: 'tcp', address: `127.0.0.1:${tlsPort}` }),
        tls: create(TLSOptionsSchema, {
          serverName: 'localhost',
          alpnProtocols: ['test-node'],
          insecureSkipVerify: true,
        }),
      }),
    },
  }));
  reqStream2.push(create(NetConnRequestSchema, {
    data: {
      case: 'bytes',
      value: Buffer.from('Node TLS Message'),
    },
  }));

  const respStream2 = client.netConn(reqStream2);
  const it2 = respStream2[Symbol.asyncIterator]();
  const conn2 = await it2.next();
  assert.strictEqual(conn2.value.data.case, 'conn');
  assert.strictEqual(conn2.value.data.value.tls.negotiatedProtocol, 'test-node');
  const echo2 = await it2.next();
  assert.strictEqual(echo2.value.data.case, 'bytes');
  assert.strictEqual(Buffer.from(echo2.value.data.value).toString('utf-8'), 'Node TLS Message');
  reqStream2.close();

  // Test 5: In-Stream Ping/Pong under Node.js
  console.log('5. Testing In-Stream Ping/Pong under Node.js...');
  const reqStream3 = new RequestStream();
  reqStream3.push(create(NetConnRequestSchema, {
    data: {
      case: 'dial',
      value: create(NetConnRequest_DialSchema, {
        address: create(NetAddrSchema, { network: 'tcp', address: `127.0.0.1:${tcpPort}` }),
        capabilities: create(NetConnRequest_CapabilitiesSchema, {
          supportsOpportunisticTls: true,
          supportsFlowControl: true,
        }),
      }),
    },
  }));
  const respStream3 = client.netConn(reqStream3);
  const it3 = respStream3[Symbol.asyncIterator]();
  const conn3 = await it3.next();
  assert.strictEqual(conn3.value.data.case, 'conn');

  reqStream3.push(create(NetConnRequestSchema, {
    data: {
      case: 'control',
      value: create(NetConnRequest_ControlSchema, {
        action: {
          case: 'ping',
          value: create(NetConnRequest_Control_PingSchema, {
            id: 777n,
            timestampNs: 999999n,
          }),
        },
      }),
    },
  }));
  const pong3 = await it3.next();
  assert.strictEqual(pong3.value.data.case, 'control');
  assert.strictEqual(pong3.value.data.value.event.case, 'pong');
  assert.strictEqual(pong3.value.data.value.event.value.id, 777n);
  reqStream3.close();

  // Test 6: Fail-Closed Unsupported Preset under Node.js
  console.log('6. Testing Fail-Closed Preset rejection under Node.js...');
  const reqStream4 = new RequestStream();
  reqStream4.push(create(NetConnRequestSchema, {
    data: {
      case: 'dial',
      value: create(NetConnRequest_DialSchema, {
        address: create(NetAddrSchema, { network: 'tcp', address: `127.0.0.1:${tlsPort}` }),
        tls: create(TLSOptionsSchema, {
          serverName: 'localhost',
          fingerprintPreset: FingerprintPreset.CHROME_120,
          insecureSkipVerify: true,
        }),
      }),
    },
  }));
  const respStream4 = client.netConn(reqStream4);
  const it4 = respStream4[Symbol.asyncIterator]();
  try {
    await it4.next();
    assert.fail('should have failed closed');
  } catch (err) {
    assert.ok(err instanceof ConnectError);
    assert.strictEqual(err.code, Code.FailedPrecondition);
  }
  reqStream4.close();

  console.log('All Node.js runtime tests passed cleanly!');
  process.exit(0);
}

run().catch(err => {
  console.error('Test failure:', err);
  process.exit(1);
});
