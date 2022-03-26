# Test runner agnostic net/nettest port

The purpose of this package is to facilitate tests involving multiplexed streams, requiring concurrent test execution on
multiple `net.Conn` pairs.

The actual package is used wherever possible.

See also [golang.org/x/net/nettest](https://pkg.go.dev/golang.org/x/net/nettest).
