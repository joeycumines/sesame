package testutil

import (
	"golang.org/x/net/nettest"
	"net"
	"runtime"
	"testing"
)

func TestConn(t *testing.T, closeConns bool, init func(t *testing.T) nettest.MakePipe) {
	defer CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
	for _, tc := range [...]struct {
		Name      string
		SwapConn  bool
		SwapClose bool
	}{
		{
			Name: `a`,
		},
		{
			Name:     `b`,
			SwapConn: true,
		},
		{
			Name:      `c`,
			SwapClose: true,
		},
		{
			Name:      `d`,
			SwapConn:  true,
			SwapClose: true,
		},
	} {
		tc := tc
		if !closeConns && tc.SwapClose {
			// skip c and d if closeConns is false
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			CleanupCheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
			mp := init(t)
			nettest.TestConn(t, func() (c1, c2 net.Conn, stop func(), err error) {
				c1, c2, stop, err = mp()
				if tc.SwapConn {
					c1, c2 = c2, c1
				}
				if closeConns {
					s := stop
					stop = func() {
						if s != nil {
							s()
						}
						c1, c2 := c1, c2
						if tc.SwapClose {
							c1, c2 = c2, c1
						}
						if err := c1.Close(); err != nil {
							t.Error(err)
						}
						if err := c2.Close(); err != nil {
							t.Error(err)
						}
					}
				}
				return
			})
		})
	}
}
