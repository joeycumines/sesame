import {describe, expect, it} from 'bun:test';
import {parseConfig, formatHelp} from '../src/config';

describe('config parser', () => {
  it('returns default configuration when no args or env are given', () => {
    const res = parseConfig([]);
    expect(res.config).toBeDefined();
    const cfg = res.config!;
    expect(cfg.host).toBe('127.0.0.1');
    expect(cfg.port).toBe(50051);
    expect(cfg.maxChunkSize).toBe(32768);
    expect(cfg.initialWindowSize).toBe(65535);
    expect(cfg.readTimeoutMs).toBe(30000);
    expect(cfg.dialTimeoutMs).toBe(10000);
    expect(cfg.enableOpportunisticTls).toBe(true);
    expect(cfg.enableFlowControl).toBe(true);
    expect(cfg.allowedNetworks).toEqual(['tcp', 'tcp4', 'tcp6']);
    expect(cfg.secrets.tlsCert).toBeUndefined();
    expect(cfg.secrets.proxyPassword).toBeUndefined();
  });

  it('parses CLI arguments with space and equals delimiters', () => {
    const res = parseConfig([
      '--host',
      '0.0.0.0',
      '--port=9090',
      '--max-chunk-size',
      '16384',
      '--initial-window-size=131072',
      '--read-timeout-ms',
      '15000',
      '--dial-timeout-ms=5000',
      '--enable-opportunistic-tls=false',
      '--enable-flow-control=false',
      '--allowed-networks=tcp,unix',
    ]);
    expect(res.config).toBeDefined();
    const cfg = res.config!;
    expect(cfg.host).toBe('0.0.0.0');
    expect(cfg.port).toBe(9090);
    expect(cfg.maxChunkSize).toBe(16384);
    expect(cfg.initialWindowSize).toBe(131072);
    expect(cfg.readTimeoutMs).toBe(15000);
    expect(cfg.dialTimeoutMs).toBe(5000);
    expect(cfg.enableOpportunisticTls).toBe(false);
    expect(cfg.enableFlowControl).toBe(false);
    expect(cfg.allowedNetworks).toEqual(['tcp', 'unix']);
    expect(cfg.initialWindowSize).toBe(131072);
  });

  it('parses secrets from scoped environment variables (SESAME_ENDPOINT_*)', () => {
    const env: NodeJS.ProcessEnv = {
      SESAME_ENDPOINT_HOST: '10.0.0.5',
      SESAME_ENDPOINT_PORT: '8443',
      SESAME_ENDPOINT_TLS_CERT:
        '-----BEGIN CERTIFICATE-----\nMOCK\n-----END CERTIFICATE-----',
      SESAME_ENDPOINT_TLS_KEY:
        '-----BEGIN PRIVATE KEY-----\nMOCK\n-----END PRIVATE KEY-----',
      SESAME_ENDPOINT_CA_CERT:
        '-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----',
      SESAME_ENDPOINT_PROXY_AUTH_TOKEN: 'secret-token-123',
      SESAME_ENDPOINT_PROXY_PASSWORD: 'super-secret-password',
    };
    const res = parseConfig([], env);
    expect(res.config).toBeDefined();
    const cfg = res.config!;
    expect(cfg.host).toBe('10.0.0.5');
    expect(cfg.port).toBe(8443);
    expect(cfg.secrets.tlsCert).toBe(env.SESAME_ENDPOINT_TLS_CERT);
    expect(cfg.secrets.tlsKey).toBe(env.SESAME_ENDPOINT_TLS_KEY);
    expect(cfg.secrets.caCert).toBe(env.SESAME_ENDPOINT_CA_CERT);
    expect(cfg.secrets.proxyAuthToken).toBe('secret-token-123');
    expect(cfg.secrets.proxyPassword).toBe('super-secret-password');
  });

  it('prioritizes CLI flags over environment variables', () => {
    const env: NodeJS.ProcessEnv = {
      SESAME_ENDPOINT_HOST: '10.0.0.1',
      SESAME_ENDPOINT_PORT: '8080',
    };
    const res = parseConfig(['--host', '192.168.1.1', '--port', '9999'], env);
    expect(res.config).toBeDefined();
    const cfg = res.config!;
    expect(cfg.host).toBe('192.168.1.1');
    expect(cfg.port).toBe(9999);
  });

  it('handles --help and --version flags', () => {
    expect(parseConfig(['--help']).helpRequested).toBe(true);
    expect(parseConfig(['-h']).helpRequested).toBe(true);
    expect(parseConfig(['--version']).versionRequested).toBe(true);
    expect(parseConfig(['-v']).versionRequested).toBe(true);
  });

  it('throws descriptive errors on invalid inputs', () => {
    expect(() => parseConfig(['--port', '0'])).toThrow('Invalid port');
    expect(() => parseConfig(['--port', '70000'])).toThrow('Invalid port');
    expect(() => parseConfig(['--port', 'notanumber'])).toThrow('Invalid port');
    expect(() => parseConfig(['--max-chunk-size', '100'])).toThrow(
      'Invalid maxChunkSize',
    );
    expect(() => parseConfig(['--initial-window-size', '500'])).toThrow(
      'Invalid initialWindowSize',
    );
    expect(() => parseConfig(['--supported-presets', 'NOT_A_PRESET'])).toThrow(
      'Unknown CLI argument',
    );
    expect(() => parseConfig(['--supported-presets', 'CHROME_120'])).toThrow(
      'Unknown CLI argument',
    );
    expect(() => parseConfig(['--unknown-flag'])).toThrow(
      'Unknown CLI argument',
    );
  });

  it('caps maxChunkSize at the gRPC per-message receive limit', () => {
    // Advertised chunk sizes must not exceed the stream's gRPC
    // per-message receive limit (grpc-go default 4MiB) per the wire
    // contract; the parser enforces the interop-safe ceiling.
    expect(
      parseConfig(['--max-chunk-size', String(4 * 1024 * 1024)]).config!
        .maxChunkSize,
    ).toBe(4 * 1024 * 1024);
    expect(() => parseConfig(['--max-chunk-size', '4194305'])).toThrow(
      'Invalid maxChunkSize',
    );
    expect(() =>
      parseConfig(['--max-chunk-size', String(16 * 1024 * 1024)]),
    ).toThrow('Invalid maxChunkSize');
  });

  it('caps initialWindowSize at the gRPC per-message receive limit', () => {
    // Same wire-contract rule as max_chunk_size: advertised chunk and
    // window sizes must not exceed the stream's gRPC per-message
    // receive limit (grpc-go default 4MiB).
    expect(
      parseConfig(['--initial-window-size', String(4 * 1024 * 1024)]).config!
        .initialWindowSize,
    ).toBe(4 * 1024 * 1024);
    expect(() => parseConfig(['--initial-window-size', '4194305'])).toThrow(
      'Invalid initialWindowSize',
    );
  });

  it('parses enable flags strictly: only true/false/1/0 are accepted', () => {
    // Accepted spellings, flag form.
    expect(
      parseConfig(['--enable-flow-control', 'false']).config!.enableFlowControl,
    ).toBe(false);
    expect(
      parseConfig(['--enable-flow-control', '0']).config!.enableFlowControl,
    ).toBe(false);
    expect(
      parseConfig(['--enable-flow-control', 'true']).config!.enableFlowControl,
    ).toBe(true);
    expect(
      parseConfig(['--enable-flow-control', '1']).config!.enableFlowControl,
    ).toBe(true);
    expect(
      parseConfig(['--enable-flow-control=TRUE']).config!.enableFlowControl,
    ).toBe(true);
    expect(
      parseConfig(['--enable-opportunistic-tls', 'false']).config!
        .enableOpportunisticTls,
    ).toBe(false);

    // Accepted spellings, env form. Critically, '0' must DISABLE - the
    // old parser enabled on anything that was not the literal 'false'.
    expect(
      parseConfig([], {SESAME_ENDPOINT_ENABLE_FLOW_CONTROL: '0'}).config!
        .enableFlowControl,
    ).toBe(false);
    expect(
      parseConfig([], {SESAME_ENDPOINT_ENABLE_FLOW_CONTROL: 'false'}).config!
        .enableFlowControl,
    ).toBe(false);
    expect(
      parseConfig([], {SESAME_ENDPOINT_ENABLE_FLOW_CONTROL: '1'}).config!
        .enableFlowControl,
    ).toBe(true);

    // Rejected spellings: previously these silently ENABLED.
    for (const bad of ['no', 'off', '2', 'yes ']) {
      expect(() =>
        parseConfig([], {SESAME_ENDPOINT_ENABLE_FLOW_CONTROL: bad}),
      ).toThrow(/Invalid boolean/);
      expect(() => parseConfig(['--enable-flow-control', bad])).toThrow(
        /Invalid boolean/,
      );
      expect(() =>
        parseConfig([], {SESAME_ENDPOINT_ENABLE_OPPORTUNISTIC_TLS: bad}),
      ).toThrow(/Invalid boolean/);
    }
  });

  it('defaults enable flags to true when env vars are absent', () => {
    expect(parseConfig([]).config!.enableFlowControl).toBe(true);
    expect(parseConfig([]).config!.enableOpportunisticTls).toBe(true);
  });

  it('formatHelp produces non-empty text mentioning scoped env vars', () => {
    const help = formatHelp();
    expect(help).toContain('sesame-endpoint [options]');
    expect(help).toContain('SESAME_ENDPOINT_TLS_CERT');
    expect(help).toContain('SESAME_ENDPOINT_PROXY_PASSWORD');
  });
});
