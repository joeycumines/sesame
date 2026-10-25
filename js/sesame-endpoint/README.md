# sesame-endpoint

`sesame-endpoint` is a gRPC endpoint server implementing `RemoteControl.NetConn`
(see `../schema/sesame/v1alpha1/remotecontrol.proto`). It is configured
exclusively through CLI arguments and `SESAME_ENDPOINT_*` environment variables.

```bash
# From a checkout
bun install
bun run compile
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

## Development

Development and package management use [Bun](https://bun.com). The compiled
CLI is also exercised under Node.js by `test:node` so the published package
stays runtime-compatible with both runtimes.

```bash
# Install dependencies
bun install

# Run full checks (clean, compile, Bun tests, Node runner, linter, and
# packed-artifact verification)
bun run check

# Run tests under Bun
bun test

# Run the compiled output under Node.js (requires `bun run compile` first)
bun run test:node

# Run linter
bun run lint
```

## Release

Releases are published to NPM using `bun publish`. The `package.json` `files`
allowlist is `["build/src"]`: it ships the compiled library and CLI. NPM
always adds the root `package.json`, `README.md`, and `LICENSE`, so those are
present in the tarball as well; no other repository content is packed.

`pack:verify` packs a real tarball in a temporary directory, checks its
contents, extracts it, and runs the extracted CLI under both Node.js and Bun.
`check`, `pack`, and the publish scripts run this verification, so a
checkout-only test cannot mask a broken artifact.

Release tooling requires Bun 1.2.18 or newer (`engines.bun`): the
`version:*` scripts shell out to `bun pm version`, which Bun gained in
1.2.18. Older Bun versions fail the engines check instead of crashing
mid-release.

```bash
# 1. Verify the tarball, then preview packaged contents
bun run pack:dry

# 2. Bump version (patch / minor / major / prerelease)
bun run version:patch

# 3. Simulate NPM publish (dry-run)
bun run publish:dry

# 4. Publish to NPM
bun run publish:latest   # or publish:next
```

`bun run pack` runs the same verification and leaves the verified tarball in
the package directory.

From the repository root, equivalent GNU Make targets are available. Use
`make` on Linux and `gmake` on macOS:
```bash
make sesame-endpoint.check
make sesame-endpoint.pack-dry
make sesame-endpoint.pack
make sesame-endpoint.publish-dry
make sesame-endpoint.publish
```

## License

This package is licensed under the [MIT License](./LICENSE).
