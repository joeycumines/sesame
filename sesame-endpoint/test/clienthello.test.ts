import {describe, expect, it} from 'bun:test';
import {create} from '@bufbuild/protobuf';
import {Code, ConnectError} from '@connectrpc/connect';
import {
  ClientHelloSpecSchema,
  TLSVersion,
} from '../src/gen/sesame/tls/v1alpha1/tls_pb';
import {
  validateClientHelloSpec,
  assertBuiltinClientHelloHonorable,
  builtinClientHelloCapabilities,
} from '../src/rc/transform';

describe('validateClientHelloSpec', () => {
  it('accepts undefined spec', () => {
    expect(() => validateClientHelloSpec(undefined)).not.toThrow();
  });

  it('accepts empty/default spec', () => {
    const spec = create(ClientHelloSpecSchema, {});
    expect(() => validateClientHelloSpec(spec)).not.toThrow();
  });

  it('accepts valid cipher suites including GREASE', () => {
    const spec = create(ClientHelloSpecSchema, {
      cipherSuites: [0x1301, 0xc02b, 0x0a0a],
    });
    expect(() => validateClientHelloSpec(spec)).not.toThrow();
  });

  it('accepts boundary values 0 and 65535', () => {
    const spec = create(ClientHelloSpecSchema, {
      cipherSuites: [0, 65535],
      supportedGroups: [0, 65535],
      signatureAlgorithms: [0, 65535],
    });
    expect(() => validateClientHelloSpec(spec)).not.toThrow();
  });

  it('rejects cipher suite above uint16', () => {
    const spec = create(ClientHelloSpecSchema, {
      cipherSuites: [0x1301, 1 << 20],
    });
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
    try {
      validateClientHelloSpec(spec);
    } catch (err) {
      expect((err as ConnectError).code).toBe(Code.InvalidArgument);
      expect((err as ConnectError).message).toContain('cipher_suites');
    }
  });

  it('rejects negative cipher suite', () => {
    const spec = create(ClientHelloSpecSchema, {cipherSuites: [-1]});
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
  });

  it('rejects supported group above uint16', () => {
    const spec = create(ClientHelloSpecSchema, {supportedGroups: [70000]});
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
  });

  it('rejects negative supported group', () => {
    const spec = create(ClientHelloSpecSchema, {supportedGroups: [-29]});
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
  });

  it('rejects signature algorithm above uint16', () => {
    const spec = create(ClientHelloSpecSchema, {
      signatureAlgorithms: [1 << 16],
    });
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
  });

  it('rejects negative signature algorithm', () => {
    const spec = create(ClientHelloSpecSchema, {signatureAlgorithms: [-1]});
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
  });

  it('rejects extension type above uint16', () => {
    const spec = create(ClientHelloSpecSchema, {
      extensions: [{type: 1 << 16, body: undefined}],
    });
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
  });

  it('rejects negative extension type', () => {
    const spec = create(ClientHelloSpecSchema, {
      extensions: [{type: -1, body: undefined}],
    });
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
  });

  it('rejects compression method above uint8', () => {
    const spec = create(ClientHelloSpecSchema, {compressionMethods: [256]});
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
  });

  it('rejects negative compression method', () => {
    const spec = create(ClientHelloSpecSchema, {compressionMethods: [-1]});
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
  });

  it('rejects session_id_length below sentinel', () => {
    const spec = create(ClientHelloSpecSchema, {sessionIdLength: -2});
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
  });

  it('rejects session_id_length above 32', () => {
    const spec = create(ClientHelloSpecSchema, {sessionIdLength: 33});
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
  });

  it('rejects negative pad_to_size', () => {
    const spec = create(ClientHelloSpecSchema, {padToSize: -1});
    expect(() => validateClientHelloSpec(spec)).toThrow(ConnectError);
  });

  it('accepts valid session_id_length values', () => {
    for (const v of [-1, 0, 1, 16, 32]) {
      const spec = create(ClientHelloSpecSchema, {sessionIdLength: v});
      expect(() => validateClientHelloSpec(spec)).not.toThrow();
    }
  });
});

describe('assertBuiltinClientHelloHonorable', () => {
  it('accepts undefined spec', () => {
    expect(() => assertBuiltinClientHelloHonorable(undefined)).not.toThrow();
  });

  it('accepts empty/default spec', () => {
    const spec = create(ClientHelloSpecSchema, {});
    expect(() => assertBuiltinClientHelloHonorable(spec)).not.toThrow();
  });

  it('accepts compression [0]', () => {
    const spec = create(ClientHelloSpecSchema, {compressionMethods: [0]});
    expect(() => assertBuiltinClientHelloHonorable(spec)).not.toThrow();
  });

  it('accepts sessionIdLength 0 and 32', () => {
    for (const v of [0, 32]) {
      const spec = create(ClientHelloSpecSchema, {sessionIdLength: v});
      expect(() => assertBuiltinClientHelloHonorable(spec)).not.toThrow();
    }
  });

  it('rejects non-empty cipherSuites', () => {
    const spec = create(ClientHelloSpecSchema, {cipherSuites: [0x1301]});
    expect(() => assertBuiltinClientHelloHonorable(spec)).toThrow(ConnectError);
    try {
      assertBuiltinClientHelloHonorable(spec);
    } catch (err) {
      expect((err as ConnectError).code).toBe(Code.FailedPrecondition);
      expect((err as ConnectError).message).toContain('cipher_suites');
    }
  });

  it('rejects non-empty supportedGroups', () => {
    const spec = create(ClientHelloSpecSchema, {supportedGroups: [29]});
    expect(() => assertBuiltinClientHelloHonorable(spec)).toThrow(ConnectError);
  });

  it('rejects non-empty signatureAlgorithms', () => {
    const spec = create(ClientHelloSpecSchema, {
      signatureAlgorithms: [0x0403],
    });
    expect(() => assertBuiltinClientHelloHonorable(spec)).toThrow(ConnectError);
  });

  it('rejects non-empty extensions', () => {
    const spec = create(ClientHelloSpecSchema, {
      extensions: [{type: 43, body: undefined}],
    });
    expect(() => assertBuiltinClientHelloHonorable(spec)).toThrow(ConnectError);
  });

  it('rejects compression other than [0]', () => {
    const spec = create(ClientHelloSpecSchema, {compressionMethods: [1]});
    expect(() => assertBuiltinClientHelloHonorable(spec)).toThrow(ConnectError);
  });

  it('rejects multi-method compression', () => {
    const spec = create(ClientHelloSpecSchema, {
      compressionMethods: [0, 1],
    });
    expect(() => assertBuiltinClientHelloHonorable(spec)).toThrow(ConnectError);
  });

  it('rejects sessionIdLength -1', () => {
    const spec = create(ClientHelloSpecSchema, {sessionIdLength: -1});
    expect(() => assertBuiltinClientHelloHonorable(spec)).toThrow(ConnectError);
  });

  it('rejects sessionIdLength 16', () => {
    const spec = create(ClientHelloSpecSchema, {sessionIdLength: 16});
    expect(() => assertBuiltinClientHelloHonorable(spec)).toThrow(ConnectError);
  });

  it('rejects non-unspecified legacyVersion', () => {
    const spec = create(ClientHelloSpecSchema, {
      legacyVersion: TLSVersion.TLS_1_2,
    });
    expect(() => assertBuiltinClientHelloHonorable(spec)).toThrow(ConnectError);
  });

  it('rejects non-zero padToSize', () => {
    const spec = create(ClientHelloSpecSchema, {padToSize: 512});
    expect(() => assertBuiltinClientHelloHonorable(spec)).toThrow(ConnectError);
  });
});

describe('builtinClientHelloCapabilities', () => {
  it('returns all-false capabilities', () => {
    const caps = builtinClientHelloCapabilities();
    expect(caps.customCipherSuites).toBe(false);
    expect(caps.customSupportedGroups).toBe(false);
    expect(caps.customSignatureAlgorithms).toBe(false);
    expect(caps.customExtensionOrder).toBe(false);
    expect(caps.rawExtensions).toBe(false);
    expect(caps.greaseValues).toBe(false);
    expect(caps.sessionIdLength).toBe(false);
    expect(caps.paddingControl).toBe(false);
    expect(caps.legacyVersionControl).toBe(false);
    expect(caps.compressionMethods).toBe(false);
  });
});
