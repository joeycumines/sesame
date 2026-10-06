import {describe, expect, it} from 'bun:test';
import {parseHostPort} from '../src/rc/transform';
import {ConnectError} from '@connectrpc/connect';
import {Code} from '@connectrpc/connect';

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
