import {describe, expect, it} from 'bun:test';
import {executeProxyHops, parseHostPort} from '../src/rc/transform';
import {ConnectError, Code} from '@connectrpc/connect';
import {create} from '@bufbuild/protobuf';
import {
  ProxyHop,
  ProxyHop_Type,
  ProxyHopSchema,
  ProxyOptionsSchema,
} from '../src/gen/sesame/proxy/v1alpha1/proxy_pb';
import {NetAddrSchema} from '../src/gen/sesame/type/netaddr_pb';

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

  it('rejects an address with no port', () => {
    expect(() => parseHostPort('example.com')).toThrow(/Invalid host:port/);
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
  ): Promise<void> => {
    try {
      await executeProxyHops('tcp', target, create(ProxyOptionsSchema, {hops}));
    } catch (err: unknown) {
      expect(err).toBeInstanceOf(ConnectError);
      const ce = err as ConnectError;
      expect(ce.code).toBe(Code.InvalidArgument);
      expect(ce.message).toContain('control byte');
      return;
    }
    throw new Error('expected InvalidArgument for a control-byte field');
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
});
