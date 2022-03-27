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
