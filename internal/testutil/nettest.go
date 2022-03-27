package testutil

import (
	"net"
	"runtime"
)

func TestConn[
	T1 TB,
	T2 interface {
		TB
		Run(name string, f func(t T1)) bool
	},
	T3 ~func() (c1, c2 net.Conn, stop func(), err error)](
	t T2,
	test func(t T1, mp T3),
	closeConns bool,
	init func(t T1) T3,
) {
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
		t.Run(tc.Name, func(t T1) {
			CleanupCheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
			mp := init(t)
			test(t, func() (c1, c2 net.Conn, stop func(), err error) {
				c1, c2, stop, err = (func() (c1, c2 net.Conn, stop func(), err error))(mp)()
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
