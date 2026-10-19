import {describe, it, expect, afterAll} from 'bun:test';
import * as http2 from 'node:http2';
import * as fs from 'node:fs';
import * as path from 'node:path';
import {parseConfig} from '../src/config';
import {createEndpointServer, type EndpointServer} from '../src/server';

const FIXTURES_DIR = fs.existsSync(path.join(__dirname, 'fixtures'))
  ? path.join(__dirname, 'fixtures')
  : path.join(__dirname, '../../test/fixtures');
const SERVER_CERT = path.join(FIXTURES_DIR, 'cert.pem');
const SERVER_KEY = path.join(FIXTURES_DIR, 'key.pem');

// Performs one HTTP/2 request against the given listener and resolves the
// response status (or null on connection/protocol failure).
function requestStatus(
  port: number,
  headers: Record<string, string>,
  options: http2.ClientSessionOptions = {},
  timeoutMs = 5000,
): Promise<number | null> {
  const scheme = 'rejectUnauthorized' in options ? 'https' : 'http';
  return new Promise(resolve => {
    const client = http2.connect(`${scheme}://127.0.0.1:${port}`, options);
    let settled = false;
    const finish = (status: number | null) => {
      if (!settled) {
        settled = true;
        resolve(status);
        client.destroy();
      }
    };
    const timer = setTimeout(() => finish(null), timeoutMs);
    client.on('error', () => finish(null));
    const req = client.request({':path': '/', ...headers});
    req.on('error', () => finish(null));
    req.on('response', res => finish(res[':status'] ?? null));
    req.on('close', () => finish(null));
    req.end();
    clearTimeout(timer);
  });
}

describe('sesame-endpoint inbound bearer token (SESAME_ENDPOINT_TOKEN)', () => {
  const servers: EndpointServer[] = [];

  async function startServer(
    env: Record<string, string | undefined>,
  ): Promise<number> {
    const res = parseConfig([], env);
    const server = createEndpointServer(res.config!);
    servers.push(server);
    const {port} = await server.listen(0, '127.0.0.1');
    return port;
  }

  afterAll(async () => {
    for (const server of servers) {
      await server.close();
    }
  });

  it('parses the token into secrets and leaves it unset when absent', () => {
    const withToken = parseConfig([], {SESAME_ENDPOINT_TOKEN: '  abc123  '});
    expect(withToken.config!.secrets.authToken).toBe('abc123');

    const without = parseConfig([], {});
    expect(without.config!.secrets.authToken).toBeUndefined();
  });

  it('fails closed on a present-but-blank token', () => {
    expect(() => parseConfig([], {SESAME_ENDPOINT_TOKEN: '   '})).toThrow(
      /SESAME_ENDPOINT_TOKEN must not be blank/,
    );
  });

  it('rejects requests without, or with a wrong, bearer token', async () => {
    const port = await startServer({SESAME_ENDPOINT_TOKEN: 'secret-token'});

    const noHeader = await requestStatus(port, {});
    expect(noHeader).toBe(401);

    const wrongToken = await requestStatus(port, {
      authorization: 'Bearer wrong-token',
    });
    expect(wrongToken).toBe(401);

    const bareToken = await requestStatus(port, {
      authorization: 'secret-token',
    });
    expect(bareToken).toBe(401);

    // The correct bearer passes the gate; the adapter's own answer for an
    // unknown path (a 4xx, typically 404) proves the request was routed
    // instead of refused.
    const correct = await requestStatus(port, {
      authorization: 'Bearer secret-token',
    });
    expect(correct).not.toBe(401);
    expect(correct).not.toBeNull();
  });

  it('enforces the token on the TLS listener too', async () => {
    const port = await startServer({
      SESAME_ENDPOINT_TOKEN: 'tls-token',
      SESAME_ENDPOINT_TLS_CERT: SERVER_CERT,
      SESAME_ENDPOINT_TLS_KEY: SERVER_KEY,
    });
    const tls: http2.SecureClientSessionOptions = {
      rejectUnauthorized: false,
    };

    const wrong = await requestStatus(
      port,
      {authorization: 'Bearer nope'},
      tls,
    );
    expect(wrong).toBe(401);

    const correct = await requestStatus(
      port,
      {authorization: 'Bearer tls-token'},
      tls,
    );
    expect(correct).not.toBe(401);
    expect(correct).not.toBeNull();
  });

  it('leaves requests unauthenticated-but-served when no token is configured', async () => {
    const port = await startServer({});
    const status = await requestStatus(port, {});
    expect(status).not.toBe(401);
    expect(status).not.toBeNull();
  });
});
