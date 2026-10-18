import {describe, it, expect, beforeAll, afterAll} from 'bun:test';
import * as http2 from 'node:http2';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';
import {execFileSync} from 'node:child_process';
import {X509Certificate} from 'node:crypto';
import {parseConfig} from '../src/config';
import {createEndpointServer, EndpointServer} from '../src/server';

const FIXTURES_DIR = fs.existsSync(path.join(__dirname, 'fixtures'))
  ? path.join(__dirname, 'fixtures')
  : path.join(__dirname, '../../test/fixtures');
const SERVER_CERT = path.join(FIXTURES_DIR, 'cert.pem');
const SERVER_KEY = path.join(FIXTURES_DIR, 'key.pem');

// Runs an openssl command as an argument vector. execFileSync never invokes a
// shell, so temp-directory paths cannot break out of the command line.
function runOpenssl(args: string[]): void {
  execFileSync('openssl', args, {stdio: 'pipe'});
}

// Writes the explicit extension file for client certificates. Plain
// `req -x509` emits an X.509 v1 CA with no extensions under LibreSSL, and
// `x509 -req` without -extfile emits an extension-less leaf (no clientAuth
// EKU) under both LibreSSL and OpenSSL, so the fixtures must not rely on
// host openssl defaults.
function writeLeafExtensions(dir: string): string {
  const extPath = path.join(dir, 'leaf.ext');
  fs.writeFileSync(
    extPath,
    [
      '[v3_leaf]',
      'basicConstraints=critical,CA:FALSE',
      'keyUsage=critical,digitalSignature',
      'extendedKeyUsage=clientAuth',
      '',
    ].join('\n'),
  );
  return extPath;
}

// Generates a CA and a client certificate signed by it. Both carry explicit
// X.509 v3 extensions (CA:TRUE on the CA, clientAuth EKU on the leaf) so the
// trust chain is well-formed on every openssl implementation.
function generateClientPair(
  dir: string,
  prefix: string,
  caCn: string,
  leafCn: string,
  leafExtPath: string,
): {caCertPath: string; certPath: string; keyPath: string} {
  const caCertPath = path.join(dir, `${prefix}-ca.crt`);
  const caKeyPath = path.join(dir, `${prefix}-ca.key`);
  const certPath = path.join(dir, `${prefix}.crt`);
  const keyPath = path.join(dir, `${prefix}.key`);
  const csrPath = path.join(dir, `${prefix}.csr`);

  runOpenssl([
    'req',
    '-x509',
    '-nodes',
    '-newkey',
    'rsa:2048',
    '-days',
    '1',
    '-keyout',
    caKeyPath,
    '-out',
    caCertPath,
    '-subj',
    `/CN=${caCn}`,
    '-addext',
    'basicConstraints=critical,CA:TRUE',
    '-addext',
    'keyUsage=critical,keyCertSign,cRLSign',
  ]);
  runOpenssl([
    'req',
    '-nodes',
    '-newkey',
    'rsa:2048',
    '-keyout',
    keyPath,
    '-out',
    csrPath,
    '-subj',
    `/CN=${leafCn}`,
  ]);
  runOpenssl([
    'x509',
    '-req',
    '-in',
    csrPath,
    '-CA',
    caCertPath,
    '-CAkey',
    caKeyPath,
    '-CAcreateserial',
    '-out',
    certPath,
    '-days',
    '1',
    '-extfile',
    leafExtPath,
    '-extensions',
    'v3_leaf',
  ]);

  return {caCertPath, certPath, keyPath};
}

// Attempts an HTTP/2 exchange with the given TLS client options. Resolves
// true when the server accepted the client and answered, false when the
// connection was rejected. The session is always destroyed in finally, so a
// rejected attempt cannot leak and settlement never races an explicit close.
async function attemptExchange(
  host: string,
  port: number,
  options: http2.SecureClientSessionOptions,
): Promise<boolean> {
  const client = http2.connect(`https://${host}:${port}`, options);
  try {
    return await new Promise<boolean>(resolve => {
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
    client.destroy();
  }
}

describe('sesame-endpoint mTLS configuration and server verification', () => {
  let tmpDir: string;
  let clientCaCertPath: string;
  let clientCertPath: string;
  let clientKeyPath: string;
  let untrustedCaPath: string;
  let untrustedCertPath: string;
  let untrustedKeyPath: string;

  beforeAll(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sesame-mtls-test-'));
    const leafExtPath = writeLeafExtensions(tmpDir);

    const client = generateClientPair(
      tmpDir,
      'client',
      'Test Client CA',
      'test-client',
      leafExtPath,
    );
    clientCaCertPath = client.caCertPath;
    clientCertPath = client.certPath;
    clientKeyPath = client.keyPath;

    const untrusted = generateClientPair(
      tmpDir,
      'untrusted',
      'Untrusted CA',
      'untrusted-client',
      leafExtPath,
    );
    untrustedCaPath = untrusted.caCertPath;
    untrustedCertPath = untrusted.certPath;
    untrustedKeyPath = untrusted.keyPath;
  });

  afterAll(() => {
    if (tmpDir && fs.existsSync(tmpDir)) {
      fs.rmSync(tmpDir, {recursive: true, force: true});
    }
  });

  describe('generated certificate fixtures', () => {
    it('produces CAs with CA:TRUE basic constraints', () => {
      expect(new X509Certificate(fs.readFileSync(clientCaCertPath)).ca).toBe(
        true,
      );
      expect(new X509Certificate(fs.readFileSync(untrustedCaPath)).ca).toBe(
        true,
      );
    });

    it('produces client certificates carrying clientAuth extended key usage', () => {
      // 1.3.6.1.5.5.7.3.2 is the id-kp-clientAuth OID. Bun surfaces extended
      // key usage OIDs through X509Certificate.keyUsage; an extension-less
      // leaf (the old recipe) reports undefined, so this pin fails if the
      // explicit -extfile extensions are ever dropped.
      const cert = new X509Certificate(fs.readFileSync(clientCertPath));
      expect(cert.keyUsage).toContain('1.3.6.1.5.5.7.3.2');
    });
  });

  describe('parseConfig mTLS flags and environment variables', () => {
    it('parses --tls-cert, --tls-key, and --client-ca-cert from CLI arguments', () => {
      const res = parseConfig([
        `--tls-cert=${SERVER_CERT}`,
        `--tls-key=${SERVER_KEY}`,
        `--client-ca-cert=${clientCaCertPath}`,
      ]);
      expect(res.config).toBeDefined();
      const secrets = res.config!.secrets;
      expect(secrets.tlsCert).toContain('BEGIN CERTIFICATE');
      expect(secrets.tlsKey).toContain('BEGIN PRIVATE KEY');
      expect(secrets.clientCaCert).toContain('BEGIN CERTIFICATE');
    });

    it('parses space-separated CLI flags', () => {
      const res = parseConfig([
        '--tls-cert',
        SERVER_CERT,
        '--tls-key',
        SERVER_KEY,
        '--client-ca-cert',
        clientCaCertPath,
      ]);
      expect(res.config).toBeDefined();
      const secrets = res.config!.secrets;
      expect(secrets.tlsCert).toContain('BEGIN CERTIFICATE');
      expect(secrets.tlsKey).toContain('BEGIN PRIVATE KEY');
      expect(secrets.clientCaCert).toContain('BEGIN CERTIFICATE');
    });

    it('parses SESAME_ENDPOINT_CLIENT_CA_CERT from environment', () => {
      const caContent = fs.readFileSync(clientCaCertPath, 'utf8');
      const res = parseConfig([], {
        SESAME_ENDPOINT_TLS_CERT: fs.readFileSync(SERVER_CERT, 'utf8'),
        SESAME_ENDPOINT_TLS_KEY: fs.readFileSync(SERVER_KEY, 'utf8'),
        SESAME_ENDPOINT_CLIENT_CA_CERT: caContent,
      });
      expect(res.config).toBeDefined();
      expect(res.config!.secrets.clientCaCert).toBe(caContent);
    });

    it('rejects clientCaCert if tlsCert or tlsKey is missing', () => {
      expect(() =>
        parseConfig([`--client-ca-cert=${clientCaCertPath}`]),
      ).toThrow('Client CA certificate');

      expect(() =>
        parseConfig([
          `--tls-cert=${SERVER_CERT}`,
          `--client-ca-cert=${clientCaCertPath}`,
        ]),
      ).toThrow('Client CA certificate');

      expect(() =>
        parseConfig([], {
          SESAME_ENDPOINT_CLIENT_CA_CERT: 'mock-ca',
        }),
      ).toThrow('Client CA certificate');
    });

    it('throws when CLI flag points to a non-existent file', () => {
      expect(() =>
        parseConfig(['--tls-cert=/path/does/not/exist/cert.pem']),
      ).toThrow();
    });

    it('rejects blank TLS flag values', () => {
      expect(() => parseConfig(['--tls-cert='])).toThrow(
        'Empty value for --tls-cert',
      );
      expect(() => parseConfig(['--tls-key='])).toThrow(
        'Empty value for --tls-key',
      );
      expect(() => parseConfig(['--client-ca-cert='])).toThrow(
        'Empty value for --client-ca-cert',
      );
    });

    it('rejects TLS flags with no value', () => {
      expect(() => parseConfig(['--tls-cert'])).toThrow(
        'Missing value for --tls-cert',
      );
      expect(() => parseConfig(['--tls-key'])).toThrow(
        'Missing value for --tls-key',
      );
      expect(() => parseConfig(['--client-ca-cert'])).toThrow(
        'Missing value for --client-ca-cert',
      );
    });

    it('rejects empty certificate files instead of silently disabling TLS', () => {
      const emptyCert = path.join(tmpDir, 'empty-cert.pem');
      const emptyKey = path.join(tmpDir, 'empty-key.pem');
      const emptyCa = path.join(tmpDir, 'empty-ca.pem');
      fs.writeFileSync(emptyCert, '');
      fs.writeFileSync(emptyKey, '');
      fs.writeFileSync(emptyCa, '');

      expect(() =>
        parseConfig([
          `--tls-cert=${emptyCert}`,
          `--tls-key=${emptyKey}`,
          `--client-ca-cert=${emptyCa}`,
        ]),
      ).toThrow('Empty content for --tls-cert');
      expect(() =>
        parseConfig([
          `--tls-cert=${SERVER_CERT}`,
          `--tls-key=${emptyKey}`,
          `--client-ca-cert=${emptyCa}`,
        ]),
      ).toThrow('Empty content for --tls-key');
      expect(() =>
        parseConfig([
          `--tls-cert=${SERVER_CERT}`,
          `--tls-key=${SERVER_KEY}`,
          `--client-ca-cert=${emptyCa}`,
        ]),
      ).toThrow('Empty content for --client-ca-cert');
    });

    it('rejects whitespace-only certificate files', () => {
      const whitespaceCert = path.join(tmpDir, 'whitespace-cert.pem');
      fs.writeFileSync(whitespaceCert, '   \n');
      expect(() =>
        parseConfig([
          `--tls-cert=${whitespaceCert}`,
          `--tls-key=${SERVER_KEY}`,
        ]),
      ).toThrow('Empty content for --tls-cert');
    });

    it('rejects TLS certificate without key and vice versa', () => {
      const bothHalves =
        'Both TLS certificate (--tls-cert or SESAME_ENDPOINT_TLS_CERT) and TLS key (--tls-key or SESAME_ENDPOINT_TLS_KEY) must be provided together';
      expect(() => parseConfig([`--tls-cert=${SERVER_CERT}`])).toThrow(
        bothHalves,
      );
      expect(() => parseConfig([`--tls-key=${SERVER_KEY}`])).toThrow(
        bothHalves,
      );
      expect(() =>
        parseConfig([], {SESAME_ENDPOINT_TLS_CERT: SERVER_CERT}),
      ).toThrow(bothHalves);
      expect(() =>
        parseConfig([], {SESAME_ENDPOINT_TLS_KEY: SERVER_KEY}),
      ).toThrow(bothHalves);
    });

    it('accepts TLS material split across CLI and environment', () => {
      const caContent = fs.readFileSync(clientCaCertPath, 'utf8');
      const res = parseConfig([`--tls-cert=${SERVER_CERT}`], {
        SESAME_ENDPOINT_TLS_KEY: SERVER_KEY,
        SESAME_ENDPOINT_CLIENT_CA_CERT: caContent,
      });
      expect(res.config).toBeDefined();
      const secrets = res.config!.secrets;
      // CLI values are read eagerly; environment values stay as provided and
      // are resolved by the server, so an environment path stays a path here.
      expect(secrets.tlsCert).toContain('BEGIN CERTIFICATE');
      expect(secrets.tlsKey).toBe(SERVER_KEY);
      expect(secrets.clientCaCert).toBe(caContent);
    });

    it('accepts literal PEM content for TLS flags', () => {
      const certContent = fs.readFileSync(SERVER_CERT, 'utf8');
      const res = parseConfig([
        `--tls-cert=${certContent}`,
        `--tls-key=${fs.readFileSync(SERVER_KEY, 'utf8')}`,
        `--client-ca-cert=${fs.readFileSync(clientCaCertPath, 'utf8')}`,
      ]);
      expect(res.config).toBeDefined();
      expect(res.config!.secrets.tlsCert).toBe(certContent);
    });

    it('treats empty TLS environment variables as unset', () => {
      const res = parseConfig([], {
        SESAME_ENDPOINT_TLS_CERT: '',
        SESAME_ENDPOINT_TLS_KEY: '   ',
        SESAME_ENDPOINT_CLIENT_CA_CERT: '',
      });
      expect(res.config).toBeDefined();
      expect(res.config!.secrets.tlsCert).toBeUndefined();
      expect(res.config!.secrets.tlsKey).toBeUndefined();
      expect(res.config!.secrets.clientCaCert).toBeUndefined();
    });

    it('fails closed on multi-line non-PEM input', () => {
      expect(() =>
        parseConfig([
          '--tls-cert=line one\nline two',
          `--tls-key=${SERVER_KEY}`,
        ]),
      ).toThrow('ENOENT');
    });
  });

  describe('createEndpointServer fail-closed validation', () => {
    let validConfig: ReturnType<typeof parseConfig>['config'];
    let emptyCertPath: string;
    let emptyKeyPath: string;
    let emptyCaPath: string;

    beforeAll(() => {
      emptyCertPath = path.join(tmpDir, 'empty-env-cert.pem');
      emptyKeyPath = path.join(tmpDir, 'empty-env-key.pem');
      emptyCaPath = path.join(tmpDir, 'empty-env-ca.pem');
      fs.writeFileSync(emptyCertPath, '');
      fs.writeFileSync(emptyKeyPath, '');
      fs.writeFileSync(emptyCaPath, '');

      validConfig = parseConfig([
        `--tls-cert=${fs.readFileSync(SERVER_CERT, 'utf8')}`,
        `--tls-key=${fs.readFileSync(SERVER_KEY, 'utf8')}`,
        `--client-ca-cert=${fs.readFileSync(clientCaCertPath, 'utf8')}`,
      ]).config;
    });

    it('throws ENOENT when an environment-provided TLS path does not exist', () => {
      const res = parseConfig([], {
        SESAME_ENDPOINT_TLS_CERT: path.join(tmpDir, 'missing-cert.pem'),
        SESAME_ENDPOINT_TLS_KEY: path.join(tmpDir, 'missing-key.pem'),
      });
      expect(() => createEndpointServer(res.config!)).toThrow('ENOENT');
    });

    it('throws when an environment-provided TLS file is empty', () => {
      const res = parseConfig([], {
        SESAME_ENDPOINT_TLS_CERT: emptyCertPath,
        SESAME_ENDPOINT_TLS_KEY: emptyKeyPath,
      });
      expect(() => createEndpointServer(res.config!)).toThrow(
        'TLS certificate is empty',
      );
    });

    it('throws on a hand-built config with partial TLS material', () => {
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {...validConfig!.secrets, tlsKey: undefined},
        }),
      ).toThrow('Both tlsCert and tlsKey must be provided');
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {...validConfig!.secrets, tlsCert: undefined},
        }),
      ).toThrow('Both tlsCert and tlsKey must be provided');
      // A defined-but-empty half is not "partial" - it reaches the content
      // check and fails there with a precise message.
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {...validConfig!.secrets, tlsCert: ''},
        }),
      ).toThrow('TLS certificate must not be empty');
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {...validConfig!.secrets, tlsKey: ''},
        }),
      ).toThrow('TLS key must not be empty');
    });

    it('throws when TLS material is defined but empty', () => {
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {...validConfig!.secrets, tlsCert: '', tlsKey: ''},
        }),
      ).toThrow('TLS certificate must not be empty');
    });

    it('throws when clientCaCert is configured without TLS material', () => {
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {clientCaCert: '-----BEGIN CERTIFICATE-----'},
        }),
      ).toThrow('clientCaCert requires both tlsCert and tlsKey');
    });

    it('throws when the client CA file is empty', () => {
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {...validConfig!.secrets, clientCaCert: emptyCaPath},
        }),
      ).toThrow('Client CA certificate is empty');
    });
  });

  describe('mTLS server handshake enforcement', () => {
    let endpointServer: EndpointServer;
    let serverHost: string;
    let serverPort: number;

    beforeAll(async () => {
      const res = parseConfig([
        '--host=127.0.0.1',
        `--tls-cert=${SERVER_CERT}`,
        `--tls-key=${SERVER_KEY}`,
        `--client-ca-cert=${clientCaCertPath}`,
      ]);
      endpointServer = createEndpointServer(res.config!);
      const bound = await endpointServer.listen(0, '127.0.0.1');
      serverHost = bound.host;
      serverPort = bound.port;
    });

    afterAll(async () => {
      if (endpointServer) {
        await endpointServer.close();
      }
    });

    it('successfully connects and completes HTTP/2 exchange with valid client certificate', async () => {
      const accepted = await attemptExchange(serverHost, serverPort, {
        ca: fs.readFileSync(SERVER_CERT),
        cert: fs.readFileSync(clientCertPath),
        key: fs.readFileSync(clientKeyPath),
        rejectUnauthorized: false, // Self-signed server cert in fixture
      });
      expect(accepted).toBe(true);
    });

    it('rejects connection when no client certificate is provided', async () => {
      const rejected = await attemptExchange(serverHost, serverPort, {
        ca: fs.readFileSync(SERVER_CERT),
        rejectUnauthorized: false,
      });
      expect(rejected).toBe(false);
    });

    it('rejects connection when client certificate is signed by an untrusted CA', async () => {
      const rejected = await attemptExchange(serverHost, serverPort, {
        ca: fs.readFileSync(SERVER_CERT),
        cert: fs.readFileSync(untrustedCertPath),
        key: fs.readFileSync(untrustedKeyPath),
        rejectUnauthorized: false,
      });
      expect(rejected).toBe(false);
    });
  });

  describe('mTLS listener configured from environment file paths', () => {
    let endpointServer: EndpointServer;
    let serverHost: string;
    let serverPort: number;

    beforeAll(async () => {
      const res = parseConfig([], {
        SESAME_ENDPOINT_TLS_CERT: SERVER_CERT,
        SESAME_ENDPOINT_TLS_KEY: SERVER_KEY,
        SESAME_ENDPOINT_CLIENT_CA_CERT: clientCaCertPath,
      });
      endpointServer = createEndpointServer(res.config!);
      const bound = await endpointServer.listen(0, '127.0.0.1');
      serverHost = bound.host;
      serverPort = bound.port;
    });

    afterAll(async () => {
      if (endpointServer) {
        await endpointServer.close();
      }
    });

    it('accepts a trusted client certificate', async () => {
      const accepted = await attemptExchange(serverHost, serverPort, {
        ca: fs.readFileSync(SERVER_CERT),
        cert: fs.readFileSync(clientCertPath),
        key: fs.readFileSync(clientKeyPath),
        rejectUnauthorized: false,
      });
      expect(accepted).toBe(true);
    });

    it('rejects a client without a certificate', async () => {
      const rejected = await attemptExchange(serverHost, serverPort, {
        ca: fs.readFileSync(SERVER_CERT),
        rejectUnauthorized: false,
      });
      expect(rejected).toBe(false);
    });
  });
});
