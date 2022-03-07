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
		halfCloser *stream.HalfCloser
	}

	netConn net.Conn

	piperI interface{ Pipe() stream.Pipe }
)

var (
	// compile time assertions

	_ net.Conn = (*WrappedPipe)(nil)
	_ piperI   = (*WrappedPipe)(nil)
	_ piperI   = (*stream.HalfCloser)(nil)
)

// Wrap uses stream.Wrap to implement net.Conn using an io.ReadWriteCloser.
//
// If conn is a stream.Pipe or implements `interface{ Pipe() stream.Pipe }` (stream.Piper), and has no reader
// (stream.Pipe.Reader), the returned conn will correctly match the io.EOF behavior. A nil stream.Pipe.Writer is
// handled in a similar manner, though that case will result in io.ErrClosedPipe.
//
// See also stream.Wrap.
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
//
// See also stream.Wrap.
func WrapPipe(options ...stream.HalfCloserOption) (*WrappedPipe, error) {
	halfCloser, err := stream.NewHalfCloser(options...)
	if err != nil {
		return nil, err
	}
	w := WrappedPipe{netConn: Wrap(halfCloser), halfCloser: halfCloser}
	return &w, nil
}

// Pipe exposes the stream.Pipe that the receiver is wrapping.
func (x *WrappedPipe) Pipe() stream.Pipe { return x.halfCloser.Pipe() }

func (x *WrappedPipe) Close() (err error) {
	defer func() {
		// Close the internal pipes (connecting to what was wrapped), then wait for copying to finish.
		// WARNING This is AFTER closing the other (wrapped / caller provided) side due to it waiting for copying to
		//         finish. That is, unless both sides of the copying process are closed and have net.Conn-like
		//         semantics, it may block forever, waiting for the copying to finish.
		if e := x.netConn.Close(); err == nil {
			// note: this is the least interesting error
			err = e
		}
	}()
	defer func() {
		// close the remaining parts of the wrapped pipe (everything except the half closer)
		// it's possible that this has already occurred, e.g. if the stream.PipeWriter returned an error
		pipe := x.Pipe()
		// avoid multiple closes, grab any cached error (will only ever propagate any error)
		pipe.Writer = x.halfCloser
		// note: error priority is like pipe.Writer > pipe.Reader > pipe.Closer
		err = pipe.Close()
	}()
	// note we'll grab any error for this in a bit
	_ = x.halfCloser.Close()
	return
}
