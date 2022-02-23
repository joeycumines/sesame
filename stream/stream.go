package stream

import (
	"errors"
	"io"
	"sync"
)

const (
	// ChunkSize is 32 kilobytes, and is provided for use in situations where an arbitrary chunk size is required.
	ChunkSize = 1 << 15
)

type (
	// Handler implements a stream server or client, which may interact with reader and/or writer, and may have
	// resources requiring cleanup, which should occur on close of the result.
	Handler interface {
		Handle(r PipeReader, w PipeWriter) io.Closer
	}

	// Pipe combines a PipeReader with a PipeWriter and an io.Closer, where all fields are optional.
	//
	// It is intended to be used as the outwards-facing implementation of a bidirectional stream, backed by a suitable
	// pipe implementation. This pipe implementation may be used as a shim, to simplify difficult-to-implement stream
	// semantics, e.g. through the use of the Handler interface. A common reason to use such implementations include
	// the need to implement cancellation / "safe" close implementations, or the need for IO wrappers with extended
	// functionality (e.g. net.Conn - see also ionet.Wrap).
	//
	// See also the method documentation.
	Pipe struct {
		Reader PipeReader
		Writer PipeWriter
		Closer io.Closer
	}

	// HandlerFunc implements Handler.
	HandlerFunc func(r PipeReader, w PipeWriter) io.Closer
)

var (
	// ErrPanic indicates that a panic took place, and is used as a sentinel value in certain cases, e.g. as the result
	// of Closer.Close following a panic during the first attempt, for caching / idempotent closers generated using
	// Closer.Once.
	ErrPanic = errors.New(`sesame/stream: unhandled panic`)

	// compile time assertions

	_ io.ReadWriteCloser = Pipe{}
	_ PipeReader         = Pipe{}
	_ PipeWriter         = Pipe{}
)

// Pair is a convenience function that may be used to build a connected bidirectional pipe, e.g. like
// `local, remote := stream.Pair(io.Pipe())(io.Pipe())`.
//
// Depending on use case, it may be desirable to "synchronise" either or both of the sender and receiver pipes, using
// SyncPipe, the results of which can be used directly in each closure. Similar behavior can also be achieved by
// implementations which implement io.WriterTo in a way that blocks writes until the end of read, e.g. ionet.Pipe.
func Pair(sendReader PipeReader, sendWriter PipeWriter) func(receiveReader PipeReader, receiveWriter PipeWriter) (local, remote Pipe) {
	return func(receiveReader PipeReader, receiveWriter PipeWriter) (local, remote Pipe) {
		local = Pipe{
			Reader: receiveReader,
			Writer: sendWriter,
		}
		remote = Pipe{
			Reader: sendReader,
			Writer: receiveWriter,
		}
		return
	}
}

// Wrap is a convenience function that wraps a raw stream (io.ReadWriteCloser) in pipes, e.g. to support
// cancellation, while retaining roughly the same semantics. Both pipes are optional, although not providing either is
// obviously rather pointless.
//
// Note that the pipe directions are named relative to the internal Handler (Join), which copies to and from stream.
// As such, the "send" pipe corresponds to reading from the result, while the "receive" pipe corresponds to writes to
// the result.
//
// If the "receive" pipe was provided, and receiveReader does not implement io.WriterTo, then the "receive" pipe will
// be synchronised using SyncPipe. This is intended as sensible default behavior, as, depending on the pipe
// implementation, it maybe necessary to facilitate close after writing, w/o prematurely closing / dropping data written
// to receiveWriter then read from receiveReader, but not yet written to stream. The io.WriterTo special case is in
// order to support implementations with their own (presumably more effective) solutions, to the problem which SyncPipe
// aims to solve. N.B. neither io.Pipe or net.Pipe implement io.WriterTo, though it is implemented by ionet.Pipe.
//
// This implementation is compatible with HalfCloser.
//
// See also ionet.Wrap and ionet.WrapPipe.
func Wrap(sendReader PipeReader, sendWriter PipeWriter) func(receiveReader PipeReader, receiveWriter PipeWriter) func(stream io.ReadWriteCloser) Pipe {
	return func(receiveReader PipeReader, receiveWriter PipeWriter) func(stream io.ReadWriteCloser) Pipe {
		return func(stream io.ReadWriteCloser) Pipe {
			if receiveReader != nil || receiveWriter != nil {
				if _, ok := receiveReader.(io.WriterTo); !ok {
					receiveReader, receiveWriter = SyncPipe(receiveReader, receiveWriter)
				}
			}
			return Handle(sendReader, sendWriter)(receiveReader, receiveWriter)(Join(stream))
		}
	}
}

// Handle is a convenience function intended to be used with io.Pipe or ionet.Pipe, to generate an io.ReadWriteCloser
// from a Handler, e.g. like `pipe := stream.Handle(io.Pipe())(io.Pipe())(handler)` or
// `handlerPipe := stream.Handle(basePipe.SendPipe())(basePipe.ReceivePipe())(handler)`.
func Handle(sendReader PipeReader, sendWriter PipeWriter) func(receiveReader PipeReader, receiveWriter PipeWriter) func(handler Handler) Pipe {
	return func(receiveReader PipeReader, receiveWriter PipeWriter) func(handler Handler) Pipe {
		return func(handler Handler) Pipe {
			return Pipe{
				Reader: sendReader,
				Writer: receiveWriter,
				Closer: handler.Handle(receiveReader, sendWriter),
			}
		}
	}
}

// Join returns a Handler that will connect to a given io.ReadWriteCloser (stream).
// Note that stream will be closed as part of triggering cleanup, it's close func may be called more than once, and may
// be called concurrently.
// Additionally, unlike Pipe, the pipes (writer then reader) will be closed PRIOR to stream, and there will be no close
// error propagation (between the components), as all three are logically separate entities. This also means any close
// error from stream takes priority (in the returned closer).
func Join(stream io.ReadWriteCloser) Handler {
	return HandlerFunc(func(reader PipeReader, writer PipeWriter) (closer io.Closer) {
		// ensure stream doesn't implement io.WriterTo or io.ReaderFrom, as the bidirectional copying + nested
		// blocking behavior may introduce deadlocks
		type ioReadWriteCloser1 io.ReadWriteCloser
		type ioReadWriteCloser2 struct{ ioReadWriteCloser1 }
		stream = ioReadWriteCloser2{stream}

		var wg sync.WaitGroup

		// initial add to prevent done before init, and to avoid panic if both reader and writer are nil
		wg.Add(1)

		if reader != nil {
			wg.Add(1)
			go joinCopy(&wg, stream, reader, stream, reader)
		}

		if writer != nil {
			wg.Add(1)
			go joinCopy(&wg, stream, writer, writer, stream)
		}

		// done init
		wg.Done()

		return Closer(func() (err error) {
			// note that we don't propagate close errors
			// (they are all separate entities)
			alwaysCallClosersOrdered(
				&err,
				writer,
				reader,
				stream,
			)
			wg.Wait()
			return
		}).Comparable()
	})
}

// joinCopy implements the copy behavior common between both pipe directions, for use by Join
// WARNING at risk of blocking forever until the io.Closer returned by Join is called
func joinCopy(wg interface{ Done() }, stream io.Closer, pipe PipeCloser, dst io.Writer, src io.Reader) {
	defer wg.Done()

	// ensure stream is always closed, deferred within a closure as stream may end up nil (to avoid closing twice)
	defer func() { alwaysCallClosersOrdered(nil, stream) }()

	err := ErrPanic
	defer func() { _ = pipe.CloseWithError(err) }()

	_, err = io.Copy(dst, src)

	// call stream.Close, first setting stream to nil to ensure it isn't called twice
	s := stream
	stream = nil
	if err != nil {
		// call close but ignore the error, we already got a more useful one
		_ = s.Close()
	} else {
		// call s.Close, potentially set err (handles panic case as well)
		alwaysCallClosersOrdered(&err, s)
	}
}

// Read passes through to Reader, or returns io.EOF if Reader is nil.
func (x Pipe) Read(p []byte) (n int, err error) {
	if x.Reader != nil {
		return x.Reader.Read(p)
	}
	return 0, io.EOF
}

// Write passes through to Writer, or returns io.ErrClosedPipe if Writer is nil.
func (x Pipe) Write(p []byte) (n int, err error) {
	if x.Writer != nil {
		return x.Writer.Write(p)
	}
	return 0, io.ErrClosedPipe
}

// Close will pass through to the closer, then close the reader, then writer, skipping any that are nil. Any close
// error will be propagated to the pipes. Note that the last close error will be used, and the error passed to
// the PipeCloser.CloseWithError methods will be the most recent error, or nil.
//
// As Pipe is intended to be the outwards-facing interface to an internal stream implementation, Pipe.Closer is called
// first, to facilitate propagation of any internal (e.g. close-related) errors, via Pipe.Reader and Pipe.Writer, prior
// to ensuring that they are closed (from this end).
//
// See also Pipe.CloseWithError.
func (x Pipe) Close() error { return x.close(nil) }

// CloseWithError provides PipeCloser semantics for the union of Pipe.Reader and Pipe.Writer.
// Calling this method with nil is equivalent to calling Pipe.Close.
// If a non-nil error is provided, that error will (always) be passed down to the pipes, after calling Pipe.Closer.
func (x Pipe) CloseWithError(err error) error { return x.close(err) }

func (x Pipe) close(override error) (err error) {
	var errCloserWithErr *error
	if override != nil {
		// use the explicitly set error for the pipe CloseWithError calls
		errCloserWithErr = &override
	} else {
		// use the most recent returned error (if any)
		errCloserWithErr = &err
	}
	closers := make([]io.Closer, 1, 3)
	closers[0] = x.Closer
	if x.Reader != nil {
		closers = append(closers, Closer(func() error { return x.Reader.CloseWithError(*errCloserWithErr) }))
	}
	if x.Writer != nil {
		closers = append(closers, Closer(func() error { return x.Writer.CloseWithError(*errCloserWithErr) }))
	}
	alwaysCallClosersOrdered(&err, closers...)
	return
}

// Handle implements Handler using the receiver.
func (x HandlerFunc) Handle(r PipeReader, w PipeWriter) io.Closer { return x(r, w) }
