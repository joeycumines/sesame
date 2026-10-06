# sesame-endpoint

`sesame-endpoint` is a gRPC endpoint server implementing `RemoteControl.NetConn`
(see `../schema/sesame/v1alpha1/remotecontrol.proto`). It is configured
exclusively through CLI arguments and `SESAME_ENDPOINT_*` environment variables.

```bash
bun run start --help
```

TLS fingerprint presets are fail-closed. This runtime applies only
`RUNTIME_DEFAULT`; `--supported-presets` (or `SESAME_ENDPOINT_SUPPORTED_PRESETS`)
declares what the server advertises, and rejects at startup any list naming a
preset the runtime cannot honour. A dial requesting a preset outside the
advertised set is refused with `FAILED_PRECONDITION`. Serving real browser
fingerprints requires a custom `TLSProvider`.

To install dependencies:

```bash
bun install
```

To run:

```bash
bun run start
```

This project was created using `bun init` in bun v1.4.2. [Bun](https://bun.com) is a fast all-in-one JavaScript runtime.
