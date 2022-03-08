package ionet

import (
	"github.com/joeycumines/sesame/stream"
	"io"
	"net"
)

type (
	// WrappedConn models an io.ReadWriteCloser which has been wrapped to implement net.Conn.
	WrappedConn struct {
		ioReadWriteCloser
		embeddedNetConn
		pipe stream.Pipe
	}

	// WrappedPipe models a stream.Pipe which has been wrapped to implement net.Conn.
	WrappedPipe struct {
		netConn
		halfCloser  *stream.HalfCloser
		wrappedConn *WrappedConn
	}

	netConn net.Conn

	ioReadWriteCloser io.ReadWriteCloser

	// embeddedNetConn embeds netConn for prioritisation of embedded methods
	embeddedNetConn struct{ netConn }
)

var (
	// compile time assertions

	_ net.Conn     = (*WrappedConn)(nil)
	_ stream.Piper = (*WrappedConn)(nil)
	_ net.Conn     = (*WrappedPipe)(nil)
	_ stream.Piper = (*WrappedPipe)(nil)
)

// Wrap uses stream.Wrap to implement net.Conn using an io.ReadWriteCloser.
//
// If conn is a stream.Pipe or implements `interface{ Pipe() stream.Pipe }` (stream.Piper), and has no reader
// (stream.Pipe.Reader), the returned conn will correctly match the io.EOF behavior. A nil stream.Pipe.Writer is
// handled in a similar manner, though that case will result in io.ErrClosedPipe.
//
// See also stream.Wrap.
func Wrap(conn io.ReadWriteCloser) (w *WrappedConn) {
	if conn == nil {
		panic(`sesame/ionet: expected non-nil conn`)
	}
	c1, c2 := Pipe()
	w = &WrappedConn{
		embeddedNetConn: embeddedNetConn{netConn: c1},
		pipe:            stream.Wrap(c2.SendPipe())(c2.ReceivePipe())(conn),
	}
	w.ioReadWriteCloser = w.pipe
	return
}

// WrapPipe extends Wrap with support for stream.HalfCloser.
//
// See also stream.Wrap.
func WrapPipe(options ...stream.HalfCloserOption) (w *WrappedPipe, err error) {
	w = new(WrappedPipe)
	w.halfCloser, err = stream.NewHalfCloser(options...)
	if err != nil {
		return nil, err
	}
	w.wrappedConn = Wrap(w.halfCloser)
	w.netConn = w.wrappedConn
	return
}

// Pipe exposes the stream.Pipe for the receiver (writes and reads to it are equivalent to the receiver).
func (x *WrappedConn) Pipe() stream.Pipe { return x.pipe.Pipe() }

// Pipe exposes the stream.Pipe for the receiver (writes and reads to it are equivalent to the receiver).
func (x *WrappedPipe) Pipe() stream.Pipe { return x.wrappedConn.Pipe() }

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
		pipe := x.halfCloser.Pipe()
		// avoid multiple closes, grab any cached error (will only ever propagate any error)
		pipe.Writer = x.halfCloser
		// note: error priority is like pipe.Writer > pipe.Reader > pipe.Closer
		err = pipe.Close()
	}()
	// note we'll grab any error for this in a bit
	_ = x.halfCloser.Close()
	return
}
