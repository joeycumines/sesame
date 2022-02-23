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
// If conn is a stream.Pipe or implements `interface{ Pipe() stream.Pipe }`, and has no reader (stream.Pipe.Reader),
// the returned conn will correctly match the io.EOF behavior. A nil stream.Pipe.Writer is handled in a similar manner,
// though that case will result in io.ErrClosedPipe.
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
	var (
		c1, c2                    = Pipe()
		sendReader, receiveReader stream.PipeReader
		sendWriter, receiveWriter stream.PipeWriter
	)
	sendReader, sendWriter = c2.SendPipe()
	receiveReader, receiveWriter = c2.ReceivePipe()
	{
		pipe, ok := conn.(stream.Pipe)
		if !ok {
			if wrapper, okWrapper := conn.(piperI); okWrapper {
				pipe, ok = wrapper.Pipe(), true
			}
		}
		if ok {
			if pipe.Reader == nil {
				_ = sendWriter.Close()
				sendReader, sendWriter = nil, nil
			}
			if pipe.Writer == nil {
				_ = receiveWriter.Close()
				receiveReader, receiveWriter = nil, nil
			}
		}
	}
	return &netStream{
		nestedConnPipe2:   nestedConnPipe2{nestedConnPipe1{netConn: c1}},
		ioReadWriteCloser: stream.Wrap(sendReader, sendWriter)(receiveReader, receiveWriter)(conn),
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
	w := WrappedPipe{netConn: Wrap(h), halfCloser: h}
	return &w, nil
}

// Pipe exposes the stream.Pipe that the receiver is wrapping.
func (x *WrappedPipe) Pipe() stream.Pipe { return x.halfCloser.Pipe() }

func (x *WrappedPipe) Close() (err error) {
	defer func() {
		pipe := x.Pipe()
		// avoid multiple closes, grab any cached error
		// note it should always have been called prior to this
		pipe.Writer = x.halfCloser
		if e := pipe.Close(); err == nil {
			err = e
		}
	}()
	err = x.netConn.Close()
	return
}
