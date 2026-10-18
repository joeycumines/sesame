import * as net from 'node:net';
import * as tls from 'node:tls';
import {Code, ConnectError} from '@connectrpc/connect';
import {create} from '@bufbuild/protobuf';
import {
  ClientHelloCapabilities,
  ClientHelloCapabilitiesSchema,
  ClientHelloSpec,
  TlsHandshakeResult,
  TlsHandshakeResultSchema,
  TlsOptions,
  TlsVersion,
} from '../gen/sesame/v1alpha1/tls_pb';
import {
  ProxyHop,
  ProxyHop_Type,
  ProxyOptions,
  ProxyResult,
  ProxyResultSchema,
} from '../gen/sesame/v1alpha1/proxy_pb';
import {NetAddr, NetAddrSchema} from '../gen/sesame/type/netaddr_pb';
import {ServerSecrets} from '../config';

export function protoToTlsVersion(
  v: TlsVersion,
): tls.SecureVersion | undefined {
  switch (v) {
    case TlsVersion.TLS_1_0:
      return 'TLSv1';
    case TlsVersion.TLS_1_1:
      return 'TLSv1.1';
    case TlsVersion.TLS_1_2:
      return 'TLSv1.2';
    case TlsVersion.TLS_1_3:
      return 'TLSv1.3';
    default:
      return undefined;
  }
}

export function tlsVersionToProto(v: string | null | undefined): TlsVersion {
  switch (v) {
    case 'TLSv1':
      return TlsVersion.TLS_1_0;
    case 'TLSv1.1':
      return TlsVersion.TLS_1_1;
    case 'TLSv1.2':
      return TlsVersion.TLS_1_2;
    case 'TLSv1.3':
      return TlsVersion.TLS_1_3;
    default:
      return TlsVersion.UNSPECIFIED;
  }
}

export function parseHostPort(addr: string): {host: string; port: number} {
  // A bare IPv6 literal has no port to extract; without this check it
  // garbage-parses (lastIndexOf(':') splits mid-address) and only fails
  // later at dial time with a confusing error.
  if (net.isIPv6(addr)) {
    throw new ConnectError(
      `sesame/rc/netconn: target address must specify a port: [${addr}]:port`,
      Code.InvalidArgument,
    );
  }
  const lastColon = addr.lastIndexOf(':');
  if (lastColon === -1) {
    throw new ConnectError(
      `sesame/rc/netconn: Invalid host:port address: ${addr}`,
      Code.InvalidArgument,
    );
  }
  let host = addr.slice(0, lastColon);
  if (host.startsWith('[') && host.endsWith(']')) {
    host = host.slice(1, -1);
  } else if (host.includes(':') || host.includes('[') || host.includes(']')) {
    // No complete bracket pair, yet the host carries a separator or a
    // stray bracket: the Go reference (net.SplitHostPort) rejects "too
    // many colons", "missing ']'", and "unexpected ']'", and splitting
    // at the last colon would otherwise silently mis-parse.
    throw new ConnectError(
      `sesame/rc/netconn: Invalid host:port address: ${addr}`,
      Code.InvalidArgument,
    );
  }
  const portStr = addr.slice(lastColon + 1);
  // The port must be pure ASCII digits: parseInt tolerates trailing
  // garbage ("host:80x" would dial host:80), leading whitespace, and a
  // sign - none of which belong in a wire address, and none of which
  // the Go reference accepts.
  if (!/^\d+$/.test(portStr)) {
    throw new ConnectError(
      `sesame/rc/netconn: Invalid port in address: ${addr}`,
      Code.InvalidArgument,
    );
  }
  const port = parseInt(portStr, 10);
  if (port < 1 || port > 65535) {
    throw new ConnectError(
      `sesame/rc/netconn: Invalid port in address: ${addr}`,
      Code.InvalidArgument,
    );
  }
  return {host, port};
}

export function createNetAddrFromSocket(
  socket: net.Socket,
  type: 'local' | 'remote',
): NetAddr {
  const host =
    type === 'local' ? socket.localAddress || '' : socket.remoteAddress || '';
  const port =
    type === 'local' ? socket.localPort || 0 : socket.remotePort || 0;
  const isIpv6 = host.includes(':');
  const formatted = isIpv6 ? `[${host}]:${port}` : `${host}:${port}`;
  return create(NetAddrSchema, {
    network: isIpv6 ? 'tcp6' : 'tcp4',
    address: formatted,
  });
}

export interface TLSExecutionResult {
  readonly tlsSocket: tls.TLSSocket;
  readonly result: TlsHandshakeResult;
}

// TLSProvider mirrors the Go rc/netconn.TLSProvider contract: a custom
// engine that can perform a TLS handshake with full ClientHello control.
// When present, the builtin node:tls path is bypassed entirely.
// Providers SHOULD honor the signal and abort the handshake promptly;
// the Go contract passes the request context for the same purpose.
export interface TLSProvider {
  handshake(
    socket: net.Socket,
    opts: TlsOptions,
    defaultServerName: string,
    fallbackSecrets?: ServerSecrets,
    signal?: AbortSignal,
  ): Promise<TLSExecutionResult>;
}

// validateClientHelloSpec checks numeric ranges in the spec. Throws
// ConnectError(Code.InvalidArgument) on any violation. Call before any
// engine-specific logic.
export function validateClientHelloSpec(
  spec: ClientHelloSpec | undefined,
): void {
  if (!spec) return;
  for (const v of spec.cipherSuites) {
    if (v < 0 || v > 65535) {
      throw new ConnectError(
        `sesame/rc/netconn: cipher_suites value out of range [0, 65535]: ${v}`,
        Code.InvalidArgument,
      );
    }
  }
  for (const v of spec.supportedGroups) {
    if (v < 0 || v > 65535) {
      throw new ConnectError(
        `sesame/rc/netconn: supported_groups value out of range [0, 65535]: ${v}`,
        Code.InvalidArgument,
      );
    }
  }
  for (const v of spec.signatureAlgorithms) {
    if (v < 0 || v > 65535) {
      throw new ConnectError(
        `sesame/rc/netconn: signature_algorithms value out of range [0, 65535]: ${v}`,
        Code.InvalidArgument,
      );
    }
  }
  for (const ext of spec.extensions) {
    if (!ext) {
      throw new ConnectError(
        'sesame/rc/netconn: nil extension entry in client_hello.extensions',
        Code.InvalidArgument,
      );
    }
    if (ext.type < 0 || ext.type > 65535) {
      throw new ConnectError(
        `sesame/rc/netconn: extension type out of range [0, 65535]: ${ext.type}`,
        Code.InvalidArgument,
      );
    }
  }
  for (const v of spec.compressionMethods) {
    if (v < 0 || v > 255) {
      throw new ConnectError(
        `sesame/rc/netconn: compression_methods value out of range [0, 255]: ${v}`,
        Code.InvalidArgument,
      );
    }
  }
  if (spec.sessionIdLength < -1 || spec.sessionIdLength > 32) {
    throw new ConnectError(
      `sesame/rc/netconn: session_id_length out of range [-1, 32]: ${spec.sessionIdLength}`,
      Code.InvalidArgument,
    );
  }
  if (spec.targetSize < 0) {
    throw new ConnectError(
      `sesame/rc/netconn: target_size must not be negative: ${spec.targetSize}`,
      Code.InvalidArgument,
    );
  }
}

// assertBuiltinClientHelloHonorable checks that the builtin node:tls/Bun
// engine can honor the spec. The builtin engine has NO public control for
// any ClientHello dimension, so any non-default value is rejected with
// FAILED_PRECONDITION. An empty/default spec is always honorable.
export function assertBuiltinClientHelloHonorable(
  spec: ClientHelloSpec | undefined,
): void {
  if (!spec) return;
  if (spec.cipherSuites.length > 0) {
    throw new ConnectError(
      'sesame/rc/netconn: builtin engine cannot honor client_hello.cipher_suites; custom TLSProvider required',
      Code.FailedPrecondition,
    );
  }
  if (spec.supportedGroups.length > 0) {
    throw new ConnectError(
      'sesame/rc/netconn: builtin engine cannot honor client_hello.supported_groups; custom TLSProvider required',
      Code.FailedPrecondition,
    );
  }
  if (spec.signatureAlgorithms.length > 0) {
    throw new ConnectError(
      'sesame/rc/netconn: builtin engine cannot honor client_hello.signature_algorithms; custom TLSProvider required',
      Code.FailedPrecondition,
    );
  }
  if (spec.extensions.length > 0) {
    throw new ConnectError(
      'sesame/rc/netconn: builtin engine cannot honor client_hello.extensions; custom TLSProvider required',
      Code.FailedPrecondition,
    );
  }
  // compressionMethods: accept [] or exactly [0] (null compression).
  if (
    spec.compressionMethods.length > 0 &&
    !(spec.compressionMethods.length === 1 && spec.compressionMethods[0] === 0)
  ) {
    throw new ConnectError(
      'sesame/rc/netconn: builtin engine cannot honor client_hello.compression_methods; custom TLSProvider required',
      Code.FailedPrecondition,
    );
  }
  // sessionIdLength: 0 (engine default) and 32 (explicit match) are
  // compatible; these runtimes send a 32-byte session id.
  if (spec.sessionIdLength !== 0 && spec.sessionIdLength !== 32) {
    throw new ConnectError(
      `sesame/rc/netconn: builtin engine cannot honor client_hello.session_id_length=${spec.sessionIdLength}; custom TLSProvider required`,
      Code.FailedPrecondition,
    );
  }
  // legacyVersion: must be UNSPECIFIED (engine default).
  if (spec.legacyVersion !== TlsVersion.UNSPECIFIED) {
    throw new ConnectError(
      'sesame/rc/netconn: builtin engine cannot honor client_hello.legacy_version; custom TLSProvider required',
      Code.FailedPrecondition,
    );
  }
  // targetSize: must be 0 (no padding).
  if (spec.targetSize !== 0) {
    throw new ConnectError(
      'sesame/rc/netconn: builtin engine cannot honor client_hello.target_size; custom TLSProvider required',
      Code.FailedPrecondition,
    );
  }
}

// builtinClientHelloCapabilities returns the honest capability advertisement
// for the builtin node:tls/Bun engine: all dimensions false, because the
// builtin engine has no public ClientHello control. Compatible defaults are
// not control.
export function builtinClientHelloCapabilities(): ClientHelloCapabilities {
  return create(ClientHelloCapabilitiesSchema, {});
}

// IANA cipher suite identifiers for the names Node/Bun report from
// TLSSocket.getCipher() (standardName preferred, opensslName fallback).
// Unrecognized names yield 0; the negotiated suite is metadata, not a
// security decision, so an unmapped runtime name must not fail the
// handshake.
const IANA_CIPHER_SUITES: Record<string, number> = {
  TLS_AES_128_GCM_SHA256: 0x1301,
  TLS_AES_256_GCM_SHA384: 0x1302,
  TLS_CHACHA20_POLY1305_SHA256: 0x1303,
  TLS_AES_128_CCM_SHA256: 0x1304,
  TLS_AES_128_CCM_8_SHA256: 0x1305,
  TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256: 0xc02f,
  TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384: 0xc030,
  TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256: 0xc02b,
  TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384: 0xc02c,
  TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256: 0xcca8,
  TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256: 0xcca9,
  TLS_RSA_WITH_AES_128_GCM_SHA256: 0x009c,
  TLS_RSA_WITH_AES_256_GCM_SHA384: 0x009d,
  TLS_RSA_WITH_AES_128_CBC_SHA256: 0x003c,
  TLS_RSA_WITH_AES_256_CBC_SHA256: 0x003d,
  TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256: 0xc027,
  TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA384: 0xc028,
};

function ianaCipherSuite(tlsSocket: tls.TLSSocket): number {
  const cipher = tlsSocket.getCipher();
  if (!cipher) {
    return 0;
  }
  // CipherNameAndProtocol.name carries the OpenSSL name;
  // standardName the IETF name.
  return (
    IANA_CIPHER_SUITES[cipher.standardName] ??
    IANA_CIPHER_SUITES[cipher.name] ??
    0
  );
}

export const MAX_PROXY_HOPS = 8;

// Fallback bound for TLS handshakes when no request deadline applies,
// mirroring the Go reference's handshakeFallbackTimeout (30s) and the
// same rationale as the proxy-handshake bounds: a peer that accepts
// TCP but never answers the ClientHello must not hang the handshake.
// The dial-time handshake instead uses the request's dial timeout
// (establishment semantics); in-stream upgrades have no live dial
// timeout to inherit and take this fixed operational fallback.
export const HANDSHAKE_FALLBACK_TIMEOUT_MS = 30_000;

export async function executeTLSHandshake(
  socket: net.Socket,
  opts: TlsOptions,
  defaultServerName: string,
  fallbackSecrets?: ServerSecrets,
  provider?: TLSProvider,
  signal?: AbortSignal,
  handshakeTimeoutMs?: number,
): Promise<TLSExecutionResult> {
  // Version-range validation at the API boundary, matching the Go
  // reference: a floor above its ceiling can never negotiate.
  // TlsVersion values are monotonic, so the enum compares directly.
  if (
    opts.minVersion !== TlsVersion.UNSPECIFIED &&
    opts.maxVersion !== TlsVersion.UNSPECIFIED &&
    opts.minVersion > opts.maxVersion
  ) {
    throw new ConnectError(
      'sesame/rc/netconn: min_version exceeds max_version',
      Code.InvalidArgument,
    );
  }

  // ClientHelloSpec validation applies to every provider path.
  validateClientHelloSpec(opts.clientHello);

  // When a custom provider is present, delegate after range+schema
  // validation. The provider itself is responsible for honoring
  // client_hello fail-closed and echoing appliedClientHello.
  if (provider) {
    return provider.handshake(
      socket,
      opts,
      defaultServerName,
      fallbackSecrets,
      signal,
    );
  }

  // Builtin engine: fail closed on any non-default spec dimension.
  assertBuiltinClientHelloHonorable(opts.clientHello);

  let sni = opts.sni || defaultServerName;
  // A bare IPv6 literal is already a valid SNI value; only strip a port when
  // the value is a bracketed literal or a name:port pair. Zoned literals
  // (e.g. '::1%eth0') are not bare IPv6 - like Go's net.ParseIP, which
  // rejects zone IDs - so they fall through to parseHostPort and are
  // rejected with InvalidArgument (a zone is interface-local and
  // meaningless as SNI), matching the Go reference.
  if (sni.includes(':') && !(net.isIPv6(sni) && !sni.includes('%'))) {
    sni = parseHostPort(sni).host;
  }
  // A zone ID (RFC 4007 "address%zone") is interface-local and can
  // never be SNI (RFC 6066 hostnames carry no '%'): a bracketed zoned
  // literal must not survive the port-strip either - matching the
  // bare-form rejection above and the Go reference, which rejects with
  // the same message.
  if (sni.includes('%')) {
    throw new ConnectError(
      `sesame/rc/netconn: invalid sni "${sni}": zone ID not permitted`,
      Code.InvalidArgument,
    );
  }

  const tlsConnectOptions: tls.ConnectionOptions = {
    socket,
    servername: sni,
    rejectUnauthorized: !opts.insecureSkipVerify,
  };

  // If alpn_protocols is empty, ALPN extension MUST NOT be sent.
  if (opts.alpnProtocols && opts.alpnProtocols.length > 0) {
    tlsConnectOptions.ALPNProtocols = [...opts.alpnProtocols];
  }

  const minVer = protoToTlsVersion(opts.minVersion);
  if (minVer) {
    tlsConnectOptions.minVersion = minVer;
  }
  const maxVer = protoToTlsVersion(opts.maxVersion);
  if (maxVer) {
    tlsConnectOptions.maxVersion = maxVer;
  }

  if (opts.caCertificates && opts.caCertificates.length > 0) {
    tlsConnectOptions.ca = Buffer.from(opts.caCertificates);
  } else if (fallbackSecrets?.caCert) {
    tlsConnectOptions.ca = Buffer.from(fallbackSecrets.caCert);
  }

  if (opts.clientCertificate && opts.clientCertificate.length > 0) {
    tlsConnectOptions.cert = Buffer.from(opts.clientCertificate);
  } else if (fallbackSecrets?.clientCert) {
    tlsConnectOptions.cert = Buffer.from(fallbackSecrets.clientCert);
  }

  if (opts.clientPrivateKey && opts.clientPrivateKey.length > 0) {
    tlsConnectOptions.key = Buffer.from(opts.clientPrivateKey);
  } else if (fallbackSecrets?.clientKey) {
    tlsConnectOptions.key = Buffer.from(fallbackSecrets.clientKey);
  }

  return new Promise<TLSExecutionResult>((resolve, reject) => {
    let resolved = false;
    const tlsSocket = tls.connect(tlsConnectOptions);

    // An aborted request stops the handshake immediately instead of
    // letting it run until the peer or the runtime gives up, mirroring
    // dialTcp and the Go reference's HandshakeContext(ctx).
    const onAbort = () => {
      if (resolved) return;
      resolved = true;
      cleanup();
      tlsSocket.destroy();
      reject(
        new ConnectError(
          'sesame/rc/netconn: connection aborted by client',
          Code.Canceled,
        ),
      );
    };

    // A peer that accepts TCP but never answers the ClientHello must
    // not hang the handshake until the client gives up: the same
    // wall-clock bound the proxy handshakes apply. Custom TLSProviders
    // own their handshake bounds.
    let handshakeTimer: ReturnType<typeof setTimeout> | undefined;
    if (handshakeTimeoutMs && handshakeTimeoutMs > 0) {
      handshakeTimer = setTimeout(() => {
        if (resolved) return;
        resolved = true;
        cleanup();
        tlsSocket.destroy();
        reject(
          new ConnectError(
            'sesame/rc/netconn: TLS handshake timeout',
            Code.DeadlineExceeded,
          ),
        );
      }, handshakeTimeoutMs);
    }

    const onSecureConnect = () => {
      if (resolved) return;
      resolved = true;
      cleanup();

      // Peer chain: walk issuerCertificate links from the leaf to the
      // root (self-signed or empty cert terminates the walk), mirroring
      // the Go reference which reports the full presented chain.
      const peerCertificates: Uint8Array[] = [];
      let cert = tlsSocket.getPeerCertificate(true);
      const seen = new Set<tls.PeerCertificate>();
      while (cert && cert.raw && cert.raw.length > 0 && !seen.has(cert)) {
        seen.add(cert);
        peerCertificates.push(new Uint8Array(cert.raw));
        cert = cert.issuerCertificate;
      }

      const proto = tlsSocket.alpnProtocol;
      const negotiatedProtocol = typeof proto === 'string' ? proto : '';

      const result = create(TlsHandshakeResultSchema, {
        negotiatedProtocol,
        cipherSuite: ianaCipherSuite(tlsSocket),
        tlsVersion: tlsVersionToProto(tlsSocket.getProtocol()),
        sni,
        peerCertificates,
        appliedClientHello: opts.clientHello,
      });

      resolve({tlsSocket, result});
    };

    const onError = (err: Error) => {
      if (resolved) return;
      resolved = true;
      cleanup();
      tlsSocket.destroy();
      reject(
        new ConnectError(
          `sesame/rc/netconn: TLS handshake failed: ${err.message}`,
          Code.Unavailable,
        ),
      );
    };

    const cleanup = () => {
      tlsSocket.removeListener('secureConnect', onSecureConnect);
      tlsSocket.removeListener('error', onError);
      if (handshakeTimer !== undefined) {
        clearTimeout(handshakeTimer);
      }
      signal?.removeEventListener('abort', onAbort);
    };

    // The guard covers an abort that fired while a previous stage held
    // the only listener: AbortSignal does not replay past events.
    if (signal) {
      if (signal.aborted) {
        onAbort();
        return;
      }
      signal.addEventListener('abort', onAbort, {once: true});
    }

    tlsSocket.once('secureConnect', onSecureConnect);
    tlsSocket.once('error', onError);
  });
}

export interface ProxyExecutionResult {
  readonly socket: net.Socket;
  readonly result: ProxyResult;
}

export async function executeProxyHops(
  targetNetwork: string,
  targetAddress: string,
  proxyOpts?: ProxyOptions,
  timeoutMs = 10000,
  fallbackSecrets?: ServerSecrets,
  signal?: AbortSignal,
): Promise<ProxyExecutionResult> {
  const hops = proxyOpts?.hops ?? [];
  if (hops.length === 0) {
    const hp = parseHostPort(targetAddress);
    const socket = await dialTcp(hp.host, hp.port, timeoutMs, signal);
    const result = create(ProxyResultSchema, {
      traversedHops: [],
      egressAddress: createNetAddrFromSocket(socket, 'remote'),
    });
    return {socket, result};
  }

  // Each hop is a sequential dial plus handshake, so the count is a server
  // resource commitment; reject chains beyond the reference bound.
  if (hops.length > MAX_PROXY_HOPS) {
    throw new ConnectError(
      `sesame/rc/netconn: too many proxy hops: ${hops.length} (max ${MAX_PROXY_HOPS})`,
      Code.InvalidArgument,
    );
  }

  validateProxyWireFields(targetAddress, hops);
  validateSocks5Targets(targetAddress, hops);
  validateProxyCredentials(hops);
  // The tunnel target and every hop address must parse as host:port
  // before any dialing, mirroring the Go reference: malformed input is
  // INVALID_ARGUMENT (AIP-193), not a late dial failure, and a chain
  // with a missing address never spends a server-side dial.
  parseHostPort(targetAddress);
  hops.forEach((hop, i) => {
    const addr = hop.address?.address;
    if (!addr) {
      throw new ConnectError(
        i === 0
          ? 'sesame/rc/netconn: first proxy hop missing address'
          : `sesame/rc/netconn: proxy hop ${i} missing address`,
        Code.InvalidArgument,
      );
    }
    try {
      parseHostPort(addr);
    } catch (err: unknown) {
      // Name the offending hop like the Go reference does, keeping the
      // underlying reason and code. ConnectError.message embeds a
      // "[code] " tag, so the bare reason text is rawMessage; strip
      // its module prefix to avoid nesting it under this message.
      const reason =
        err instanceof ConnectError
          ? err.rawMessage.replace(/^sesame\/rc\/netconn: /, '')
          : String(err);
      throw new ConnectError(
        `sesame/rc/netconn: invalid proxy hop ${i} address: ${reason}`,
        Code.InvalidArgument,
      );
    }
  });

  // Rejection happens after validation, matching the Go reference: a
  // request that is both malformed and aborted reports the malformed
  // input (InvalidArgument), and only a well-formed aborted request
  // surfaces the cancellation before any dial.
  if (signal?.aborted) {
    throw new ConnectError(
      'sesame/rc/netconn: connection aborted by client',
      Code.Canceled,
    );
  }

  const traversedHops: NetAddr[] = [];
  const firstHop = hops[0];
  const firstHp = parseHostPort(firstHop.address!.address);
  let currentSocket = await dialTcp(
    firstHp.host,
    firstHp.port,
    timeoutMs,
    signal,
  );
  traversedHops.push(firstHop.address!);

  for (let i = 0; i < hops.length; i++) {
    const hop = hops[i];
    let nextAddrStr: string;
    if (i + 1 < hops.length) {
      nextAddrStr = hops[i + 1].address!.address;
    } else {
      nextAddrStr = targetAddress;
    }

    switch (hop.type) {
      case ProxyHop_Type.HTTP_CONNECT: {
        currentSocket = await httpConnectHandshake(
          currentSocket,
          hop,
          nextAddrStr,
          timeoutMs,
          fallbackSecrets,
          signal,
        );
        break;
      }
      case ProxyHop_Type.SOCKS5: {
        currentSocket = await socks5Handshake(
          currentSocket,
          hop,
          nextAddrStr,
          timeoutMs,
          fallbackSecrets,
          signal,
        );
        break;
      }
      default: {
        currentSocket.destroy();
        throw new ConnectError(
          `sesame/rc/netconn: unsupported proxy type ${hop.type} on hop ${i}`,
          Code.InvalidArgument,
        );
      }
    }

    if (i + 1 < hops.length && hops[i + 1].address) {
      traversedHops.push(hops[i + 1].address!);
    }
  }

  const result = create(ProxyResultSchema, {
    traversedHops,
    egressAddress: createNetAddrFromSocket(currentSocket, 'remote'),
  });

  return {socket: currentSocket, result};
}

// validateProxyWireFields mirrors the Go reference: it rejects control
// bytes in the strings interpolated verbatim into a raw HTTP CONNECT
// request line - the tunnel target (request line and Host header), each
// hop's authHeader (Proxy-Authorization line), and each subsequent hop
// address (the CONNECT target of the next hop's handshake). CR/LF in
// any of them would inject header or request lines into the proxy
// request; other C0 controls and DEL are garbage in an authority or
// header value and are rejected by the same fail-closed rule.
// Credentials are outside this rule: usernames and passwords travel
// base64-encoded (CONNECT basic auth) or length-prefixed (SOCKS5 RFC
// 1929), so they cannot break request framing.
function validateProxyWireFields(
  targetAddress: string,
  hops: readonly ProxyHop[],
): void {
  rejectControlBytes(targetAddress, 'proxy target address');
  hops.forEach((hop, i) => {
    rejectControlBytes(hop.address?.address ?? '', `proxy hop ${i} address`);
    rejectControlBytes(hop.authHeader, `proxy hop ${i} auth_header`);
  });
}

// rejectControlBytes fails closed on any C0 control or DEL byte. The
// offending value is never echoed: proxy fields carry credentials.
function rejectControlBytes(value: string, field: string): void {
  for (let i = 0; i < value.length; i++) {
    const c = value.charCodeAt(i);
    if (c < 0x20 || c === 0x7f) {
      throw new ConnectError(
        `sesame/rc/netconn: ${field} contains a control byte at offset ${i}; proxy handshake fields must not carry CR/LF or other control characters`,
        Code.InvalidArgument,
      );
    }
  }
}

// validateSocks5Targets mirrors the Go reference: per SOCKS5 hop, the
// address that hop will CONNECT to (the next hop's address, or the
// tunnel target for the last hop) must be encodable - RFC 1928 carries
// a domain target behind a u8 length field (ATYP 0x03), so a non-IP
// host longer than 255 bytes would silently wrap the length and
// corrupt the frame. HTTP CONNECT has no such limit and is untouched.
function validateSocks5Targets(
  targetAddress: string,
  hops: readonly ProxyHop[],
): void {
  for (let i = 0; i < hops.length; i++) {
    if (hops[i].type !== ProxyHop_Type.SOCKS5) {
      continue;
    }
    const target =
      i + 1 < hops.length
        ? (hops[i + 1].address?.address ?? '')
        : targetAddress;
    const lastColon = target.lastIndexOf(':');
    if (lastColon === -1) {
      // Malformed targets are the dial path's problem; the
      // encodability check only rejects what cannot be encoded.
      continue;
    }
    let host = target.slice(0, lastColon);
    if (host.startsWith('[') && host.endsWith(']')) {
      host = host.slice(1, -1);
    }
    if (net.isIPv4(host) || net.isIPv6(host)) {
      continue;
    }
    // Count BYTES, not UTF-16 code units: the wire field carries an
    // octet count, and a multibyte (IDN) domain can be under 256
    // characters yet over 255 bytes - matching the Go reference's
    // byte-based check.
    const hostBytes = Buffer.byteLength(host, 'utf-8');
    if (hostBytes > 255) {
      throw new ConnectError(
        `sesame/rc/netconn: SOCKS5 target domain exceeds the RFC 1928 255-byte limit (${hostBytes} bytes)`,
        Code.InvalidArgument,
      );
    }
  }
}

// validateProxyCredentials mirrors the Go reference: it rejects, per
// hop, a password with no username. Both handshake builders gate
// authentication on a non-empty username (HTTP CONNECT emits
// Proxy-Authorization only when the username is set; SOCKS5 negotiates
// RFC 1929 auth only then), so a password-only hop would silently
// negotiate NO-AUTH and the configured password would never reach the
// proxy. A username with an empty password remains valid. The password
// value is never echoed.
function validateProxyCredentials(hops: readonly ProxyHop[]): void {
  hops.forEach((hop, i) => {
    if (hop.password && !hop.username) {
      throw new ConnectError(
        `sesame/rc/netconn: proxy hop ${i} has a password but no username; authentication requires a username (an empty password is valid)`,
        Code.InvalidArgument,
      );
    }
  });
}

function dialTcp(
  host: string,
  port: number,
  timeoutMs: number,
  signal?: AbortSignal,
): Promise<net.Socket> {
  return new Promise((resolve, reject) => {
    let resolved = false;
    const socket = net.createConnection({host, port});

    const timer = setTimeout(() => {
      if (resolved) return;
      resolved = true;
      signal?.removeEventListener('abort', onAbort);
      socket.destroy();
      reject(
        new ConnectError(
          `sesame/rc/netconn: dial timeout to ${host}:${port}`,
          Code.DeadlineExceeded,
        ),
      );
    }, timeoutMs);

    // An aborted request stops the dial immediately instead of letting
    // it run to its own timeout, mirroring the direct-dial path in
    // server.ts and the context cancellation the Go reference gets
    // from DialContext.
    const onAbort = () => {
      if (resolved) return;
      resolved = true;
      clearTimeout(timer);
      socket.destroy();
      reject(
        new ConnectError(
          'sesame/rc/netconn: connection aborted by client',
          Code.Canceled,
        ),
      );
    };

    const onConnect = () => {
      if (resolved) return;
      resolved = true;
      clearTimeout(timer);
      signal?.removeEventListener('abort', onAbort);
      socket.removeListener('error', onError);
      resolve(socket);
    };

    const onError = (err: Error) => {
      if (resolved) return;
      resolved = true;
      clearTimeout(timer);
      signal?.removeEventListener('abort', onAbort);
      socket.removeListener('connect', onConnect);
      reject(
        new ConnectError(
          `sesame/rc/netconn: dial error to ${host}:${port}: ${err.message}`,
          Code.Unavailable,
        ),
      );
    };

    // The guard covers an abort that fired while a previous stage held
    // the only listener: AbortSignal does not replay past events.
    if (signal) {
      if (signal.aborted) {
        onAbort();
        return;
      }
      signal.addEventListener('abort', onAbort, {once: true});
    }

    socket.once('connect', onConnect);
    socket.once('error', onError);
  });
}

function httpConnectHandshake(
  socket: net.Socket,
  hop: ProxyHop,
  target: string,
  timeoutMs: number,
  fallbackSecrets?: ServerSecrets,
  signal?: AbortSignal,
): Promise<net.Socket> {
  return new Promise((resolve, reject) => {
    let reqStr = `CONNECT ${target} HTTP/1.1\r\nHost: ${target}\r\n`;
    if (hop.authHeader) {
      reqStr += `Proxy-Authorization: ${hop.authHeader}\r\n`;
    } else if (fallbackSecrets?.proxyAuthToken) {
      reqStr += `Proxy-Authorization: Bearer ${fallbackSecrets.proxyAuthToken}\r\n`;
    } else if (hop.username) {
      const password = hop.password || fallbackSecrets?.proxyPassword || '';
      const creds = Buffer.from(`${hop.username}:${password}`).toString(
        'base64',
      );
      reqStr += `Proxy-Authorization: Basic ${creds}\r\n`;
    }
    reqStr += '\r\n';

    let buffer = Buffer.alloc(0);
    let resolved = false;

    const timer = setTimeout(() => {
      if (resolved) return;
      resolved = true;
      cleanup();
      socket.destroy();
      reject(
        new ConnectError(
          `sesame/rc/netconn: HTTP CONNECT timeout to ${target}`,
          Code.DeadlineExceeded,
        ),
      );
    }, timeoutMs);

    // An aborted request stops the handshake immediately instead of
    // letting it run to its own timeout, mirroring dialTcp and the
    // direct-dial path in server.ts.
    const onAbort = () => {
      if (resolved) return;
      resolved = true;
      clearTimeout(timer);
      cleanup();
      socket.destroy();
      reject(
        new ConnectError(
          'sesame/rc/netconn: connection aborted by client',
          Code.Canceled,
        ),
      );
    };

    const onData = (chunk: Buffer) => {
      buffer = Buffer.concat([buffer, chunk]);
      const headerEnd = buffer.indexOf('\r\n\r\n');
      if (headerEnd !== -1) {
        resolved = true;
        clearTimeout(timer);
        cleanup();

        const headerText = buffer.subarray(0, headerEnd).toString('utf-8');
        const firstLine = headerText.split('\r\n')[0] || '';
        const match = firstLine.match(/^HTTP\/1\.[01]\s+(\d{3})/);
        const statusCode = match ? parseInt(match[1], 10) : 0;

        if (statusCode !== 200) {
          socket.destroy();
          reject(
            new ConnectError(
              `sesame/rc/netconn: HTTP CONNECT to ${target} failed with status: ${firstLine}`,
              Code.PermissionDenied,
            ),
          );
          return;
        }

        // A 200 response to CONNECT must not carry a body. Any framing
        // (chunked) or declared Content-Length means the proxy is not
        // tunnelling, and its bytes must not enter the tunnel as payload.
        const headerLines = headerText.split('\r\n').slice(1);
        let framed = false;
        let contentLength = 0;
        for (const line of headerLines) {
          const idx = line.indexOf(':');
          if (idx === -1) continue;
          const name = line.slice(0, idx).trim().toLowerCase();
          const value = line.slice(idx + 1).trim();
          if (name === 'transfer-encoding') {
            // Any token other than identity means the body is framed.
            // A bare "identity" declares no transformation and is safe.
            const tokens = value
              .split(',')
              .map(t => t.trim().toLowerCase())
              .filter(t => t.length > 0);
            if (tokens.some(t => t !== 'identity')) framed = true;
          } else if (name === 'content-length') {
            // Take the max over all declared values: a second value of
            // "Content-Length: 0, 5" must not hide a body.
            for (const part of value.split(',')) {
              const n = parseInt(part.trim(), 10);
              if (!isNaN(n) && n > contentLength) contentLength = n;
            }
          }
        }
        if (framed || contentLength > 0) {
          socket.destroy();
          reject(
            new ConnectError(
              `sesame/rc/netconn: HTTP CONNECT to ${target} returned a framed response body`,
              Code.Unavailable,
            ),
          );
          return;
        }

        // Any bytes coalesced after the headers are legitimate early
        // tunnel payload (fast proxies pipeline the first upstream flight
        // in the same TCP segment). Push them back so the tunnel reads
        // them first, mirroring the SOCKS5 path and the Go reference
        // (bufferedPrefixConn). Pause first: unshifted bytes are only
        // re-emitted to a later 'data' listener if the stream is paused
        // when the listener attaches, so an unpaused socket would drop
        // them silently. The server pauses the socket before its first
        // read (see server.ts onData), matching the SOCKS5 path which
        // works because its handshake leaves the socket paused.
        const extra = buffer.subarray(headerEnd + 4);
        if (extra.length > 0) {
          socket.pause();
          socket.unshift(extra);
        }

        resolve(socket);
      }
    };

    const onError = (err: Error) => {
      if (resolved) return;
      resolved = true;
      clearTimeout(timer);
      cleanup();
      reject(
        new ConnectError(
          `sesame/rc/netconn: HTTP CONNECT error: ${err.message}`,
          Code.Unavailable,
        ),
      );
    };

    const cleanup = () => {
      socket.removeListener('data', onData);
      socket.removeListener('error', onError);
      signal?.removeEventListener('abort', onAbort);
    };

    // The guard covers an abort that fired while a previous stage held
    // the only listener: AbortSignal does not replay past events.
    if (signal) {
      if (signal.aborted) {
        onAbort();
        return;
      }
      signal.addEventListener('abort', onAbort, {once: true});
    }

    socket.on('data', onData);
    socket.once('error', onError);
    socket.write(reqStr);
  });
}

function socks5Handshake(
  socket: net.Socket,
  hop: ProxyHop,
  target: string,
  timeoutMs: number,
  fallbackSecrets?: ServerSecrets,
  signal?: AbortSignal,
): Promise<net.Socket> {
  return new Promise((resolve, reject) => {
    let resolved = false;
    let stage: 'greeting' | 'auth' | 'connect' = 'greeting';
    let buffer = Buffer.alloc(0);

    const timer = setTimeout(() => {
      if (resolved) return;
      resolved = true;
      cleanup();
      socket.destroy();
      reject(
        new ConnectError(
          `sesame/rc/netconn: SOCKS5 timeout to ${target}`,
          Code.DeadlineExceeded,
        ),
      );
    }, timeoutMs);

    const cleanup = () => {
      socket.removeListener('data', onData);
      socket.removeListener('error', onError);
      signal?.removeEventListener('abort', onAbort);
    };

    // An aborted request stops the handshake immediately instead of
    // letting it run to its own timeout, mirroring dialTcp and the
    // direct-dial path in server.ts.
    const onAbort = () => {
      if (resolved) return;
      resolved = true;
      clearTimeout(timer);
      cleanup();
      socket.destroy();
      reject(
        new ConnectError(
          'sesame/rc/netconn: connection aborted by client',
          Code.Canceled,
        ),
      );
    };

    const onError = (err: Error) => {
      if (resolved) return;
      resolved = true;
      clearTimeout(timer);
      cleanup();
      reject(
        new ConnectError(
          `sesame/rc/netconn: SOCKS5 error: ${err.message}`,
          Code.Unavailable,
        ),
      );
    };

    const onData = (chunk: Buffer) => {
      buffer = Buffer.concat([buffer, chunk]);
      try {
        if (stage === 'greeting') {
          if (buffer.length < 2) return;
          const version = buffer[0];
          const method = buffer[1];
          buffer = buffer.subarray(2);

          if (version !== 0x05) {
            throw new ConnectError(
              `sesame/rc/netconn: invalid SOCKS version: ${version}`,
              Code.Unavailable,
            );
          }

          if (method === 0x02) {
            // Username/Password authentication (RFC 1929)
            stage = 'auth';
            const username = hop.username;
            const password =
              hop.password || fallbackSecrets?.proxyPassword || '';
            const u = Buffer.from(username, 'utf-8');
            const p = Buffer.from(password, 'utf-8');
            // RFC 1929 fields are length-prefixed by a single byte;
            // anything longer cannot be encoded and would silently
            // corrupt the frame (a 256-byte username writes length
            // 0x00). The Go reference rejects the same input
            // (x/net socks: invalid username/password) as a
            // handshake failure; never echo the values themselves.
            if (u.length > 255 || p.length > 255) {
              throw new ConnectError(
                'sesame/rc/netconn: SOCKS5 username/password exceed the RFC 1929 255-byte limit',
                Code.Unavailable,
              );
            }
            const authReq = Buffer.concat([
              Buffer.from([0x01, u.length]),
              u,
              Buffer.from([p.length]),
              p,
            ]);
            socket.write(authReq);
            return;
          } else if (method === 0x00) {
            // No auth
            sendConnectRequest();
          } else {
            throw new ConnectError(
              `sesame/rc/netconn: unsupported SOCKS5 auth method: ${method}`,
              Code.PermissionDenied,
            );
          }
        }

        if (stage === 'auth') {
          if (buffer.length < 2) return;
          const subVer = buffer[0];
          const status = buffer[1];
          buffer = buffer.subarray(2);

          if (subVer !== 0x01 || status !== 0x00) {
            throw new ConnectError(
              `sesame/rc/netconn: SOCKS5 auth failed: status ${status}`,
              Code.PermissionDenied,
            );
          }
          sendConnectRequest();
        }

        if (stage === 'connect') {
          // Response: VER (1), REP (1), RSV (1), ATYP (1), BND.ADDR (var), BND.PORT (2)
          if (buffer.length < 4) return;
          const rep = buffer[1];
          const atyp = buffer[3];
          let expectedLen = 4;
          if (atyp === 0x01)
            expectedLen += 4 + 2; // IPv4 + port
          else if (atyp === 0x04)
            expectedLen += 16 + 2; // IPv6 + port
          else if (atyp === 0x03) {
            if (buffer.length < 5) return;
            const domainLen = buffer[4];
            expectedLen += 1 + domainLen + 2;
          }

          if (buffer.length < expectedLen) return;

          if (rep !== 0x00) {
            throw new ConnectError(
              `sesame/rc/netconn: SOCKS5 connect failed: reply code ${rep}`,
              Code.Unavailable,
            );
          }

          const extra = buffer.subarray(expectedLen);
          if (extra.length > 0) {
            // Pause first: unshifted bytes are only re-emitted to a
            // later 'data' listener if the stream is paused when that
            // listener attaches. On a flowing socket the unshift
            // re-enters THIS handler with tunnel payload, which it
            // re-parses as handshake frames - and because `buffer`
            // still holds the reply, Buffer.concat doubles it every
            // re-entry until the runtime's typed-array limit throws
            // (measured: a proxy coalescing early bytes with its
            // CONNECT reply crashed the handshake with RangeError).
            // The server resumes the socket after attaching its relay
            // listener, mirroring the HTTP CONNECT path.
            socket.pause();
            socket.unshift(extra);
          }

          resolved = true;
          clearTimeout(timer);
          cleanup();
          resolve(socket);
        }
      } catch (e) {
        if (resolved) return;
        resolved = true;
        clearTimeout(timer);
        cleanup();
        socket.destroy();
        reject(e);
      }
    };

    const sendConnectRequest = () => {
      stage = 'connect';
      const hp = parseHostPort(target);
      const isIpv4 = net.isIPv4(hp.host);
      const isIpv6 = net.isIPv6(hp.host);
      const header = Buffer.from([0x05, 0x01, 0x00]);
      let addrBuf: Buffer;
      if (isIpv4) {
        const parts = hp.host.split('.').map(p => parseInt(p, 10));
        addrBuf = Buffer.concat([Buffer.from([0x01]), Buffer.from(parts)]);
      } else if (isIpv6) {
        addrBuf = Buffer.concat([Buffer.from([0x04]), ipv6ToBytes(hp.host)]);
      } else {
        const domain = Buffer.from(hp.host, 'utf-8');
        // The ATYP 0x03 length field carries an OCTET count: use the
        // buffer's byte length, not the string's code-unit count
        // (multibyte/IDN domains must not truncate the field).
        addrBuf = Buffer.concat([Buffer.from([0x03, domain.length]), domain]);
      }

      const portBuf = Buffer.alloc(2);
      portBuf.writeUInt16BE(hp.port, 0);

      socket.write(Buffer.concat([header, addrBuf, portBuf]));
    };

    // The guard covers an abort that fired while a previous stage held
    // the only listener: AbortSignal does not replay past events.
    if (signal) {
      if (signal.aborted) {
        onAbort();
        return;
      }
      signal.addEventListener('abort', onAbort, {once: true});
    }

    socket.on('data', onData);
    socket.once('error', onError);

    // Initial greeting
    const hasAuth = !!hop.username;
    const greeting = hasAuth
      ? Buffer.from([0x05, 0x02, 0x00, 0x02])
      : Buffer.from([0x05, 0x01, 0x00]);
    socket.write(greeting);
  });
}

function parseIPv4Octets(literal: string, host: string): number[] {
  const parts = literal.split('.');
  if (parts.length !== 4) {
    throw new Error(`Invalid IPv6 address: ${host}`);
  }
  return parts.map(part => {
    if (!/^\d{1,3}$/.test(part)) {
      throw new Error(`Invalid IPv6 address: ${host}`);
    }
    const octet = parseInt(part, 10);
    if (octet > 255) {
      throw new Error(`Invalid IPv6 address: ${host}`);
    }
    return octet;
  });
}

// Parses an IPv6 literal into its 16 wire-order bytes. `::` expands to exactly
// one run of zero groups, and a trailing dotted-quad occupies the final two
// groups (the IPv4-mapped form, e.g. ::ffff:1.2.3.4).
function ipv6ToBytes(host: string): Buffer {
  const zoneIndex = host.indexOf('%');
  const literal = zoneIndex === -1 ? host : host.slice(0, zoneIndex);
  if (literal.length === 0) {
    throw new Error(`Invalid IPv6 address: ${host}`);
  }

  const compressIndex = literal.indexOf('::');
  if (compressIndex !== literal.lastIndexOf('::')) {
    throw new Error(`Invalid IPv6 address: ${host}`);
  }

  // A dotted-quad may only appear as the address's final group, so it is
  // allowed in the tail of a compressed literal or in a fully written one.
  const parseGroups = (segment: string, allowIPv4: boolean): number[] => {
    if (segment.length === 0) {
      return [];
    }
    const parts = segment.split(':');
    const groups: number[] = [];
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      if (part.includes('.')) {
        if (!allowIPv4 || i !== parts.length - 1) {
          throw new Error(`Invalid IPv6 address: ${host}`);
        }
        const octets = parseIPv4Octets(part, host);
        groups.push((octets[0] << 8) | octets[1], (octets[2] << 8) | octets[3]);
        continue;
      }
      if (!/^[0-9a-fA-F]{1,4}$/.test(part)) {
        throw new Error(`Invalid IPv6 address: ${host}`);
      }
      groups.push(parseInt(part, 16));
    }
    return groups;
  };

  let groups: number[];
  if (compressIndex === -1) {
    groups = parseGroups(literal, true);
  } else {
    const head = parseGroups(literal.slice(0, compressIndex), false);
    const tail = parseGroups(literal.slice(compressIndex + 2), true);
    const zeroCount = 8 - head.length - tail.length;
    if (zeroCount < 1) {
      throw new Error(`Invalid IPv6 address: ${host}`);
    }
    groups = [...head, ...new Array<number>(zeroCount).fill(0), ...tail];
  }

  if (groups.length !== 8) {
    throw new Error(`Invalid IPv6 address: ${host}`);
  }

  const bytes = Buffer.alloc(16);
  for (let i = 0; i < 8; i++) {
    bytes.writeUInt16BE(groups[i], i * 2);
  }
  return bytes;
}
