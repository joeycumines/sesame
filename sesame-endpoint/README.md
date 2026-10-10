# sesame-endpoint

`sesame-endpoint` is a gRPC endpoint server implementing `RemoteControl.NetConn`
(see `../schema/sesame/v1alpha1/remotecontrol.proto`). It is configured
exclusively through CLI arguments and `SESAME_ENDPOINT_*` environment variables.

```bash
bun run start --help
```

Upstream TLS impersonation is expressed as a decomposed, wire-level
`ClientHelloSpec` (`TLSOptions.client_hello` in
`../schema/sesame/tls/v1alpha1/tls.proto`) — there are no presets and no
modes. If the field is absent, the engine performs its default handshake;
if present, the engine MUST honour every requested dimension exactly or
fail closed with `FAILED_PRECONDITION` before the handshake, and the
applied spec is echoed back in `TLSHandshakeResult.applied_client_hello`
for client-side verification. The builtin `node:tls` engine advertises
all-false `ClientHelloCapabilities` (see `Capabilities.client_hello_capabilities`)
and rejects any non-default spec dimension; full-fidelity impersonation
(e.g. opencode/Bun) is provided by plugging a custom `TLSProvider` into
`createRemoteControlService`.

To install dependencies:

```bash
bun install
```

To run:

```bash
bun run start
```

This project was created using `bun init` in bun v1.4.2. [Bun](https://bun.com) is a fast all-in-one JavaScript runtime.
