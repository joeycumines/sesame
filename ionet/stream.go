package ionet

import (
	"github.com/joeycumines/sesame/stream"
	"io"
	"net"
)

type (
	// WrappedPipe models a stream.Pipe which has been wrapped to implement net.Conn.
	WrappedPipe struct {
		netConn
		pipe stream.Pipe
	}

	netConn net.Conn
)

var (
	// compile time assertions
	_ net.Conn = (*WrappedPipe)(nil)
)

// Wrap uses stream.Wrap to implement net.Conn using an io.ReadWriteCloser.
func Wrap(conn io.ReadWriteCloser) net.Conn {
	if conn == nil {
		panic(`sesame/ionet: expected non-nil conn`)
	}
	type (
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

// WrapPipe extends Wrap with support for stream.HalfCloser.
func WrapPipe(conn stream.Pipe, options ...stream.HalfCloserOption) (*WrappedPipe, error) {
	const c = 1
	o := make([]stream.HalfCloserOption, c, len(options)+c)
	o[0] = stream.OptHalfCloser.Pipe(conn)
	o = append(o, options...)
	h, err := stream.NewHalfCloser(o...)
	if err != nil {
		return nil, err
	}
	w := WrappedPipe{netConn: Wrap(h), pipe: conn}
	return &w, nil
}

// Pipe exposes the stream.Pipe that the receiver is wrapping.
func (x *WrappedPipe) Pipe() stream.Pipe { return x.pipe }

func (x *WrappedPipe) Close() (err error) {
	defer func() {
		if e := x.pipe.Close(); err == nil {
			err = e
		}
	}()
	err = x.netConn.Close()
	return
}
