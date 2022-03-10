package stream

import (
	"io"
	"sync"
	"sync/atomic"
)

type (
	// GracefulCloser provides conditional execution of a "graceful" (less aggressive) io.Closer, for use with Pipe.
	GracefulCloser struct {
		pipe   Pipe
		closer io.Closer
		ok     int32
		once   sync.Once
	}

	gracefulCloserWrapper struct{ x *GracefulCloser }

	gracefulCloserWriter struct {
		pipeWriterI
		ok *int32
	}
)

var (
	// compile time assertions

	_ Piper      = (*GracefulCloser)(nil)
	_ io.Closer  = gracefulCloserWrapper{}
	_ PipeWriter = (*gracefulCloserWriter)(nil)
)

// NewGracefulCloser initialises a new GracefulCloser wrapping any existing Pipe.Closer, to prepend closer (with a
// guard). A panic will occur if closer is nil. See also GracefulCloser.
func NewGracefulCloser(pipe Pipe, closer io.Closer) *GracefulCloser {
	if closer == nil {
		panic(`sesame/stream: nil closer`)
	}
	r := GracefulCloser{
		pipe:   pipe,
		closer: closer,
	}
	closer = gracefulCloserWrapper{&r}
	if r.pipe.Closer != nil {
		closer = Closers(closer, r.pipe.Closer)
	}
	r.pipe.Closer = closer
	return &r
}

// EnableOnWriterClose will wrap the Pipe.Writer's Close and CloseWithError methods to call GracefulCloser.Enable on
// success (no error or panic), returning the receiver. A panic will occur if the writer is nil. The resultant Pipe
// will be available via GracefulCloser.Pipe, which should be called only after this method, if this method is used.
func (x *GracefulCloser) EnableOnWriterClose() *GracefulCloser {
	if x.pipe.Writer == nil {
		panic(`sesame/stream: nil writer`)
	}
	x.pipe.Writer = &gracefulCloserWriter{
		pipeWriterI: x.pipe.Writer,
		ok:          &x.ok,
	}
	return x
}

// Enable will allow the graceful closer to run, unless disable is called prior to close.
func (x *GracefulCloser) Enable() { atomic.StoreInt32(&x.ok, 1) }

// Disable will prevent the graceful closer from running (won't block or cancel if already running).
func (x *GracefulCloser) Disable() { x.once.Do(func() {}) }

// Pipe returns the modified pipe, any should be called after any modifications.
func (x *GracefulCloser) Pipe() Pipe { return x.pipe }

func (x gracefulCloserWrapper) Close() error {
	// deliberately not blocking in the closure
	var ok bool
	x.x.once.Do(func() { ok = atomic.LoadInt32(&x.x.ok) != 0 })
	if ok {
		return x.x.closer.Close()
	}
	return nil
}

func (x *gracefulCloserWriter) Close() (err error) {
	err = x.pipeWriterI.Close()
	if err == nil {
		atomic.StoreInt32(x.ok, 1)
	}
	return
}

func (x *gracefulCloserWriter) CloseWithError(err error) error {
	err = x.pipeWriterI.CloseWithError(err)
	if err == nil {
		atomic.StoreInt32(x.ok, 1)
	}
	return err
}
