# [gRPC Tunnels](https://github.com/jhump/grpctunnel)

This package is a hard fork of the project linked above.

There weren't any existing copyright notices, aside from the LICENSE file, so I've added the license header (to the
original files), for clarity.

Other notable changes:

- Deleted
  [stream_adapter.go](https://github.com/jhump/grpctunnel/blob/525f1361e55b62188ee09dedceed5b12a6fdb0f3/stream_adapter.go)
- Renamed and restructured a messages / fields
- Used the Sesame gRPC metadata type (note that the one in the grpctunnel package was broken for binary header vals)
- Renamed `tunnel_client.go` -> `channel.go`
- Renamed `tunnel_server.go` -> `tunnel.go`
- Renamed `tunnel_test.go` -> `service_test.go`
- Simplified API by removing pointless `ReverseTunnelChannel` wrapper
- Renamed `TunnelChannel` to `Channel`
- Simplified and improved extensibility of API by removing duplicated method pairs (`ServeTunnel` vs
  `ServeReverseTunnel`, `NewChannel` vs `NewReverseChannel`), using the option pattern instead
- Fixed unsafe send behavior via wrapper (see `stream.go`)
- Added graceful stop support
- Fixed CloseSend behavior (error handling)
- Implemented flow control (involving fixing numerous deadlocks)

## From the original readme

This library enables carrying gRPC over gRPC. There are a few niche use cases where this could be useful, but the most
widely applicable one is likely for letting gRPC servers communicate in the reverse direction, sending requests to
connected clients.

The tunnel is itself a gRPC service, which provides bidirectional streaming methods for forward and reverse tunneling.
There is also API for easily configuring the server handlers, be it on the server or (in the case of reverse tunnels) on
the client. Similarly, there is API for getting a "channel", from which you can create service stubs. This allows the
code that uses the stubs to not even care whether it has a normal gRPC client connection or a stub that sends the data
via a tunnel.

There is also API for "light-weight" tunneling, which is where a custom bidirectional stream can be used to send
messages back and forth, where the messages each act as RPC requests and responses, but on a single stream (for
pinning/affinity, for example).
