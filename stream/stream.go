package stream

import (
	"errors"
	"io"
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

	// PipeCloser models a stream implementation which MAY support propagation of a specific error as part of close.
	// See also io.Pipe.
	PipeCloser interface {
		CloseWithError(err error) error
	}

	// PipeReader models the read side of a stream. See also io.ReadCloser, PipeCloser, and io.Pipe.
	PipeReader interface {
		io.ReadCloser
		PipeCloser
	}

	// PipeWriter models the write side of a stream. See also io.WriteCloser, PipeCloser, and io.Pipe.
	PipeWriter interface {
		io.WriteCloser
		PipeCloser
	}

	// Pipe combines a PipeReader with a PipeWriter and an io.Closer, where all fields are optional.
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

// Close will pass through to the closer, then close the reader, then writer, skipping any that are nil.
func (x Pipe) Close() (err error) {
	if x.Writer != nil {
		defer func() { _ = x.Writer.CloseWithError(err) }()
	}
	if x.Reader != nil {
		defer func() { _ = x.Reader.CloseWithError(err) }()
	}
	if x.Closer != nil {
		err = ErrPanic
		err = x.Closer.Close()
	}
	return
}

// CloseWithError will pass through to the closer, then close reader with err, then close writer with err, skipping any
// that are nil.
func (x Pipe) CloseWithError(err error) error {
	if x.Writer != nil {
		defer func() { _ = x.Writer.CloseWithError(err) }()
	}
	if x.Reader != nil {
		defer func() { _ = x.Reader.CloseWithError(err) }()
	}
	if x.Closer != nil {
		return x.Closer.Close()
	}
	return nil
}

// Handle implements Handler using the receiver.
func (x HandlerFunc) Handle(r PipeReader, w PipeWriter) io.Closer { return x(r, w) }
