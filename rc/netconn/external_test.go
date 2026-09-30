package netconn_test

import (
	"github.com/joeycumines/sesame/internal/grpctest"
	"github.com/joeycumines/sesame/internal/testutil"
	"golang.org/x/exp/maps"
	"sort"
	"testing"
)

func TestClient_DialContext(t *testing.T) {
	for _, k := range testutil.CallOn(maps.Keys(testutil.ClientConnFactories), func(v []string) { sort.Strings(v) }) {
		t.Run(k, func(t *testing.T) {
			grpctest.RC_NetConn_TestClient_DialContext(t, testutil.ClientConnFactories[k])
		})
	}
}

func Test_nettest(t *testing.T) {
	wt := testutil.Wrap(t)
	wt = testutil.DepthLimiter{T: testutil.GoroutineChecker{T: wt}, Depth: 3}
	for _, k := range testutil.CallOn(maps.Keys(testutil.ClientConnFactories), func(v []string) { sort.Strings(v) }) {
		wt.Run(k, func(t testutil.T) {
			grpctest.RC_NetConn_Test_nettest(t, testutil.ClientConnFactories[k])
		})
	}
}
