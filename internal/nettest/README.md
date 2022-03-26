# Test runner agnostic net/nettest port

The purpose of this package is to facilitate tests involving multiplexed streams, requiring concurrent test execution on
multiple `net.Conn` pairs.

The actual package is used wherever possible.

Last updated from `de3da57026dec695a705c07b5db51ef7a5252239`.

See also [golang.org/x/net/nettest](https://pkg.go.dev/golang.org/x/net/nettest).
