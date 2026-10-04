import * as net from 'node:net';
import * as tls from 'node:tls';
import {Code, ConnectError} from '@connectrpc/connect';
import {create} from '@bufbuild/protobuf';
import {
  FingerprintPreset,
  TLSHandshakeResult,
  TLSHandshakeResultSchema,
  TLSOptions,
  TLSVersion,
} from '../gen/sesame/type/tls_pb';
import {
  ProxyHop,
  ProxyHop_Type,
  ProxyOptions,
  ProxyResult,
  ProxyResultSchema,
} from '../gen/sesame/type/proxy_pb';
import {NetAddr, NetAddrSchema} from '../gen/sesame/type/netaddr_pb';
import {ServerSecrets} from '../config';

export function protoToTLSVersion(
  v: TLSVersion,
): tls.SecureVersion | undefined {
  switch (v) {
    case TLSVersion.TLS_1_0:
      return 'TLSv1';
    case TLSVersion.TLS_1_1:
      return 'TLSv1.1';
    case TLSVersion.TLS_1_2:
      return 'TLSv1.2';
    case TLSVersion.TLS_1_3:
      return 'TLSv1.3';
    default:
      return undefined;
  }
}

export function tlsVersionToProto(v: string | null | undefined): TLSVersion {
  switch (v) {
    case 'TLSv1':
      return TLSVersion.TLS_1_0;
    case 'TLSv1.1':
      return TLSVersion.TLS_1_1;
    case 'TLSv1.2':
      return TLSVersion.TLS_1_2;
    case 'TLSv1.3':
      return TLSVersion.TLS_1_3;
    default:
      return TLSVersion.TLS_VERSION_UNSPECIFIED;
  }
}

export function parseHostPort(addr: string): {host: string; port: number} {
  const lastColon = addr.lastIndexOf(':');
  if (lastColon === -1) {
    throw new Error(`Invalid host:port address: ${addr}`);
  }
  let host = addr.slice(0, lastColon);
  if (host.startsWith('[') && host.endsWith(']')) {
    host = host.slice(1, -1);
  }
  const port = parseInt(addr.slice(lastColon + 1), 10);
  if (isNaN(port) || port < 1 || port > 65535) {
    throw new Error(`Invalid port in address: ${addr}`);
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
  readonly result: TLSHandshakeResult;
}

export async function executeTLSHandshake(
  socket: net.Socket,
  opts: TLSOptions,
  defaultServerName: string,
  fallbackSecrets?: ServerSecrets,
): Promise<TLSExecutionResult> {
  const preset = opts.fingerprintPreset;
  if (
    preset !== FingerprintPreset.FINGERPRINT_PRESET_UNSPECIFIED &&
    preset !== FingerprintPreset.RUNTIME_DEFAULT
  ) {
    throw new ConnectError(
      `sesame/rc/netconn: requested fingerprint preset ${preset} is not supported by standard runtime; custom TLSProvider required`,
      Code.FailedPrecondition,
    );
  }

  let serverName = opts.serverName || defaultServerName;
  // A bare IPv6 literal is already a valid SNI value; only strip a port when
  // the value is a bracketed literal or a name:port pair.
  if (serverName.includes(':') && !net.isIPv6(serverName)) {
    serverName = parseHostPort(serverName).host;
  }

  const tlsConnectOptions: tls.ConnectionOptions = {
    socket,
    servername: serverName,
    rejectUnauthorized: !opts.insecureSkipVerify,
  };

  // RFC Invariant: If alpn_protocols is empty, ALPN extension MUST NOT be sent.
  if (opts.alpnProtocols && opts.alpnProtocols.length > 0) {
    tlsConnectOptions.ALPNProtocols = [...opts.alpnProtocols];
  }

  const minVer = protoToTLSVersion(opts.minVersion);
  if (minVer) {
    tlsConnectOptions.minVersion = minVer;
  }
  const maxVer = protoToTLSVersion(opts.maxVersion);
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

    const onSecureConnect = () => {
      if (resolved) return;
      resolved = true;
      cleanup();

      const peerCert = tlsSocket.getPeerCertificate(true);
      const rawCert = peerCert?.raw;
      const proto = tlsSocket.alpnProtocol;
      const negotiatedProtocol = typeof proto === 'string' ? proto : '';

      const result = create(TLSHandshakeResultSchema, {
        negotiatedProtocol,
        cipherSuite: 0,
        tlsVersion: tlsVersionToProto(tlsSocket.getProtocol()),
        serverName,
        peerCertificates: rawCert ? [new Uint8Array(rawCert)] : [],
        appliedPreset: FingerprintPreset.RUNTIME_DEFAULT,
      });

      resolve({tlsSocket, result});
    };

    const onError = (err: Error) => {
      if (resolved) return;
      resolved = true;
      cleanup();
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
    };

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
): Promise<ProxyExecutionResult> {
  const hops = proxyOpts?.hops ?? [];
  if (hops.length === 0) {
    const hp = parseHostPort(targetAddress);
    const socket = await dialTcp(hp.host, hp.port, timeoutMs);
    const result = create(ProxyResultSchema, {
      traversedHops: [],
      egressAddress: createNetAddrFromSocket(socket, 'remote'),
    });
    return {socket, result};
  }

  const traversedHops: NetAddr[] = [];
  const firstHop = hops[0];
  if (!firstHop.address?.address) {
    throw new ConnectError(
      'sesame/rc/netconn: first proxy hop missing address',
      Code.InvalidArgument,
    );
  }

  const firstHp = parseHostPort(firstHop.address.address);
  let currentSocket = await dialTcp(firstHp.host, firstHp.port, timeoutMs);
  traversedHops.push(firstHop.address);

  for (let i = 0; i < hops.length; i++) {
    const hop = hops[i];
    let nextAddrStr: string;
    if (i + 1 < hops.length) {
      const nextAddr = hops[i + 1].address;
      if (!nextAddr?.address) {
        currentSocket.destroy();
        throw new ConnectError(
          `sesame/rc/netconn: proxy hop ${i + 1} missing address`,
          Code.InvalidArgument,
        );
      }
      nextAddrStr = nextAddr.address;
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

function dialTcp(
  host: string,
  port: number,
  timeoutMs: number,
): Promise<net.Socket> {
  return new Promise((resolve, reject) => {
    let resolved = false;
    const socket = net.createConnection({host, port});

    const timer = setTimeout(() => {
      if (resolved) return;
      resolved = true;
      socket.destroy();
      reject(
        new ConnectError(
          `sesame/rc/netconn: dial timeout to ${host}:${port}`,
          Code.DeadlineExceeded,
        ),
      );
    }, timeoutMs);

    const onConnect = () => {
      if (resolved) return;
      resolved = true;
      clearTimeout(timer);
      socket.removeListener('error', onError);
      resolve(socket);
    };

    const onError = (err: Error) => {
      if (resolved) return;
      resolved = true;
      clearTimeout(timer);
      socket.removeListener('connect', onConnect);
      reject(
        new ConnectError(
          `sesame/rc/netconn: dial error to ${host}:${port}: ${err.message}`,
          Code.Unavailable,
        ),
      );
    };

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

        // Unshift any remaining data back to socket
        const extra = buffer.subarray(headerEnd + 4);
        if (extra.length > 0) {
          socket.destroy();
          reject(
            new ConnectError(
              `sesame/rc/netconn: HTTP CONNECT to ${target} returned unexpected bytes after headers`,
              Code.Unavailable,
            ),
          );
          return;
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
    };

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
        addrBuf = Buffer.concat([Buffer.from([0x03, domain.length]), domain]);
      }

      const portBuf = Buffer.alloc(2);
      portBuf.writeUInt16BE(hp.port, 0);

      socket.write(Buffer.concat([header, addrBuf, portBuf]));
    };

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
