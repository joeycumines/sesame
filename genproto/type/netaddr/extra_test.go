package netaddr

import (
	"google.golang.org/protobuf/proto"
	"testing"
)

type mockNetAddr struct {
	network string
	address string
}

func (x *mockNetAddr) Network() string { return x.network }

func (x *mockNetAddr) String() string { return x.address }

func TestNew(t *testing.T) {
	t.Run(`nil`, func(t *testing.T) {
		msg := New(nil)
		if msg != nil {
			t.Fatal(msg)
		}
		val := msg.AsGoNetAddr()
		if val != nil {
			t.Fatal(val)
		}
	})
	t.Run(`copy`, func(t *testing.T) {
		orig := &mockNetAddr{
			network: "some network",
			address: "some address",
		}
		msg := New(orig)
		if !proto.Equal(msg, &NetAddr{
			Network: "some network",
			Address: "some address",
		}) {
			t.Error(msg)
		}
		src := msg.AsGoNetAddr()
		if src == nil {
			t.Fatal()
		}
		if v := src.String(); v != `some address` {
			t.Error(v)
		}
		if v := src.Network(); v != `some network` {
			t.Error(v)
		}
		msg.Network = `another network`
		msg.Address = `another address`
		if v := src.String(); v != `some address` {
			t.Error(v)
		}
		if v := src.Network(); v != `some network` {
			t.Error(v)
		}
		dst := New(src).AsGoNetAddr()
		if src != dst {
			t.Error(src, dst)
		}
		if msg.AsGoNetAddr() != New(&mockNetAddr{
			network: "another network",
			address: "another address",
		}).AsGoNetAddr() {
			t.Error(dst)
		}
	})
}
