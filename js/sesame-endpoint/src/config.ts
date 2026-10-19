import * as fs from 'node:fs';

export interface ServerSecrets {
  readonly tlsCert?: string;
  readonly tlsKey?: string;
  readonly clientCaCert?: string;
  readonly caCert?: string;
  readonly clientCert?: string;
  readonly clientKey?: string;
  readonly proxyAuthToken?: string;
  readonly proxyPassword?: string;
  /** Inbound endpoint bearer token: every RPC must carry it as "Authorization: Bearer <token>". */
  readonly authToken?: string;
}

export interface ServerConfig {
  readonly host: string;
  readonly port: number;
  readonly maxChunkSize: number;
  readonly initialWindowSize: number;
  readonly readTimeoutMs: number;
  readonly dialTimeoutMs: number;
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

// Returns true when the value looks like PEM content rather than a path.
// Real PEM starts with '-----BEGIN ' and has a matching '-----END '.
function isPemLiteral(val: string): boolean {
  const trimmed = val.trim();
  return trimmed.startsWith('-----BEGIN ') && trimmed.includes('-----END ');
}

// Resolves TLS material to its PEM content: literal PEM is kept as-is,
// anything else is treated as a file path. Empty values, empty files, and
// files whose content is not PEM are rejected loudly: a value resolving to
// '' would be silently treated as "TLS disabled" by downstream truthiness
// checks, and arbitrary file content would be re-interpreted as a path.
// If an input contains newlines, carriage returns, or null bytes, it is
// treated as an inline string attempt, never passed to fs.readFileSync (which
// would echo raw secret material in ENOENT/ENAMETOOLONG error messages).
function readCertOrLiteral(val: string, sourceName: string): string {
  if (val.trim().length === 0) {
    throw new Error(`Empty value for ${sourceName}`);
  }
  if (isPemLiteral(val)) {
    return val;
  }
  if (
    val.includes('\n') ||
    val.includes('\r') ||
    val.includes('\0') ||
    val.includes('-----END')
  ) {
    throw new Error(`Invalid PEM content for ${sourceName}`);
  }
  const content = fs.readFileSync(val, 'utf8');
  if (content.trim().length === 0) {
    throw new Error(`Empty content for ${sourceName}: ${val}`);
  }
  if (!isPemLiteral(content)) {
    throw new Error(`Invalid PEM content for ${sourceName}: ${val}`);
  }
  return content;
}

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

  // TLS material is resolved after the flag loop has completed so that
  // --help and --version are respected before any file I/O occurs, and
  // earlier flags can be cleanly overridden by later flags.
  let rawTlsCert: {val: string; source: string} | undefined;
  let rawTlsKey: {val: string; source: string} | undefined;
  let rawClientCaCert: {val: string; source: string} | undefined;

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
    } else if (arg === '--tls-cert') {
      if (i + 1 >= argv.length || argv[i + 1].startsWith('-'))
        throw new Error('Missing value for --tls-cert');
      rawTlsCert = {val: argv[++i], source: '--tls-cert'};
    } else if (arg.startsWith('--tls-cert=')) {
      rawTlsCert = {
        val: arg.slice('--tls-cert='.length),
        source: '--tls-cert',
      };
    } else if (arg === '--tls-key') {
      if (i + 1 >= argv.length || argv[i + 1].startsWith('-'))
        throw new Error('Missing value for --tls-key');
      rawTlsKey = {val: argv[++i], source: '--tls-key'};
    } else if (arg.startsWith('--tls-key=')) {
      rawTlsKey = {
        val: arg.slice('--tls-key='.length),
        source: '--tls-key',
      };
    } else if (arg === '--client-ca-cert') {
      if (i + 1 >= argv.length || argv[i + 1].startsWith('-'))
        throw new Error('Missing value for --client-ca-cert');
      rawClientCaCert = {val: argv[++i], source: '--client-ca-cert'};
    } else if (arg.startsWith('--client-ca-cert=')) {
      rawClientCaCert = {
        val: arg.slice('--client-ca-cert='.length),
        source: '--client-ca-cert',
      };
    } else {
      throw new Error(`Unknown CLI argument: ${arg}`);
    }
  }

  // Resolve TLS listener material after flag loop completes: CLI precedence
  // applies first, then environment variables fill gaps. Present-but-blank
  // values are rejected loudly rather than treated as unset.
  let tlsCert: string | undefined;
  let tlsKey: string | undefined;
  let clientCaCert: string | undefined;

  if (rawTlsCert !== undefined) {
    tlsCert = readCertOrLiteral(rawTlsCert.val, rawTlsCert.source);
  } else if (env.SESAME_ENDPOINT_TLS_CERT !== undefined) {
    tlsCert = readCertOrLiteral(
      env.SESAME_ENDPOINT_TLS_CERT,
      'SESAME_ENDPOINT_TLS_CERT',
    );
  }

  if (rawTlsKey !== undefined) {
    tlsKey = readCertOrLiteral(rawTlsKey.val, rawTlsKey.source);
  } else if (env.SESAME_ENDPOINT_TLS_KEY !== undefined) {
    tlsKey = readCertOrLiteral(
      env.SESAME_ENDPOINT_TLS_KEY,
      'SESAME_ENDPOINT_TLS_KEY',
    );
  }

  if (rawClientCaCert !== undefined) {
    clientCaCert = readCertOrLiteral(
      rawClientCaCert.val,
      rawClientCaCert.source,
    );
  } else if (env.SESAME_ENDPOINT_CLIENT_CA_CERT !== undefined) {
    clientCaCert = readCertOrLiteral(
      env.SESAME_ENDPOINT_CLIENT_CA_CERT,
      'SESAME_ENDPOINT_CLIENT_CA_CERT',
    );
  }

  // Resolve upstream TLS secrets eagerly at the configuration entrypoint
  // per the AGENTS.md invariant: file paths are read into PEM content,
  // blank values fail closed, and clientCert/Key pairing is enforced.
  let caCert: string | undefined;
  let clientCert: string | undefined;
  let clientKey: string | undefined;

  if (env.SESAME_ENDPOINT_CA_CERT !== undefined) {
    caCert = readCertOrLiteral(
      env.SESAME_ENDPOINT_CA_CERT,
      'SESAME_ENDPOINT_CA_CERT',
    );
  }
  if (env.SESAME_ENDPOINT_CLIENT_CERT !== undefined) {
    clientCert = readCertOrLiteral(
      env.SESAME_ENDPOINT_CLIENT_CERT,
      'SESAME_ENDPOINT_CLIENT_CERT',
    );
  }
  if (env.SESAME_ENDPOINT_CLIENT_KEY !== undefined) {
    clientKey = readCertOrLiteral(
      env.SESAME_ENDPOINT_CLIENT_KEY,
      'SESAME_ENDPOINT_CLIENT_KEY',
    );
  }

  if ((clientCert === undefined) !== (clientKey === undefined)) {
    throw new Error(
      'Both client certificate (SESAME_ENDPOINT_CLIENT_CERT) and client key (SESAME_ENDPOINT_CLIENT_KEY) must be provided together',
    );
  }

  // Inbound endpoint bearer token: present-but-blank fails closed like the
  // other secrets. Unset means no application-layer enforcement (mTLS stays
  // the channel authentication). The value is trimmed to match the client
  // side's trimming, never logged, and compared constant-time at the door.
  let authToken: string | undefined;
  if (env.SESAME_ENDPOINT_TOKEN !== undefined) {
    const trimmed = env.SESAME_ENDPOINT_TOKEN.trim();
    if (trimmed.length === 0) {
      throw new Error('SESAME_ENDPOINT_TOKEN must not be blank');
    }
    authToken = trimmed;
  }

  // Validate values
  if (!host || host.trim().length === 0) {
    throw new Error('Host cannot be empty');
  }
  if (isNaN(port) || port < 1 || port > 65535) {
    throw new Error(`Invalid port: ${port}. Must be between 1 and 65535`);
  }
  // The wire contract (remotecontrol.proto) requires advertised chunk and
  // window sizes not to exceed the stream's gRPC per-message receive
  // limit; the grpc-go default is 4MiB, so that is the interop-safe
  // ceiling here.
  const MAX_CHUNK_SIZE_CEILING = 4 * 1024 * 1024;

  if (
    isNaN(maxChunkSize) ||
    maxChunkSize < 512 ||
    maxChunkSize > MAX_CHUNK_SIZE_CEILING
  ) {
    throw new Error(
      `Invalid maxChunkSize: ${maxChunkSize}. Must be between 512 and ${MAX_CHUNK_SIZE_CEILING} (the gRPC per-message receive limit; advertised values must not exceed it per the wire contract)`,
    );
  }
  if (
    isNaN(initialWindowSize) ||
    initialWindowSize < 1024 ||
    initialWindowSize > MAX_CHUNK_SIZE_CEILING
  ) {
    throw new Error(
      `Invalid initialWindowSize: ${initialWindowSize}. Must be between 1024 and ${MAX_CHUNK_SIZE_CEILING} (the gRPC per-message receive limit; advertised values must not exceed it per the wire contract)`,
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
  // An empty entry (e.g. a trailing comma or a blank flag value) can never
  // match a well-formed dial request network, so it would silently narrow
  // policy; reject it so the operator's intent is explicit.
  for (const n of allowedNetworks) {
    if (n.length === 0) {
      throw new Error(
        'Allowed networks must not contain empty entries (check for stray commas)',
      );
    }
  }

  if (
    clientCaCert !== undefined &&
    (tlsCert === undefined || tlsKey === undefined)
  ) {
    throw new Error(
      'Client CA certificate (--client-ca-cert or SESAME_ENDPOINT_CLIENT_CA_CERT) requires both TLS certificate and key to be configured',
    );
  }

  // The TLS listener needs both halves: a lone certificate or key would
  // otherwise fall through to the plaintext listener on truthiness checks.
  if ((tlsCert === undefined) !== (tlsKey === undefined)) {
    throw new Error(
      'Both TLS certificate (--tls-cert or SESAME_ENDPOINT_TLS_CERT) and TLS key (--tls-key or SESAME_ENDPOINT_TLS_KEY) must be provided together',
    );
  }

  const secrets: ServerSecrets = {
    tlsCert,
    tlsKey,
    clientCaCert,
    caCert,
    clientCert,
    clientKey,
    proxyAuthToken: env.SESAME_ENDPOINT_PROXY_AUTH_TOKEN,
    proxyPassword: env.SESAME_ENDPOINT_PROXY_PASSWORD,
    authToken,
  };

  return {
    config: {
      host,
      port,
      maxChunkSize,
      initialWindowSize,
      readTimeoutMs,
      dialTimeoutMs,
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

export function formatHelp(): string {
  return `Usage: sesame-endpoint [options]

Sesame Endpoint Server (RemoteControl.NetConn)

Options:
  --host <string>                 Bind host address (default: 127.0.0.1)
  --port <number>                 Bind port (default: 50051)
  --max-chunk-size <bytes>        Max chunk size for stream payloads (default: 32768, max: 4194304)
  --initial-window-size <bytes>   Initial stream flow control credit (default: 65535, max: 4194304)
  --read-timeout-ms <ms>          Socket read timeout in ms (default: 30000, 0=disable)
  --dial-timeout-ms <ms>          Default upstream dial timeout in ms (default: 10000)
  --enable-opportunistic-tls <b>  Enable in-stream upgrade_tls (default: true)
  --enable-flow-control <b>       Enable stream flow control (default: true)
  --allowed-networks <list>       Comma-separated allowed target networks (default: tcp,tcp4,tcp6)
  --tls-cert <path|pem>           Server certificate for the TLS listener (file path or literal PEM)
  --tls-key <path|pem>            Server private key for the TLS listener (file path or literal PEM)
  --client-ca-cert <path|pem>     Client CA certificate for mTLS verification (file path or literal PEM)
  -h, --help                      Show this help message
  -v, --version                   Show version

Environment Variables (for Secrets):
  SESAME_ENDPOINT_HOST               Host override
  SESAME_ENDPOINT_PORT               Port override
  SESAME_ENDPOINT_TLS_CERT           Server certificate for the TLS listener (file path or literal PEM)
  SESAME_ENDPOINT_TLS_KEY            Server private key for the TLS listener (file path or literal PEM)
  SESAME_ENDPOINT_CLIENT_CA_CERT     Client CA certificate for mTLS verification (file path or literal PEM)
  SESAME_ENDPOINT_CA_CERT            Custom Root CA (file path or literal PEM)
  SESAME_ENDPOINT_CLIENT_CERT        Client certificate for mTLS upstream (file path or literal PEM)
  SESAME_ENDPOINT_CLIENT_KEY         Client private key for mTLS upstream (file path or literal PEM)
  SESAME_ENDPOINT_PROXY_AUTH_TOKEN   Proxy authentication bearer/token
  SESAME_ENDPOINT_PROXY_PASSWORD     Proxy authentication password
  SESAME_ENDPOINT_TOKEN             Inbound endpoint bearer token: every RPC must carry it as "Authorization: Bearer <token>" (pair with the shaper's -sesame-token-env); unset disables the check
`;
}
