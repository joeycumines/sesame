import * as http2 from 'node:http2';
import * as fs from 'node:fs';
import {ConnectRouter} from '@connectrpc/connect';
import {connectNodeAdapter} from '@connectrpc/connect-node';
import {RemoteControl} from './gen/sesame/v1alpha1/remotecontrol_pb';
import {createRemoteControlService} from './rc/server';
import {ServerConfig} from './config';

export interface EndpointServer {
  readonly server: http2.Http2Server | http2.Http2SecureServer;
  listen(port?: number, host?: string): Promise<{host: string; port: number}>;
  close(): Promise<void>;
}

export function createEndpointServer(config: ServerConfig): EndpointServer {
  const routes = (router: ConnectRouter) => {
    router.service(RemoteControl, createRemoteControlService(config));
  };

  const handler = connectNodeAdapter({routes});

  let server: http2.Http2Server | http2.Http2SecureServer;

  if (config.secrets.tlsCert && config.secrets.tlsKey) {
    const cert = fs.existsSync(config.secrets.tlsCert)
      ? fs.readFileSync(config.secrets.tlsCert)
      : Buffer.from(config.secrets.tlsCert);
    const key = fs.existsSync(config.secrets.tlsKey)
      ? fs.readFileSync(config.secrets.tlsKey)
      : Buffer.from(config.secrets.tlsKey);

    server = http2.createSecureServer({cert, key}, handler);
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
