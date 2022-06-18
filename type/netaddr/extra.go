package netaddr

type (
	// GoNetAddr is a copy of net.Addr.
	// See also NetAddr.AsGoNetAddr and New.
	GoNetAddr interface {
		Network() string
		String() string
	}

	goNetAddr struct {
		network string
		address string
	}
)

var (
	// compile time assertions

	_ GoNetAddr = goNetAddr{}
)

// New constructs a new NetAddr from the provided net.Addr.
// Note that nil will be returned if value is nil.
func New(value GoNetAddr) *NetAddr {
	if value != nil {
		return &NetAddr{
			Network: value.Network(),
			Address: value.String(),
		}
	}
	return nil
}

// AsGoNetAddr returns the receiver as a net.Addr.
// Note that nil will be returned if the receiver is nil.
func (x *NetAddr) AsGoNetAddr() GoNetAddr {
	if x != nil {
		return goNetAddr{
			network: x.Network,
			address: x.Address,
		}
	}
	return nil
}

func (x goNetAddr) Network() string { return x.network }

func (x goNetAddr) String() string { return x.address }
