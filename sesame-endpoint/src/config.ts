import {FingerprintPreset} from './gen/sesame/tls/v1alpha1/tls_pb';

export interface ServerSecrets {
  readonly tlsCert?: string;
  readonly tlsKey?: string;
  readonly caCert?: string;
  readonly clientCert?: string;
  readonly clientKey?: string;
  readonly proxyAuthToken?: string;
  readonly proxyPassword?: string;
}

export interface ServerConfig {
  readonly host: string;
  readonly port: number;
  readonly maxChunkSize: number;
  readonly initialWindowSize: number;
  readonly readTimeoutMs: number;
  readonly dialTimeoutMs: number;
  readonly supportedPresets: readonly FingerprintPreset[];
  readonly enableOpportunisticTls: boolean;
  readonly enableFlowControl: boolean;
  readonly allowedNetworks: readonly string[];
  readonly secrets: ServerSecrets;
}

export interface ParseConfigResult {
  readonly config?: ServerConfig;
  readonly helpRequested?: boolean;
  readonly versionRequested?: boolean;
}

const DEFAULT_HOST = '127.0.0.1';
const DEFAULT_PORT = 50051;
const DEFAULT_MAX_CHUNK_SIZE = 32 * 1024; // 32 KB
const DEFAULT_INITIAL_WINDOW_SIZE = 65535; // 64 KB - 1 (default)
const DEFAULT_READ_TIMEOUT_MS = 30000;
const DEFAULT_DIAL_TIMEOUT_MS = 10000;
const DEFAULT_ALLOWED_NETWORKS = ['tcp', 'tcp4', 'tcp6'];

// Parses a boolean strictly: only true/false/1/0 (case-insensitive) are
// accepted. Security-relevant flags must not silently enable on values
// like 'no', 'off', or '2'.
function parseStrictBoolean(val: string, flagName: string): boolean {
  const normalized = val.trim().toLowerCase();
  switch (normalized) {
    case 'true':
    case '1':
      return true;
    case 'false':
    case '0':
      return false;
    default:
      throw new Error(
        `Invalid boolean for ${flagName}: "${val}". Must be true, false, 1, or 0`,
      );
  }
}

const PRESET_MAP: Record<string, FingerprintPreset> = {
  RUNTIME_DEFAULT: FingerprintPreset.RUNTIME_DEFAULT,
  CHROME_AUTO: FingerprintPreset.CHROME_AUTO,
  CHROME_120: FingerprintPreset.CHROME_120,
  CHROME_124: FingerprintPreset.CHROME_124,
  CHROME_131: FingerprintPreset.CHROME_131,
  FIREFOX_AUTO: FingerprintPreset.FIREFOX_AUTO,
  FIREFOX_120: FingerprintPreset.FIREFOX_120,
  SAFARI_AUTO: FingerprintPreset.SAFARI_AUTO,
  SAFARI_16: FingerprintPreset.SAFARI_16,
  SAFARI_17: FingerprintPreset.SAFARI_17,
  IOS_AUTO: FingerprintPreset.IOS_AUTO,
  IOS_16: FingerprintPreset.IOS_16,
  ANDROID_OKHTTP: FingerprintPreset.ANDROID_OKHTTP,
};

export function parseConfig(
  argv: readonly string[],
  env: NodeJS.ProcessEnv = {},
): ParseConfigResult {
  let host = env.SESAME_ENDPOINT_HOST || DEFAULT_HOST;
  let port = env.SESAME_ENDPOINT_PORT
    ? parseInt(env.SESAME_ENDPOINT_PORT, 10)
    : DEFAULT_PORT;
  let maxChunkSize = env.SESAME_ENDPOINT_MAX_CHUNK_SIZE
    ? parseInt(env.SESAME_ENDPOINT_MAX_CHUNK_SIZE, 10)
    : DEFAULT_MAX_CHUNK_SIZE;
  let initialWindowSize = env.SESAME_ENDPOINT_INITIAL_WINDOW_SIZE
    ? parseInt(env.SESAME_ENDPOINT_INITIAL_WINDOW_SIZE, 10)
    : DEFAULT_INITIAL_WINDOW_SIZE;
  let readTimeoutMs = env.SESAME_ENDPOINT_READ_TIMEOUT_MS
    ? parseInt(env.SESAME_ENDPOINT_READ_TIMEOUT_MS, 10)
    : DEFAULT_READ_TIMEOUT_MS;
  let dialTimeoutMs = env.SESAME_ENDPOINT_DIAL_TIMEOUT_MS
    ? parseInt(env.SESAME_ENDPOINT_DIAL_TIMEOUT_MS, 10)
    : DEFAULT_DIAL_TIMEOUT_MS;
  let enableOpportunisticTls = env.SESAME_ENDPOINT_ENABLE_OPPORTUNISTIC_TLS
    ? parseStrictBoolean(
        env.SESAME_ENDPOINT_ENABLE_OPPORTUNISTIC_TLS,
        'SESAME_ENDPOINT_ENABLE_OPPORTUNISTIC_TLS',
      )
    : true;
  let enableFlowControl = env.SESAME_ENDPOINT_ENABLE_FLOW_CONTROL
    ? parseStrictBoolean(
        env.SESAME_ENDPOINT_ENABLE_FLOW_CONTROL,
        'SESAME_ENDPOINT_ENABLE_FLOW_CONTROL',
      )
    : true;
  let allowedNetworks: string[] = env.SESAME_ENDPOINT_ALLOWED_NETWORKS
    ? env.SESAME_ENDPOINT_ALLOWED_NETWORKS.split(',').map(s => s.trim())
    : [...DEFAULT_ALLOWED_NETWORKS];
  let supportedPresets: FingerprintPreset[] = [
    FingerprintPreset.RUNTIME_DEFAULT,
  ];

  if (env.SESAME_ENDPOINT_SUPPORTED_PRESETS) {
    supportedPresets = parsePresetList(env.SESAME_ENDPOINT_SUPPORTED_PRESETS);
  }

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === '--help' || arg === '-h') {
      return {helpRequested: true};
    }
    if (arg === '--version' || arg === '-v') {
      return {versionRequested: true};
    }
    if (arg === '--host') {
      if (i + 1 >= argv.length) throw new Error('Missing value for --host');
      host = argv[++i];
    } else if (arg.startsWith('--host=')) {
      host = arg.slice('--host='.length);
    } else if (arg === '--port') {
      if (i + 1 >= argv.length) throw new Error('Missing value for --port');
      port = parsePort(argv[++i]);
    } else if (arg.startsWith('--port=')) {
      port = parsePort(arg.slice('--port='.length));
    } else if (arg === '--max-chunk-size') {
      if (i + 1 >= argv.length)
        throw new Error('Missing value for --max-chunk-size');
      maxChunkSize = parseInt(argv[++i], 10);
    } else if (arg.startsWith('--max-chunk-size=')) {
      maxChunkSize = parseInt(arg.slice('--max-chunk-size='.length), 10);
    } else if (arg === '--initial-window-size') {
      if (i + 1 >= argv.length)
        throw new Error('Missing value for --initial-window-size');
      initialWindowSize = parseInt(argv[++i], 10);
    } else if (arg.startsWith('--initial-window-size=')) {
      initialWindowSize = parseInt(
        arg.slice('--initial-window-size='.length),
        10,
      );
    } else if (arg === '--read-timeout-ms') {
      if (i + 1 >= argv.length)
        throw new Error('Missing value for --read-timeout-ms');
      readTimeoutMs = parseInt(argv[++i], 10);
    } else if (arg.startsWith('--read-timeout-ms=')) {
      readTimeoutMs = parseInt(arg.slice('--read-timeout-ms='.length), 10);
    } else if (arg === '--dial-timeout-ms') {
      if (i + 1 >= argv.length)
        throw new Error('Missing value for --dial-timeout-ms');
      dialTimeoutMs = parseInt(argv[++i], 10);
    } else if (arg.startsWith('--dial-timeout-ms=')) {
      dialTimeoutMs = parseInt(arg.slice('--dial-timeout-ms='.length), 10);
    } else if (arg === '--supported-presets') {
      if (i + 1 >= argv.length)
        throw new Error('Missing value for --supported-presets');
      supportedPresets = parsePresetList(argv[++i]);
    } else if (arg.startsWith('--supported-presets=')) {
      supportedPresets = parsePresetList(
        arg.slice('--supported-presets='.length),
      );
    } else if (arg === '--enable-opportunistic-tls') {
      if (i + 1 >= argv.length)
        throw new Error('Missing value for --enable-opportunistic-tls');
      enableOpportunisticTls = parseStrictBoolean(
        argv[++i],
        '--enable-opportunistic-tls',
      );
    } else if (arg.startsWith('--enable-opportunistic-tls=')) {
      enableOpportunisticTls = parseStrictBoolean(
        arg.slice('--enable-opportunistic-tls='.length),
        '--enable-opportunistic-tls',
      );
    } else if (arg === '--enable-flow-control') {
      if (i + 1 >= argv.length)
        throw new Error('Missing value for --enable-flow-control');
      enableFlowControl = parseStrictBoolean(
        argv[++i],
        '--enable-flow-control',
      );
    } else if (arg.startsWith('--enable-flow-control=')) {
      enableFlowControl = parseStrictBoolean(
        arg.slice('--enable-flow-control='.length),
        '--enable-flow-control',
      );
    } else if (arg === '--allowed-networks') {
      if (i + 1 >= argv.length)
        throw new Error('Missing value for --allowed-networks');
      allowedNetworks = argv[++i].split(',').map(s => s.trim());
    } else if (arg.startsWith('--allowed-networks=')) {
      allowedNetworks = arg
        .slice('--allowed-networks='.length)
        .split(',')
        .map(s => s.trim());
    } else {
      throw new Error(`Unknown CLI argument: ${arg}`);
    }
  }

  // Validate values
  if (!host || host.trim().length === 0) {
    throw new Error('Host cannot be empty');
  }
  if (isNaN(port) || port < 1 || port > 65535) {
    throw new Error(`Invalid port: ${port}. Must be between 1 and 65535`);
  }
  if (
    isNaN(maxChunkSize) ||
    maxChunkSize < 512 ||
    maxChunkSize > 16 * 1024 * 1024
  ) {
    throw new Error(
      `Invalid maxChunkSize: ${maxChunkSize}. Must be between 512 and 16777216`,
    );
  }
  if (
    isNaN(initialWindowSize) ||
    initialWindowSize < 1024 ||
    initialWindowSize > 1024 * 1024 * 1024
  ) {
    throw new Error(
      `Invalid initialWindowSize: ${initialWindowSize}. Must be between 1024 and 1073741824`,
    );
  }
  if (isNaN(readTimeoutMs) || readTimeoutMs < 0) {
    throw new Error(`Invalid readTimeoutMs: ${readTimeoutMs}. Must be >= 0`);
  }
  if (isNaN(dialTimeoutMs) || dialTimeoutMs < 0) {
    throw new Error(`Invalid dialTimeoutMs: ${dialTimeoutMs}. Must be >= 0`);
  }
  if (!allowedNetworks || allowedNetworks.length === 0) {
    throw new Error('Allowed networks must not be empty');
  }

  const secrets: ServerSecrets = {
    tlsCert: env.SESAME_ENDPOINT_TLS_CERT,
    tlsKey: env.SESAME_ENDPOINT_TLS_KEY,
    caCert: env.SESAME_ENDPOINT_CA_CERT,
    clientCert: env.SESAME_ENDPOINT_CLIENT_CERT,
    clientKey: env.SESAME_ENDPOINT_CLIENT_KEY,
    proxyAuthToken: env.SESAME_ENDPOINT_PROXY_AUTH_TOKEN,
    proxyPassword: env.SESAME_ENDPOINT_PROXY_PASSWORD,
  };

  return {
    config: {
      host,
      port,
      maxChunkSize,
      initialWindowSize,
      readTimeoutMs,
      dialTimeoutMs,
      supportedPresets,
      enableOpportunisticTls,
      enableFlowControl,
      allowedNetworks,
      secrets,
    },
  };
}

function parsePort(val: string): number {
  const p = parseInt(val, 10);
  if (isNaN(p) || p < 1 || p > 65535) {
    throw new Error(`Invalid port: ${val}. Must be between 1 and 65535`);
  }
  return p;
}

function parsePresetList(val: string): FingerprintPreset[] {
  const parts = val.split(',').map(s => s.trim().toUpperCase());
  const presets: FingerprintPreset[] = [];
  for (const part of parts) {
    if (!part) continue;
    const preset = PRESET_MAP[part];
    if (preset === undefined) {
      throw new Error(
        `Unknown fingerprint preset: "${part}". Valid presets are: ${Object.keys(PRESET_MAP).join(', ')}`,
      );
    }
    if (preset !== FingerprintPreset.RUNTIME_DEFAULT) {
      // Known name, but the standard runtime cannot honor it: reject at
      // parse time with an accurate message rather than letting the
      // surface express a configuration that would fail later at
      // handshake time.
      throw new Error(
        `Fingerprint preset ${part} requires a custom TLSProvider; the standard runtime only supports RUNTIME_DEFAULT`,
      );
    }
    if (!presets.includes(preset)) {
      presets.push(preset);
    }
  }
  if (presets.length === 0) {
    presets.push(FingerprintPreset.RUNTIME_DEFAULT);
  }
  return presets;
}

export function formatHelp(): string {
  return `Usage: sesame-endpoint [options]

Sesame Endpoint Server (RemoteControl.NetConn)

Options:
  --host <string>                 Bind host address (default: 127.0.0.1)
  --port <number>                 Bind port (default: 50051)
  --max-chunk-size <bytes>        Max chunk size for stream payloads (default: 32768)
  --initial-window-size <bytes>   Initial stream flow control credit (default: 65535)
  --read-timeout-ms <ms>          Socket read timeout in ms (default: 30000, 0=disable)
  --dial-timeout-ms <ms>          Default upstream dial timeout in ms (default: 10000)
  --supported-presets <list>      Comma-separated supported presets (default: RUNTIME_DEFAULT)
  --enable-opportunistic-tls <b>  Enable in-stream UpgradeTLS (default: true)
  --enable-flow-control <b>       Enable stream flow control (default: true)
  --allowed-networks <list>       Comma-separated allowed target networks (default: tcp,tcp4,tcp6)
  -h, --help                      Show this help message
  -v, --version                   Show version

Environment Variables (for Secrets):
  SESAME_ENDPOINT_HOST               Host override
  SESAME_ENDPOINT_PORT               Port override
  SESAME_ENDPOINT_TLS_CERT           PEM server certificate for TLS listener
  SESAME_ENDPOINT_TLS_KEY            PEM server private key for TLS listener
  SESAME_ENDPOINT_CA_CERT            Custom Root CA PEM
  SESAME_ENDPOINT_CLIENT_CERT        Client certificate PEM for mTLS upstream
  SESAME_ENDPOINT_CLIENT_KEY         Client private key PEM for mTLS upstream
  SESAME_ENDPOINT_PROXY_AUTH_TOKEN   Proxy authentication bearer/token
  SESAME_ENDPOINT_PROXY_PASSWORD     Proxy authentication password
`;
}
