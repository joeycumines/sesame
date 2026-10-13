import {describe, expect, it} from 'bun:test';
import * as net from 'node:net';
import {
  executeProxyHops,
  executeTLSHandshake,
  parseHostPort,
} from '../src/rc/transform';
import {ConnectError, Code} from '@connectrpc/connect';
import {create} from '@bufbuild/protobuf';
import {
  ProxyHop,
  ProxyHop_Type,
  ProxyHopSchema,
  ProxyOptionsSchema,
} from '../src/gen/sesame/proxy/v1alpha1/proxy_pb';
import {NetAddr, NetAddrSchema} from '../src/gen/sesame/type/netaddr_pb';
import {TLSOptionsSchema} from '../src/gen/sesame/tls/v1alpha1/tls_pb';

describe('parseHostPort', () => {
  it('parses a host:port pair', () => {
    expect(parseHostPort('example.com:443')).toEqual({
      host: 'example.com',
      port: 443,
    });
  });

  it('parses an IPv4 literal with port', () => {
    expect(parseHostPort('127.0.0.1:8080')).toEqual({
      host: '127.0.0.1',
      port: 8080,
    });
  });

  it('parses a bracketed IPv6 literal with port', () => {
    expect(parseHostPort('[::1]:9090')).toEqual({host: '::1', port: 9090});
    expect(parseHostPort('[2001:db8::1]:443')).toEqual({
      host: '2001:db8::1',
      port: 443,
    });
  });

  it('rejects a bare IPv6 literal with a clear error', () => {
    let err: unknown;
    try {
      parseHostPort('::1');
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    const ce = err as ConnectError;
    expect(ce.code).toBe(Code.InvalidArgument);
    expect(ce.message).toContain('must specify a port');
  });

  it('rejects an address with no port with InvalidArgument', () => {
    let err: unknown;
    try {
      parseHostPort('example.com');
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.InvalidArgument);
    expect((err as ConnectError).message).toContain('Invalid host:port');
  });

  it('rejects an invalid port with InvalidArgument', () => {
    // Trailing garbage, signs, whitespace, empty ports, and out-of-range
    // values are all malformed input; parseInt previously tolerated
    // some of these and dialed the wrong port.
    for (const addr of [
      'host:notaport',
      'host:80x',
      'host:+80',
      'host: 80',
      'host:',
      'host:0',
      'host:65536',
    ]) {
      let err: unknown;
      try {
        parseHostPort(addr);
      } catch (e: unknown) {
        err = e;
      }
      expect(err).toBeInstanceOf(ConnectError);
      expect((err as ConnectError).code).toBe(Code.InvalidArgument);
      expect((err as ConnectError).message).toContain('Invalid port');
    }
  });

  it('rejects an unbracketed multi-colon address with InvalidArgument', () => {
    // The Go reference (net.SplitHostPort) rejects "too many colons",
    // "missing ']'", and "unexpected ']'"; splitting at the last colon
    // would otherwise silently mis-parse.
    for (const addr of [
      'host:80:90',
      '::1:443',
      '[::1]x:443',
      '[foo:80',
      'x]:80',
      ']:80',
    ]) {
      let err: unknown;
      try {
        parseHostPort(addr);
      } catch (e: unknown) {
        err = e;
      }
      expect(err).toBeInstanceOf(ConnectError);
      expect((err as ConnectError).code).toBe(Code.InvalidArgument);
    }
  });
});

describe('executeProxyHops credential and address validation', () => {
  // A structural init shape (not Partial<ProxyHop>, whose $typeName
  // marker key is incompatible with create()'s MessageInit parameter).
  const hopWith = (
    mod: {
      type?: ProxyHop_Type;
      address?: NetAddr;
      username?: string;
      password?: string;
      authHeader?: string;
    } = {},
  ): ProxyHop =>
    create(ProxyHopSchema, {
      type: ProxyHop_Type.HTTP_CONNECT,
      address: create(NetAddrSchema, {
        network: 'tcp',
        address: 'proxy.internal:8080',
      }),
      ...mod,
    });

  it('rejects a password with no username on a SOCKS5 hop', async () => {
    // The SOCKS5 builder negotiates RFC 1929 auth only when a username
    // is set, so a password-only hop would silently negotiate NO-AUTH.
    let err: unknown;
    try {
      await executeProxyHops(
        'tcp',
        'target.internal:443',
        create(ProxyOptionsSchema, {
          hops: [hopWith({type: ProxyHop_Type.SOCKS5, password: 'orphaned'})],
        }),
      );
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.InvalidArgument);
    expect((err as ConnectError).message).toContain('password but no username');
  });

  it('rejects a password with no username on an HTTP CONNECT hop', async () => {
    // The CONNECT builder has the same username gate for basic auth.
    let err: unknown;
    try {
      await executeProxyHops(
        'tcp',
        'target.internal:443',
        create(ProxyOptionsSchema, {
          hops: [hopWith({password: 'orphaned'})],
        }),
      );
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.InvalidArgument);
    expect((err as ConnectError).message).toContain('password but no username');
  });

  it('accepts a username with an empty password', async () => {
    // RFC 1929 allows an empty password; validation passes and the dial
    // stage is reached (the unresolvable host yields a
    // non-InvalidArgument dial error).
    let err: unknown;
    try {
      await executeProxyHops(
        'tcp',
        'target.internal:443',
        create(ProxyOptionsSchema, {
          hops: [hopWith({type: ProxyHop_Type.SOCKS5, username: 'user'})],
        }),
      );
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).not.toBe(Code.InvalidArgument);
  });

  it('rejects a malformed target address before any dialing', async () => {
    for (const target of ['example.com', '::1', 'host:80:90', 'host:0']) {
      let err: unknown;
      try {
        await executeProxyHops(
          'tcp',
          target,
          create(ProxyOptionsSchema, {hops: [hopWith({})]}),
        );
      } catch (e: unknown) {
        err = e;
      }
      expect(err).toBeInstanceOf(ConnectError);
      expect((err as ConnectError).code).toBe(Code.InvalidArgument);
    }
  });

  it('rejects a malformed hop address before any dialing, naming the hop', async () => {
    // A malformed address on a multi-hop chain names the offending hop
    // index, matching the Go reference's per-hop error.
    let err: unknown;
    try {
      await executeProxyHops(
        'tcp',
        'target.internal:443',
        create(ProxyOptionsSchema, {
          hops: [
            hopWith(),
            hopWith({
              address: create(NetAddrSchema, {
                network: 'tcp',
                address: 'proxy.internal',
              }),
            }),
          ],
        }),
      );
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.InvalidArgument);
    expect((err as ConnectError).message).toContain(
      'invalid proxy hop 1 address',
    );
  });

  it('reports malformed input over a concurrent abort, matching the Go reference', async () => {
    // A request that is both malformed and aborted reports the
    // malformed input (InvalidArgument), not the cancellation; only a
    // well-formed aborted request surfaces Canceled.
    const ctrl = new AbortController();
    ctrl.abort();
    let err: unknown;
    try {
      await executeProxyHops(
        'tcp',
        'target.internal:443',
        create(ProxyOptionsSchema, {
          hops: [hopWith({password: 'orphaned'})],
        }),
        10000,
        undefined,
        ctrl.signal,
      );
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.InvalidArgument);
    expect((err as ConnectError).message).toContain('password but no username');
  });

  it('rejects a missing middle hop address before any dialing', async () => {
    // The upfront check fires before the first hop's dial; reaching the
    // dial would surface hop 0's DNS error (Unavailable) instead.
    let err: unknown;
    try {
      await executeProxyHops(
        'tcp',
        'target.internal:443',
        create(ProxyOptionsSchema, {
          hops: [
            hopWith(),
            create(ProxyHopSchema, {type: ProxyHop_Type.HTTP_CONNECT}),
          ],
        }),
      );
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.InvalidArgument);
    expect((err as ConnectError).message).toContain(
      'proxy hop 1 missing address',
    );
  });
});

describe('executeProxyHops abort wiring', () => {
  // An aborted request must stop the dial promptly instead of running
  // to its own timeout; before the abort wiring the request signal was
  // not passed to executeProxyHops at all.
  const hop = () =>
    create(ProxyHopSchema, {
      type: ProxyHop_Type.HTTP_CONNECT,
      address: create(NetAddrSchema, {
        network: 'tcp',
        address: '10.255.255.1:81',
      }),
    });

  it('rejects an already-aborted signal before dialing', async () => {
    const ctrl = new AbortController();
    ctrl.abort();
    let err: unknown;
    try {
      await executeProxyHops(
        'tcp',
        'target.internal:443',
        create(ProxyOptionsSchema, {hops: [hop()]}),
        10000,
        undefined,
        ctrl.signal,
      );
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.Canceled);
    expect((err as ConnectError).message).toContain('aborted by client');
  });

  it('stops an in-flight dial when the signal aborts', async () => {
    // 10.255.255.1 is non-routable: nothing can connect or error before
    // the abort fires, so the rejection is deterministic.
    const ctrl = new AbortController();
    const pending = executeProxyHops(
      'tcp',
      'target.internal:443',
      create(ProxyOptionsSchema, {hops: [hop()]}),
      10000,
      undefined,
      ctrl.signal,
    );
    ctrl.abort();
    let err: unknown;
    try {
      await pending;
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.Canceled);
  });
});

describe('executeTLSHandshake abort and timeout wiring', () => {
  it('stops an in-flight handshake when the signal aborts', async () => {
    // A TCP server that accepts and never replies: the TLS handshake
    // cannot complete, so the abort is the only way it settles. The
    // server-side sockets are tracked and destroyed before close, or
    // close's callback never fires (a discarded connection never ends).
    const conns: net.Socket[] = [];
    const silent = net.createServer(sock => {
      conns.push(sock);
    });
    await new Promise<void>(r => silent.listen(0, '127.0.0.1', () => r()));
    const port = (silent.address() as net.AddressInfo).port;

    const socket = net.createConnection({host: '127.0.0.1', port});
    await new Promise<void>(r => socket.once('connect', r));

    const ctrl = new AbortController();
    const pending = executeTLSHandshake(
      socket,
      create(TLSOptionsSchema, {serverName: 'example.com'}),
      `127.0.0.1:${port}`,
      undefined,
      undefined,
      ctrl.signal,
    );
    ctrl.abort();
    let err: unknown;
    try {
      await pending;
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.Canceled);
    expect((err as ConnectError).message).toContain('aborted by client');
    socket.destroy();
    for (const c of conns) {
      c.destroy();
    }
    await new Promise<void>(r => silent.close(() => r()));
  });

  it('fails a stalled handshake with DeadlineExceeded instead of hanging', async () => {
    // A server that accepts TCP but never answers the ClientHello: the
    // handshake timeout must settle the promise instead of waiting for
    // the client to give up.
    const conns: net.Socket[] = [];
    const silent = net.createServer(sock => {
      conns.push(sock);
    });
    await new Promise<void>(r => silent.listen(0, '127.0.0.1', () => r()));
    const port = (silent.address() as net.AddressInfo).port;

    const socket = net.createConnection({host: '127.0.0.1', port});
    await new Promise<void>(r => socket.once('connect', r));

    let err: unknown;
    try {
      await executeTLSHandshake(
        socket,
        create(TLSOptionsSchema, {serverName: 'example.com'}),
        `127.0.0.1:${port}`,
        undefined,
        undefined,
        undefined,
        100,
      );
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.DeadlineExceeded);
    socket.destroy();
    for (const c of conns) {
      c.destroy();
    }
    await new Promise<void>(r => silent.close(() => r()));
  });
});

describe('executeProxyHops control-byte rejection', () => {
  // CR/LF in a proxy handshake field would inject header or request
  // lines into the raw CONNECT request; other control bytes are garbage
  // in an authority or header value. Validation must fire before any
  // dialing, so no server is needed for the rejection cases.
  const validHop = (mod?: (h: ProxyHop) => void) => {
    const hop = create(ProxyHopSchema, {
      type: ProxyHop_Type.HTTP_CONNECT,
      address: create(NetAddrSchema, {
        network: 'tcp',
        address: 'proxy.internal:8080',
      }),
    });
    if (mod) mod(hop);
    return hop;
  };

  const expectInvalidArgument = async (
    target: string,
    hops: ProxyHop[],
    messageFragment = 'control byte',
  ): Promise<void> => {
    try {
      await executeProxyHops('tcp', target, create(ProxyOptionsSchema, {hops}));
    } catch (err: unknown) {
      expect(err).toBeInstanceOf(ConnectError);
      const ce = err as ConnectError;
      expect(ce.code).toBe(Code.InvalidArgument);
      expect(ce.message).toContain(messageFragment);
      return;
    }
    throw new Error(
      `expected InvalidArgument (${messageFragment}) for a proxy field`,
    );
  };

  it('rejects CR/LF injected into the CONNECT target', async () => {
    await expectInvalidArgument('target.internal:443\r\nHost: evil', [
      validHop(),
    ]);
  });

  it('rejects NUL and DEL in the target', async () => {
    await expectInvalidArgument('target\x00.internal:443', [validHop()]);
    await expectInvalidArgument('target.internal:443\x7f', [validHop()]);
  });

  it('rejects a control byte in a hop address', async () => {
    await expectInvalidArgument('target.internal:443', [
      validHop(h => {
        h.address = create(NetAddrSchema, {
          network: 'tcp',
          address: 'proxy\r.internal:8080',
        });
      }),
    ]);
  });

  it('rejects a control byte in auth_header', async () => {
    await expectInvalidArgument('target.internal:443', [
      validHop(h => {
        h.authHeader = 'Bearer tok\r\nX-Injected: 1';
      }),
    ]);
  });

  it('rejects a control byte in a later hop address (the next CONNECT target)', async () => {
    await expectInvalidArgument('target.internal:443', [
      validHop(),
      validHop(h => {
        h.address = create(NetAddrSchema, {
          network: 'tcp',
          address: 'proxy2.\ninternal:8080',
        });
      }),
    ]);
  });

  it('lets clean fields through validation', async () => {
    // Clean fields must reach the dial stage: proxy.internal does not
    // resolve, so a non-InvalidArgument dial failure is the proof that
    // validation did not over-reject.
    let err: unknown;
    try {
      await executeProxyHops(
        'tcp',
        'target.internal:443',
        create(ProxyOptionsSchema, {
          hops: [validHop(h => (h.authHeader = 'Bearer valid-token'))],
        }),
      );
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    const ce = err as ConnectError;
    expect(ce.code).not.toBe(Code.InvalidArgument);
  });

  it('rejects a SOCKS5 target domain longer than the 255-byte u8 field', async () => {
    // RFC 1928 ATYP 0x03 carries the domain behind a u8 length: a
    // >255-byte host cannot be encoded and previously wrapped the
    // length byte. Rejection must happen before any dialing.
    const oversized = 'a'.repeat(256) + '.example:443';
    await expectInvalidArgument(
      oversized,
      [
        create(ProxyHopSchema, {
          type: ProxyHop_Type.SOCKS5,
          address: create(NetAddrSchema, {
            network: 'tcp',
            address: 'socks.internal:1080',
          }),
        }),
      ],
      '255-byte limit',
    );
  });

  it('counts bytes, not code units, for the SOCKS5 255-byte limit', async () => {
    // A multibyte (IDN-style) domain of 200 characters is 400 UTF-8
    // bytes: it fits the 255 limit only when (wrongly) counted in
    // UTF-16 code units. Both stacks must reject on bytes.
    const multibyte = 'é'.repeat(200) + ':443';
    await expectInvalidArgument(
      multibyte,
      [
        create(ProxyHopSchema, {
          type: ProxyHop_Type.SOCKS5,
          address: create(NetAddrSchema, {
            network: 'tcp',
            address: 'socks.internal:1080',
          }),
        }),
      ],
      '255-byte limit',
    );
  });

  it('lets the same oversized domain through an HTTP CONNECT hop', async () => {
    // HTTP CONNECT interpolates the target into a raw request line -
    // no length field - so the oversized domain must pass validation
    // and reach the dial stage (non-InvalidArgument failure).
    const oversized = 'a'.repeat(256) + '.example:443';
    let err: unknown;
    try {
      await executeProxyHops(
        'tcp',
        oversized,
        create(ProxyOptionsSchema, {
          hops: [
            create(ProxyHopSchema, {
              type: ProxyHop_Type.HTTP_CONNECT,
              address: create(NetAddrSchema, {
                network: 'tcp',
                address: 'proxy.internal:8080',
              }),
            }),
          ],
        }),
      );
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).not.toBe(Code.InvalidArgument);
  });

  it('accepts a SOCKS5 target domain of exactly 255 bytes', async () => {
    // The boundary must pass validation and reach the dial stage.
    const exact255 = 'a'.repeat(250) + '.test:443';
    let err: unknown;
    try {
      await executeProxyHops(
        'tcp',
        exact255,
        create(ProxyOptionsSchema, {
          hops: [
            create(ProxyHopSchema, {
              type: ProxyHop_Type.SOCKS5,
              address: create(NetAddrSchema, {
                network: 'tcp',
                address: 'socks.internal:1080',
              }),
            }),
          ],
        }),
      );
    } catch (e: unknown) {
      err = e;
    }
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).not.toBe(Code.InvalidArgument);
  });

  it("rejects an oversized NEXT-hop address as a SOCKS5 hop's target", async () => {
    // Chained case: hop 0 is SOCKS5, so its CONNECT target is hop 1's
    // address - oversized there must be rejected before any dialing.
    const oversized = 'a'.repeat(256) + '.example:443';
    await expectInvalidArgument(
      'target.internal:443',
      [
        create(ProxyHopSchema, {
          type: ProxyHop_Type.SOCKS5,
          address: create(NetAddrSchema, {
            network: 'tcp',
            address: 'socks.internal:1080',
          }),
        }),
        create(ProxyHopSchema, {
          type: ProxyHop_Type.SOCKS5,
          address: create(NetAddrSchema, {
            network: 'tcp',
            address: oversized,
          }),
        }),
      ],
      '255-byte limit',
    );
  });
});
