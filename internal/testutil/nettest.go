package testutil

import (
	"net"
	"runtime"
	"testing"
)

// TestConn runs (an implementation of) nettest.TestConn multiple times, testing different permutations of conn order
// (swapping the roles of c1 and c2), and if closeConns is true, also the order of close (better at catching
// close-related deadlocks). If closeConns is false, then the caller must close the two conns as part of the stop
// closure. Any stop closure will be run before any auto-generated (closeConns) stop.
//
// If the nested t, provided by t.Run, is *testing.T, then the number of goroutines will be validated, to test for
// leaks. Other types (presumably testutil.T or equivalent) should be capable of implementing the same functionality via
// wrappers / other custom behavior.
func TestConn[
	T1 TG,
	T2 interface {
		TG
		Run(name string, f func(t T1)) bool
	},
	T3 ~func() (c1, c2 net.Conn, stop func(), err error)](
	t T2,
	test func(t T1, mp T3),
	closeConns bool,
	init func(t T1) T3,
) {
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
			// note: tests the underlying type
			switch t := (interface{})(t).(type) {
			case *testing.T:
				defer CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
			}
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
