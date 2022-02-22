# Sesame

The goal of this module is to implement tooling to implement a sensible and performant "reverse tunnel" (i.e. NAT proxy
/ hole punching) mechanism, suitable for production use in a cloud context, using gRPC streams. It is intended that this
project will expose a suite of useful, modular tooling (as packages or commands), to best address this inherently niche
use case.

Exported packages will be versioned and maintained as appropriate for a Go module. Note that v1 is _at least_ several
months away.

This project aims to address similar use cases as [fatedier/frp](https://github.com/fatedier/frp),
[jhump/grpctunnel](https://github.com/jhump/grpctunnel), and numerous others. While it is not yet clear what the
supported architectures will be, it is expected that the core architecture will be composed of

1. Remotes, an arbitrary and dynamic number of devices acting as endpoints that expose (pluggable) "socket"
   implementations, gRPC APIs, etc, that runs or otherwise implements the necessary client implementation
2. Remote registration / tunnel API (a gRPC service will be provided, but it's expected to be common to implement this
   yourself, e.g. to extend the functionality)
3. Outward facing reverse dialer API, initiated over 2., and initiated by 4.
4. Endpoint API similar to app mesh implementations, exposing remotes, and remote APIs (APIs running on remotes), e.g.
   using virtualhosts, and an internal message bus to communicate with 2. (and some other mechanism to route the reverse
   dialed request)

Implementation wise, the core implementation of 2. can be expected to be a message-based protocol that uses gRPC's
bidirectional streaming. Similarly, 3. will likely be a gRPC API.

It's quite likely that a basic standalone server will be provided, to implement 2-4.

There are notable gaps in the above architecture, but they will need to be filled in over time.
