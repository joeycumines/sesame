package stream

import (
	"io"
	"sync"
	"sync/atomic"
	"time"
)

type (
	// GracefulCloser provides conditional execution of a "graceful" (less aggressive) io.Closer, for use with Pipe.
	GracefulCloser struct {
		pipe    Pipe
		closer  io.Closer
		timeout *time.Duration
		ok      int32
		once    sync.Once
	}

	gracefulCloserWrapper struct{ c *GracefulCloser }

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
	return &GracefulCloser{
		pipe:   pipe,
		closer: closer,
	}
}

// WithClosePolicy applies a close policy to the graceful closer.
// It will panic if called more than once.
func (x *GracefulCloser) WithClosePolicy(policy ClosePolicy) *GracefulCloser {
	if x.timeout != nil {
		panic(`sesame/stream: graceful closer accepts at most one close policy`)
	}
	timeout := closePolicyTimeout(UnwrapClosePolicy(policy))
	x.timeout = &timeout
	return x
}

// EnableOnWriterClose will wrap the Pipe.Writer's Close and CloseWithError methods to call GracefulCloser.Enable on
// success (no error or panic), returning the receiver. A panic will occur if the writer is nil. The resultant Pipe
// will be available via GracefulCloser.Pipe, which should be called only after this method, if this method is used.
// It will panic if called more than once.
func (x *GracefulCloser) EnableOnWriterClose() *GracefulCloser {
	if x.pipe.Writer == nil {
		panic(`sesame/stream: nil writer`)
	}
	if v, ok := x.pipe.Writer.(*gracefulCloserWriter); ok && v.ok == &x.ok {
		panic(`sesame/stream: graceful closer already called enable on writer close`)
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
func (x *GracefulCloser) Pipe() (p Pipe) {
	p = x.pipe
	p.Closer = gracefulCloserWrapper{x}
	return
}

func (x gracefulCloserWrapper) Close() (err error) {
	var closer io.Closer
	if x.c.pipe.Closer != nil {
		closer = Closer(x.c.pipe.Closer.Close).Once()
	}
	alwaysCallClosersOrdered(&err, x.graceful(closer), closer)
	return
}

func (x gracefulCloserWrapper) graceful(closer io.Closer) io.Closer {
	return Closer(func() error {
		// deliberately not blocking in the closure
		var ok bool
		if x.c.timeout == nil || *x.c.timeout != 0 {
			// note that a timeout of 0 disables the graceful closer
			x.c.once.Do(func() { ok = atomic.LoadInt32(&x.c.ok) != 0 })
		}
		if !ok {
			return nil
		}
		if closer != nil && x.c.timeout != nil && *x.c.timeout > 0 {
			timer := time.NewTimer(*x.c.timeout)
			defer timer.Stop()
			done := make(chan struct{})
			defer close(done)
			go func() {
				select {
				case <-done:
				case <-timer.C:
					select {
					case <-done:
					default:
						// any error will be cached / this will be synchronised with the following call
						_ = closer.Close()
					}
				}
			}()
		}
		return x.c.closer.Close()
	})
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
