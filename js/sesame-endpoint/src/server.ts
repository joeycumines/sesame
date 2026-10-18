import * as http2 from 'node:http2';
import * as fs from 'node:fs';
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

// Resolves operator-provided TLS material to PEM bytes. Literal PEM content
// is used as-is; anything else is a file path, read eagerly so a missing,
// empty, or non-PEM file fails here with a clear error instead of surfacing
// as an opaque handshake failure - or worse, silently disabling the TLS
// listener.
function loadPemMaterial(val: string, name: string): Buffer {
  if (val.trim().length === 0) {
    throw new Error(`${name} must not be empty`);
  }
  const content = val.includes('-----BEGIN')
    ? val
    : fs.readFileSync(val, 'utf8');
  if (content.trim().length === 0) {
    throw new Error(`${name} is empty: ${val}`);
  }
  if (!content.includes('-----BEGIN')) {
    throw new Error(`Invalid PEM content for ${name}: ${val}`);
  }
  return Buffer.from(content, 'utf8');
}

export function createEndpointServer(
  config: ServerConfig,
  tlsProvider?: TLSProvider,
): EndpointServer {
  const routes = (router: ConnectRouter) => {
    router.service(
      RemoteControl,
      createRemoteControlService(config, tlsProvider),
    );
  };

  const handler = connectNodeAdapter({routes});

  const {tlsCert, tlsKey, clientCaCert} = config.secrets;

  // Defined-ness (not truthiness) decides whether the TLS listener is
  // requested: a defined-but-empty field fails loudly here instead of
  // silently falling through to the plaintext listener.
  if ((tlsCert === undefined) !== (tlsKey === undefined)) {
    throw new Error(
      'Both tlsCert and tlsKey must be provided to enable the TLS listener',
    );
  }
  if (clientCaCert !== undefined && tlsCert === undefined) {
    throw new Error(
      'clientCaCert requires both tlsCert and tlsKey to be configured',
    );
  }

  let server: http2.Http2Server | http2.Http2SecureServer;

  if (tlsCert !== undefined && tlsKey !== undefined) {
    const secureOptions: http2.SecureServerOptions = {
      cert: loadPemMaterial(tlsCert, 'TLS certificate'),
      key: loadPemMaterial(tlsKey, 'TLS key'),
    };
    if (clientCaCert !== undefined) {
      secureOptions.ca = loadPemMaterial(clientCaCert, 'Client CA certificate');
      secureOptions.requestCert = true;
      secureOptions.rejectUnauthorized = true;
    }

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
