import * as http2 from 'node:http2';
import * as fs from 'node:fs';
import * as tls from 'node:tls';
import * as crypto from 'node:crypto';
import {ConnectRouter} from '@connectrpc/connect';
import {connectNodeAdapter} from '@connectrpc/connect-node';
import {RemoteControl} from './gen/sesame/v1alpha1/remotecontrol_pb';
import {createRemoteControlService} from './rc/server';
import {TLSProvider} from './rc/transform';
import {ServerConfig} from './config';

export interface EndpointServer {
  readonly server: http2.Http2Server | http2.Http2SecureServer;
  listen(port?: number, host?: string): Promise<{host: string; port: number}>;
  close(): Promise<void>;
}

// Returns true when the value looks like PEM content rather than a path.
function isPemLiteral(val: string): boolean {
  const trimmed = val.trim();
  return trimmed.startsWith('-----BEGIN ') && trimmed.includes('-----END ');
}

// Resolves operator-provided TLS material to PEM bytes. Literal PEM content
// is used as-is; anything else is a file path, read eagerly so a missing,
// empty, or non-PEM file fails here with a clear error instead of surfacing
// as an opaque handshake failure - or worse, silently disabling the TLS
// listener.
function loadPemMaterial(val: string, name: string): Buffer {
  if (val.trim().length === 0) {
    throw new Error(`${name} must not be empty`);
  }
  if (isPemLiteral(val)) {
    return Buffer.from(val, 'utf8');
  }
  if (
    val.includes('\n') ||
    val.includes('\r') ||
    val.includes('\0') ||
    val.includes('-----END')
  ) {
    throw new Error(`Invalid PEM content for ${name}`);
  }
  const content = fs.readFileSync(val, 'utf8');
  if (content.trim().length === 0) {
    throw new Error(`${name} is empty: ${val}`);
  }
  if (!isPemLiteral(content)) {
    throw new Error(`Invalid PEM content for ${name}: ${val}`);
  }
  return Buffer.from(content, 'utf8');
}

// requireBearerAuth wraps the RPC request handler with a constant-time
// bearer-token check when an endpoint token is configured
// (SESAME_ENDPOINT_TOKEN). Unset means no application-layer enforcement: mTLS
// stays the channel authentication. The check runs on every request the
// listener answers regardless of RPC protocol (gRPC, grpc-web, or connect),
// and the 401 body is a minimal well-formed JSON refusal. timingSafeEqual
// throws on length mismatch, so lengths are compared first and both branches
// run in time independent of the provided value's content.
function requireBearerAuth(
  next: (req: http2.Http2ServerRequest, res: http2.Http2ServerResponse) => void,
  token: string | undefined,
): (req: http2.Http2ServerRequest, res: http2.Http2ServerResponse) => void {
  if (token === undefined) {
    return next;
  }
  const expected = Buffer.from(`Bearer ${token}`, 'utf8');
  return (req, res) => {
    const provided = Buffer.from(
      String(req.headers.authorization ?? ''),
      'utf8',
    );
    const matches =
      provided.length === expected.length &&
      crypto.timingSafeEqual(provided, expected);
    if (!matches) {
      res.writeHead(401, {'content-type': 'application/json'});
      res.end(
        JSON.stringify({error: {message: 'missing or invalid bearer token'}}),
      );
      return;
    }
    next(req, res);
  };
}

export function createEndpointServer(
  config: ServerConfig,
  tlsProvider?: TLSProvider,
): EndpointServer {
  const {tlsCert, tlsKey, clientCaCert} = config.secrets;

  // Validate TLS configuration order matching config.ts: clientCaCert
  // requires both halves; then both halves must be provided together.
  if (
    clientCaCert !== undefined &&
    (tlsCert === undefined || tlsKey === undefined)
  ) {
    throw new Error(
      'clientCaCert requires both tlsCert and tlsKey to be configured',
    );
  }

  // Defined-ness (not truthiness) decides whether the TLS listener is
  // requested: a defined-but-empty field fails loudly here instead of
  // silently falling through to the plaintext listener.
  if ((tlsCert === undefined) !== (tlsKey === undefined)) {
    throw new Error(
      'Both tlsCert and tlsKey must be provided to enable the TLS listener',
    );
  }

  const routes = (router: ConnectRouter) => {
    router.service(
      RemoteControl,
      createRemoteControlService(config, tlsProvider),
    );
  };
  const handler = requireBearerAuth(
    connectNodeAdapter({routes}),
    config.secrets.authToken,
  );

  let server: http2.Http2Server | http2.Http2SecureServer;

  if (tlsCert !== undefined && tlsKey !== undefined) {
    const cert = loadPemMaterial(tlsCert, 'TLS certificate');
    const key = loadPemMaterial(tlsKey, 'TLS key');
    const secureOptions: http2.SecureServerOptions = {cert, key};
    if (clientCaCert !== undefined) {
      secureOptions.ca = loadPemMaterial(clientCaCert, 'Client CA certificate');
      secureOptions.requestCert = true;
      secureOptions.rejectUnauthorized = true;
    }

    // Force synchronous OpenSSL/BoringSSL context initialization so that
    // malformed PEM or mismatched key material fails synchronously inside
    // createEndpointServer under both Bun and Node.js runtimes.
    const secureContext = tls.createSecureContext(secureOptions);
    secureOptions.secureContext = secureContext;

    server = http2.createSecureServer(secureOptions, handler);
  } else {
    server = http2.createServer(handler);
  }

  return {
    server,
    listen(
      port = config.port,
      host = config.host,
    ): Promise<{host: string; port: number}> {
      return new Promise((resolve, reject) => {
        server.listen(port, host, () => {
          const addr = server.address();
          if (addr && typeof addr === 'object') {
            resolve({host: addr.address, port: addr.port});
          } else {
            resolve({host, port});
          }
        });
        server.once('error', reject);
      });
    },
    close(): Promise<void> {
      return new Promise((resolve, reject) => {
        server.close(err => {
          if (err) reject(err);
          else resolve();
        });
      });
    },
  };
}
