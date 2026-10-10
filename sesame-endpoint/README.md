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

## Cross-stack behavior notes

The Go reference implementation (`rc/netconn`) plus
`remotecontrol.proto` define the wire contract; this endpoint matches
their observable semantics. A few deployment-policy defaults differ
intentionally and are recorded here so cross-stack testing is not
surprising:

- **Idle read timeout**: this endpoint tears down a tunnel whose
  upstream reads nothing for 30s (`DeadlineExceeded`); disable with
  `--read-timeout-ms 0`. The Go server applies no idle timeout.
- **Dial timeout default**: 10s here (`--dial-timeout-ms 0` disables);
  the Go server honors per-request dial timeouts and is otherwise
  unlimited.
- **Proxy auth precedence**: `auth_header` > the configured
  `SESAME_ENDPOINT_PROXY_AUTH_TOKEN` (sent as a Bearer token) > basic
  `username`/`password`. The Go stack uses `auth_header` > basic
  credentials only.
- **Upgrade failure**: both stacks emit `tls_upgrade_failed` and then
  terminate the stream with the failure status — never `OK`, never
  cleartext. Treat the event, not the terminal status, as the upgrade
  outcome.
- **Advertised `max_chunk_size`**: the wire contract requires
  advertised chunk and window sizes not to exceed the stream's gRPC
  per-message receive limit (grpc-go default: 4MiB); this endpoint
  enforces that ceiling at configuration parse time.

To install dependencies:

```bash
bun install
```

To run:

```bash
bun run start
```

This project was created using `bun init` in bun v1.4.2. [Bun](https://bun.com) is a fast all-in-one JavaScript runtime.
