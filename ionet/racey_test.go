//go:build !race
// +build !race

package ionet

import (
	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/stream"
	"golang.org/x/net/nettest"
	"io"
	"net"
	"runtime"
	"testing"
)

func TestPipe_nettest_a(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
	nettest.TestConn(t, func() (c1, c2 net.Conn, stop func(), _ error) {
		c1, c2 = Pipe()
		stop = func() {
			_ = c1.Close()
			_ = c2.Close()
		}
		return
	})
}

func TestPipe_nettest_b(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
	nettest.TestConn(t, func() (c1, c2 net.Conn, stop func(), _ error) {
		c2, c1 = Pipe()
		stop = func() {
			_ = c1.Close()
			_ = c2.Close()
		}
		return
	})
}

func TestWrap_nettest(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
	wrapN := func(stream io.ReadWriteCloser, n int) (conn net.Conn) {
		if n <= 0 {
			panic(n)
		}
		for i := 0; i < n; i++ {
			conn = Wrap(stream)
			stream = conn
		}
		return
	}
	for _, tc := range [...]struct {
		Name string
		Init func(t *testing.T) func() (c1, c2 net.Conn, stop func(), _ error)
	}{
		{
			Name: `pipe rwc`,
			Init: func(t *testing.T) func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					type c interface {
						io.ReadWriteCloser
					}
					type C struct{ c }
					a, b := net.Pipe()
					c1, c2 = Wrap(C{a}), Wrap(C{b})
					stop = func() {
						_ = c1.Close()
						_ = c2.Close()
					}
					return
				}
			},
		},
		{
			Name: `pipe dual deep nested`,
			Init: func(t *testing.T) func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					a, b := Pipe()
					c1, c2 = wrapN(a, 20), wrapN(b, 50)
					stop = func() {
						_ = c1.Close()
						_ = c2.Close()
					}
					return
				}
			},
		},
		{
			Name: `pipe rwc+writeto dual`,
			Init: func(t *testing.T) func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					type c interface {
						io.ReadWriteCloser
						io.WriterTo
					}
					type C struct{ c }
					a, b := Pipe()
					c1, c2 = Wrap(C{a}), Wrap(C{b})
					stop = func() {
						_ = c1.Close()
						_ = c2.Close()
					}
					return
				}
			},
		},
		{
			Name: `pipe rwc+writeto single`,
			Init: func(t *testing.T) func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					type c interface {
						io.ReadWriteCloser
						io.WriterTo
					}
					type C struct{ c }
					a, b := Pipe()
					c1, c2 = Wrap(C{a}), b
					stop = func() {
						_ = c1.Close()
						_ = c2.Close()
					}
					return
				}
			},
		},
		{
			Name: `stream pair io pipe`,
			Init: func(t *testing.T) func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					a, b := stream.Pair(stream.SyncPipe(io.Pipe()))(stream.SyncPipe(io.Pipe()))
					c1, c2 = Wrap(a), Wrap(b)
					stop = func() {
						_ = c1.Close()
						_ = c2.Close()
					}
					return
				}
			},
		},
		{
			Name: `stream pair io pipe half closer`,
			Init: func(t *testing.T) func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					a, b := stream.Pair(stream.SyncPipe(io.Pipe()))(stream.SyncPipe(io.Pipe()))
					ac, err := WrapPipe(a)
					if err != nil {
						t.Fatal(err)
					}
					bc, err := WrapPipe(b)
					if err != nil {
						t.Fatal(err)
					}
					c1, c2 = ac, bc
					stop = func() {
						_ = c1.Close()
						_ = c2.Close()
					}
					return
				}
			},
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Run(`a`, func(t *testing.T) {
				defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
				nettest.TestConn(t, tc.Init(t))
			})
			t.Run(`b`, func(t *testing.T) {
				defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
				nettest.TestConn(t, func() (c1, c2 net.Conn, stop func(), err error) {
					c2, c1, stop, err = tc.Init(t)()
					return
				})
			})
		})
	}
}
