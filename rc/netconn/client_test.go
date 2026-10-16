package netconn

import (
	"fmt"
	sesamepb "github.com/joeycumines/sesame/sesamepb"
	typepb "github.com/joeycumines/sesame/typepb"
	"google.golang.org/protobuf/types/known/durationpb"
	"testing"
	"time"
)

func TestNetConn_String(t *testing.T) {
	const (
		networkValue       = `network`
		dialerValue        = `dialer`
		listenerValue      = `listener`
		clientNetworkValue = `client_network`
		clientAddressValue = `client_address`
		clientTimeout      = time.Duration(1234567898)
	)
	for _, tc := range [...]struct {
		Name   string
		Conn   *netConn
		String string
	}{
		{
			Name:   `unset`,
			Conn:   &netConn{},
			String: "sesame.v1alpha1.RemoteControl.NetConn()",
		},
		{
			Name: `set`,
			Conn: &netConn{
				req: &sesamepb.NetConnRequest_Dial{
					Address: &typepb.NetAddr{
						Network: clientNetworkValue,
						Address: clientAddressValue,
					},
					Timeout: durationpb.New(clientTimeout),
				},
				res: &sesamepb.NetConnResponse_Conn{
					Local: &typepb.NetAddr{
						Network: networkValue,
						Address: dialerValue,
					},
					Remote: &typepb.NetAddr{
						Network: networkValue,
						Address: listenerValue,
					},
				},
			},
			String: "sesame.v1alpha1.RemoteControl.NetConn(req_net=\"client_network\" req_addr=\"client_address\" req_timeout=1.234567898s local_net=\"network\" local_addr=\"dialer\" remote_net=\"network\" remote_addr=\"listener\")",
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			if s := tc.Conn.String(); s != fmt.Sprint(tc.Conn) || s != tc.String {
				t.Errorf("unexpected stringer: %q\n%s", s, s)
			}
		})
	}
}
