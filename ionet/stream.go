package ionet

import (
	"github.com/joeycumines/sesame/stream"
	"io"
	"net"
)

// Wrap uses stream.Wrap to implement net.Conn using an io.ReadWriteCloser.
func Wrap(conn io.ReadWriteCloser) net.Conn {
	if conn == nil {
		panic(`sesame/ionet: expected non-nil conn`)
	}
	type (
		netConn           net.Conn
		nestedConnPipe1   struct{ netConn }
		nestedConnPipe2   struct{ nestedConnPipe1 }
		ioReadWriteCloser io.ReadWriteCloser
		netStream         struct {
			nestedConnPipe2
			ioReadWriteCloser
		}
	)
	c1, c2 := Pipe()
	return &netStream{
		nestedConnPipe2:   nestedConnPipe2{nestedConnPipe1{netConn: c1}},
		ioReadWriteCloser: stream.Wrap(c2.SendPipe())(c2.ReceivePipe())(conn),
	}
}
