import {describe, it, expect, beforeAll, afterAll} from 'bun:test';
import * as http2 from 'node:http2';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';
import {execFileSync, spawnSync} from 'node:child_process';
import {X509Certificate} from 'node:crypto';
import {parseConfig} from '../src/config';
import {createEndpointServer, type EndpointServer} from '../src/server';

const FIXTURES_DIR = fs.existsSync(path.join(__dirname, 'fixtures'))
  ? path.join(__dirname, 'fixtures')
  : path.join(__dirname, '../../test/fixtures');
const SERVER_CERT = path.join(FIXTURES_DIR, 'cert.pem');
const SERVER_KEY = path.join(FIXTURES_DIR, 'key.pem');
const CLI_PATH = path.join(__dirname, '..', 'src', 'cli.ts');

// Runs an openssl command as an argument vector. execFileSync never invokes a
// shell, so temp-directory paths cannot break out of the command line.
function runOpenssl(args: string[]): void {
  execFileSync('openssl', args, {stdio: 'pipe'});
}

// Writes explicit extension files for CA and client certificates so fixtures
// are well-formed across all OpenSSL and LibreSSL versions without relying
// on openssl.cnf defaults or implementation-specific flags like req -addext.
function writeCaExtensions(dir: string): string {
  const extPath = path.join(dir, 'ca.ext');
  fs.writeFileSync(
    extPath,
    [
      '[v3_ca]',
      'basicConstraints=critical,CA:TRUE',
      'keyUsage=critical,keyCertSign,cRLSign',
      '',
    ].join('\n'),
  );
  return extPath;
}

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

// Generates a CA and a client certificate signed by it using portable extfiles.
function generateClientPair(
  dir: string,
  prefix: string,
  caCn: string,
  leafCn: string,
  caExtPath: string,
  leafExtPath: string,
): {caCertPath: string; certPath: string; keyPath: string} {
  const caCertPath = path.join(dir, `${prefix}-ca.crt`);
  const caKeyPath = path.join(dir, `${prefix}-ca.key`);
  const caCsrPath = path.join(dir, `${prefix}-ca.csr`);
  const certPath = path.join(dir, `${prefix}.crt`);
  const keyPath = path.join(dir, `${prefix}.key`);
  const csrPath = path.join(dir, `${prefix}.csr`);

  runOpenssl([
    'req',
    '-nodes',
    '-newkey',
    'rsa:2048',
    '-keyout',
    caKeyPath,
    '-out',
    caCsrPath,
    '-subj',
    `/CN=${caCn}`,
  ]);
  runOpenssl([
    'x509',
    '-req',
    '-in',
    caCsrPath,
    '-signkey',
    caKeyPath,
    '-out',
    caCertPath,
    '-days',
    '1',
    '-extfile',
    caExtPath,
    '-extensions',
    'v3_ca',
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
// connection was rejected. A 5-second fallback timeout prevents test hangs.
// The session is always destroyed in finally.
async function attemptExchange(
  host: string,
  port: number,
  options: http2.SecureClientSessionOptions,
  timeoutMs = 5000,
): Promise<boolean> {
  const client = http2.connect(`https://${host}:${port}`, options);
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await new Promise<boolean>(resolve => {
      timer = setTimeout(() => resolve(false), timeoutMs);
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
    const caExtPath = writeCaExtensions(tmpDir);
    const leafExtPath = writeLeafExtensions(tmpDir);

    const client = generateClientPair(
      tmpDir,
      'client',
      'Test Client CA',
      'test-client',
      caExtPath,
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
      caExtPath,
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
          SESAME_ENDPOINT_CLIENT_CA_CERT:
            '-----BEGIN CERTIFICATE-----\nMOCK\n-----END CERTIFICATE-----',
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
      // Every value is resolved inside the parser: the environment path
      // arrives as PEM content, never as a path for a later component.
      expect(secrets.tlsCert).toContain('BEGIN CERTIFICATE');
      expect(secrets.tlsKey).toBe(fs.readFileSync(SERVER_KEY, 'utf8'));
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

    it('resolves environment TLS file paths to PEM content at parse time', () => {
      const res = parseConfig([], {
        SESAME_ENDPOINT_TLS_CERT: SERVER_CERT,
        SESAME_ENDPOINT_TLS_KEY: SERVER_KEY,
        SESAME_ENDPOINT_CLIENT_CA_CERT: clientCaCertPath,
      });
      expect(res.config).toBeDefined();
      const secrets = res.config!.secrets;
      // No unresolved path may leave the parser: every secret arrives as PEM
      // content, ready for the server without further I/O.
      expect(secrets.tlsCert).toContain('BEGIN CERTIFICATE');
      expect(secrets.tlsKey).toContain('BEGIN PRIVATE KEY');
      expect(secrets.clientCaCert).toContain('BEGIN CERTIFICATE');
    });

    it('rejects blank TLS environment variables instead of treating them as unset', () => {
      expect(() => parseConfig([], {SESAME_ENDPOINT_TLS_CERT: ''})).toThrow(
        'Empty value for SESAME_ENDPOINT_TLS_CERT',
      );
      expect(() => parseConfig([], {SESAME_ENDPOINT_TLS_KEY: '   '})).toThrow(
        'Empty value for SESAME_ENDPOINT_TLS_KEY',
      );
      expect(() =>
        parseConfig([], {SESAME_ENDPOINT_CLIENT_CA_CERT: ''}),
      ).toThrow('Empty value for SESAME_ENDPOINT_CLIENT_CA_CERT');
    });

    it('lets CLI flags take precedence over environment TLS paths', () => {
      // The overridden environment paths do not exist: parsing succeeds only
      // because the flags win and the overridden values are never read.
      const res = parseConfig(
        [
          `--tls-cert=${fs.readFileSync(SERVER_CERT, 'utf8')}`,
          `--tls-key=${fs.readFileSync(SERVER_KEY, 'utf8')}`,
        ],
        {
          SESAME_ENDPOINT_TLS_CERT: path.join(tmpDir, 'unread-env-cert.pem'),
          SESAME_ENDPOINT_TLS_KEY: path.join(tmpDir, 'unread-env-key.pem'),
        },
      );
      expect(res.config).toBeDefined();
      expect(res.config!.secrets.tlsCert).toContain('BEGIN CERTIFICATE');
    });

    it('throws ENOENT at parse time when an environment TLS path does not exist', () => {
      expect(() =>
        parseConfig([], {
          SESAME_ENDPOINT_TLS_CERT: path.join(tmpDir, 'missing-cert.pem'),
          SESAME_ENDPOINT_TLS_KEY: path.join(tmpDir, 'missing-key.pem'),
        }),
      ).toThrow('ENOENT');
    });

    it('rejects an empty environment-provided TLS file at parse time', () => {
      const emptyCert = path.join(tmpDir, 'empty-env-file-cert.pem');
      const emptyKey = path.join(tmpDir, 'empty-env-file-key.pem');
      fs.writeFileSync(emptyCert, '');
      fs.writeFileSync(emptyKey, '');
      expect(() =>
        parseConfig([], {
          SESAME_ENDPOINT_TLS_CERT: emptyCert,
          SESAME_ENDPOINT_TLS_KEY: emptyKey,
        }),
      ).toThrow('Empty content for SESAME_ENDPOINT_TLS_CERT');
    });

    it('rejects a certificate file whose content is not PEM', () => {
      const garbageCert = path.join(tmpDir, 'not-pem-cert.pem');
      fs.writeFileSync(garbageCert, 'this is not a certificate\n');
      expect(() =>
        parseConfig([`--tls-cert=${garbageCert}`, `--tls-key=${SERVER_KEY}`]),
      ).toThrow('Invalid PEM content for --tls-cert');
    });

    it('fails closed on multi-line non-PEM input without leaking secret in ENOENT', () => {
      const sensitiveSecret =
        'SECRET_KEY_HEADERLESS_LINE_ONE\nSECRET_KEY_HEADERLESS_LINE_TWO';
      let caughtErr: Error | undefined;
      try {
        parseConfig([
          `--tls-cert=${sensitiveSecret}`,
          `--tls-key=${SERVER_KEY}`,
        ]);
      } catch (err) {
        caughtErr = err as Error;
      }
      expect(caughtErr).toBeDefined();
      expect(caughtErr!.message).toContain(
        'Invalid PEM content for --tls-cert',
      );
      expect(caughtErr!.message).not.toContain('ENOENT');
      expect(caughtErr!.message).not.toContain(sensitiveSecret);
    });

    it('honors --help and --version even when preceding TLS flags point to a non-existent file', () => {
      const helpRes = parseConfig([
        '--tls-cert=/path/does/not/exist/cert.pem',
        '--help',
      ]);
      expect(helpRes.helpRequested).toBe(true);

      const versionRes = parseConfig([
        '--tls-cert=/path/does/not/exist/cert.pem',
        '-v',
      ]);
      expect(versionRes.versionRequested).toBe(true);
    });

    it('supports CLI flag overriding without failing on earlier invalid/missing files', () => {
      const res = parseConfig([
        '--tls-cert=/missing/initial.pem',
        `--tls-cert=${SERVER_CERT}`,
        `--tls-key=${SERVER_KEY}`,
      ]);
      expect(res.config).toBeDefined();
      expect(res.config!.secrets.tlsCert).toContain('BEGIN CERTIFICATE');
    });

    it('rejects TLS flags with no value when followed by another flag', () => {
      expect(() =>
        parseConfig(['--tls-cert', `--tls-key=${SERVER_KEY}`]),
      ).toThrow('Missing value for --tls-cert');
      expect(() =>
        parseConfig(['--tls-key', `--tls-cert=${SERVER_CERT}`]),
      ).toThrow('Missing value for --tls-key');
    });

    it('reads certificate files whose filename contains "-----BEGIN"', () => {
      const pathWithBegin = path.join(tmpDir, 'custom-----BEGIN-fixture.pem');
      fs.copyFileSync(SERVER_CERT, pathWithBegin);
      const res = parseConfig([
        `--tls-cert=${pathWithBegin}`,
        `--tls-key=${SERVER_KEY}`,
      ]);
      expect(res.config).toBeDefined();
      expect(res.config!.secrets.tlsCert).toContain('BEGIN CERTIFICATE');
    });

    it('eagerly resolves upstream secrets and rejects blank values and partial pairs', () => {
      // Blank values fail closed
      expect(() => parseConfig([], {SESAME_ENDPOINT_CA_CERT: ''})).toThrow(
        'Empty value for SESAME_ENDPOINT_CA_CERT',
      );
      expect(() =>
        parseConfig([], {SESAME_ENDPOINT_CLIENT_CERT: '   '}),
      ).toThrow('Empty value for SESAME_ENDPOINT_CLIENT_CERT');
      expect(() => parseConfig([], {SESAME_ENDPOINT_CLIENT_KEY: ''})).toThrow(
        'Empty value for SESAME_ENDPOINT_CLIENT_KEY',
      );

      // File paths resolved to PEM content
      const res = parseConfig([], {
        SESAME_ENDPOINT_CA_CERT: clientCaCertPath,
        SESAME_ENDPOINT_CLIENT_CERT: clientCertPath,
        SESAME_ENDPOINT_CLIENT_KEY: clientKeyPath,
      });
      expect(res.config).toBeDefined();
      expect(res.config!.secrets.caCert).toContain('BEGIN CERTIFICATE');
      expect(res.config!.secrets.clientCert).toContain('BEGIN CERTIFICATE');
      expect(res.config!.secrets.clientKey).toContain('BEGIN PRIVATE KEY');

      // Pair requirement: clientCert requires clientKey and vice versa
      const bothClientHalves =
        'Both client certificate (SESAME_ENDPOINT_CLIENT_CERT) and client key (SESAME_ENDPOINT_CLIENT_KEY) must be provided together';
      expect(() =>
        parseConfig([], {SESAME_ENDPOINT_CLIENT_CERT: clientCertPath}),
      ).toThrow(bothClientHalves);
      expect(() =>
        parseConfig([], {SESAME_ENDPOINT_CLIENT_KEY: clientKeyPath}),
      ).toThrow(bothClientHalves);
    });
  });

  describe('createEndpointServer fail-closed validation', () => {
    let validConfig: ReturnType<typeof parseConfig>['config'];
    let emptyCaPath: string;

    beforeAll(() => {
      emptyCaPath = path.join(tmpDir, 'empty-env-ca.pem');
      fs.writeFileSync(emptyCaPath, '');

      validConfig = parseConfig([
        `--tls-cert=${fs.readFileSync(SERVER_CERT, 'utf8')}`,
        `--tls-key=${fs.readFileSync(SERVER_KEY, 'utf8')}`,
        `--client-ca-cert=${fs.readFileSync(clientCaCertPath, 'utf8')}`,
      ]).config;
    });

    it('throws ENOENT when a hand-built config points at a missing TLS path', () => {
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {
            ...validConfig!.secrets,
            tlsCert: path.join(tmpDir, 'missing-hand-built-cert.pem'),
            tlsKey: path.join(tmpDir, 'missing-hand-built-key.pem'),
          },
        }),
      ).toThrow('ENOENT');
    });

    it('throws when a hand-built config points at a non-PEM file', () => {
      const garbageCert = path.join(tmpDir, 'not-pem-hand-built.pem');
      fs.writeFileSync(garbageCert, 'still not a certificate\n');
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {...validConfig!.secrets, tlsCert: garbageCert},
        }),
      ).toThrow('Invalid PEM content for TLS certificate');
    });

    it('throws on a hand-built config with partial TLS material', () => {
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {
            ...validConfig!.secrets,
            clientCaCert: undefined,
            tlsKey: undefined,
          },
        }),
      ).toThrow('Both tlsCert and tlsKey must be provided');
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {
            ...validConfig!.secrets,
            clientCaCert: undefined,
            tlsCert: undefined,
          },
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
          secrets: {
            clientCaCert:
              '-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----',
          },
        }),
      ).toThrow('clientCaCert requires both tlsCert and tlsKey');
    });

    it('matches config.ts check order: clientCaCert check precedes partial TLS check', () => {
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {
            ...validConfig!.secrets,
            tlsKey: undefined,
            clientCaCert:
              '-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----',
          },
        }),
      ).toThrow('clientCaCert requires both tlsCert and tlsKey');
    });

    it('throws synchronously inside createEndpointServer on malformed PEM under Bun and Node', () => {
      expect(() =>
        createEndpointServer({
          ...validConfig!,
          secrets: {
            ...validConfig!.secrets,
            tlsCert:
              '-----BEGIN CERTIFICATE-----\nnot-base64\n-----END CERTIFICATE-----\n',
            tlsKey:
              '-----BEGIN PRIVATE KEY-----\nnot-base64\n-----END PRIVATE KEY-----\n',
          },
        }),
      ).toThrow();
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

  describe('TLS listener without client CA (standard TLS)', () => {
    let endpointServer: EndpointServer;
    let serverHost: string;
    let serverPort: number;

    beforeAll(async () => {
      const res = parseConfig([
        '--host=127.0.0.1',
        `--tls-cert=${SERVER_CERT}`,
        `--tls-key=${SERVER_KEY}`,
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

    it('accepts a client connecting without a client certificate', async () => {
      const accepted = await attemptExchange(serverHost, serverPort, {
        ca: fs.readFileSync(SERVER_CERT),
        rejectUnauthorized: false,
      });
      expect(accepted).toBe(true);
    });
  });

  describe('CLI startup error reporting', () => {
    const cleanEnv = Object.fromEntries(
      Object.entries(process.env).filter(
        ([k]) => !k.startsWith('SESAME_ENDPOINT_'),
      ),
    );

    it('reports an unresolvable environment TLS path as a friendly error', () => {
      const res = spawnSync(process.execPath, [CLI_PATH], {
        env: {
          ...cleanEnv,
          SESAME_ENDPOINT_TLS_CERT: path.join(tmpDir, 'cli-missing-cert.pem'),
          SESAME_ENDPOINT_TLS_KEY: path.join(tmpDir, 'cli-missing-key.pem'),
        },
        encoding: 'utf8',
        timeout: 30000,
      });
      expect(res.status).toBe(1);
      // Friendly one-liner with the parse error; not a raw stack trace.
      expect(res.stderr.startsWith('sesame-endpoint: ')).toBe(true);
      expect(res.stderr).toContain('ENOENT');
      expect(res.stderr).not.toContain('\n    at ');
    });

    it('reports malformed TLS material as a friendly error, not a raw stack', () => {
      const badCert = path.join(tmpDir, 'cli-bad-cert.pem');
      const badKey = path.join(tmpDir, 'cli-bad-key.pem');
      fs.writeFileSync(
        badCert,
        '-----BEGIN CERTIFICATE-----\nnot-base64\n-----END CERTIFICATE-----\n',
      );
      fs.writeFileSync(
        badKey,
        '-----BEGIN PRIVATE KEY-----\nnot-base64\n-----END PRIVATE KEY-----\n',
      );
      const res = spawnSync(process.execPath, [CLI_PATH], {
        env: {
          ...cleanEnv,
          SESAME_ENDPOINT_TLS_CERT: badCert,
          SESAME_ENDPOINT_TLS_KEY: badKey,
        },
        encoding: 'utf8',
        timeout: 30000,
      });
      expect(res.status).toBe(1);
      // Both Node and Bun fail synchronously inside createEndpointServer
      // and surface the friendly one-line error without a raw stack trace.
      expect(res.stderr.startsWith('sesame-endpoint: ')).toBe(true);
      expect(res.stderr).not.toContain('sesame-endpoint failed to start:');
      expect(res.stderr).not.toContain('\n    at ');
    });
  });
});
