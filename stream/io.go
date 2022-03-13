package stream

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

type (
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

	// Closer implements io.Closer, also supporting wrapping to be idempotent/cached, via the Once method.
	Closer func() error

	// SequentialClosers implements io.Closer and will call it's values in order, bailing out on the first error.
	// See also Closers, which implements similar behavior, except it guarantees all closers will be attempted.
	// Unlike Closers, nil values are not ignored / handled, and will result in a panic.
	SequentialClosers []io.Closer

	// ChunkWriter implements io.Writer by wrapping another writer (e.g. io.Writer.Write), such that calls will be
	// chunked, at a maximum of ChunkSize bytes per chunk.
	ChunkWriter func(b []byte) (int, error)

	syncPipe struct {
		r      PipeReader
		w      PipeWriter
		mu     sync.RWMutex
		wakeup chan<- struct{}
		// size is used to track the data that was written but is yet to be read
		size *int64
	}

	syncPipeReader struct {
		*syncPipe
	}

	syncPipeWriter struct {
		*syncPipe
	}

	ioCloser io.Closer

	comparableCloser struct{ ioCloser }
)

var (
	// compile time assertions

	_ PipeReader = syncPipeReader{}
	_ PipeWriter = syncPipeWriter{}
	_ io.Closer  = Closer(nil)
	_ io.Closer  = SequentialClosers(nil)
)

// SyncPipe may be used to block writes until the reader requests additional data, or indicates that they are done (by
// closing their side of the pipe). It is primarily intended to be used with HalfCloser, addressing a variety of
// buffering-related use/problem cases.
func SyncPipe(r PipeReader, w PipeWriter) (PipeReader, PipeWriter) {
	if r == nil || w == nil {
		panic(`sesame/stream: expected non-nil pipes`)
	}
	if rs, ok := r.(syncPipeReader); ok {
		if ws, ok := w.(syncPipeWriter); ok && rs.syncPipe == ws.syncPipe {
			return r, w
		}
	}
	p := syncPipe{
		r: r,
		w: w,
	}
	return syncPipeReader{&p}, syncPipeWriter{&p}
}

// Closers combines a set of closers, always calling them in order, and using the last as the result.
func Closers(closers ...io.Closer) io.Closer {
	return Closer(func() (err error) {
		alwaysCallClosersOrdered(&err, closers...)
		return
	}).Comparable()
}

// alwaysCallClosersOrdered calls all closers in order, and combines their results (only uses the last error)
// note it doesn't call Close on any nil values, and err is a pointer is to ensure it gets set on panic
func alwaysCallClosersOrdered(err *error, closers ...io.Closer) {
	for i := len(closers) - 1; i >= 0; i-- {
		if closer := closers[i]; closer != nil {
			defer func() {
				var ok bool
				defer func() {
					if err != nil && *err == nil && !ok {
						*err = ErrPanic
					}
				}()
				if e := closer.Close(); err != nil && e != nil {
					*err = e
				}
				ok = true
			}()
		}
	}
}

func (x *syncPipe) sleep(size int64) <-chan struct{} {
	ch := make(chan struct{}, 1)
	x.mu.Lock()
	defer x.mu.Unlock()
	select {
	case x.wakeup <- struct{}{}:
	default:
	}
	x.wakeup = ch
	x.size = &size
	return ch
}

func (x *syncPipe) notify() {
	if x.size != nil {
		atomic.StoreInt64(x.size, 0)
	}
	select {
	case x.wakeup <- struct{}{}:
	default:
	}
}

func (x *syncPipe) notifyClose() {
	x.mu.RLock()
	defer x.mu.RUnlock()
	x.notify()
}

func (x *syncPipe) notifyRead() func(n int, err error) {
	var (
		size   *int64
		wakeup chan<- struct{}
	)
	x.mu.RLock()
	defer x.mu.RUnlock()
	if x.size != nil && atomic.LoadInt64(x.size) > 0 {
		// we have a write that's pending read
		size, wakeup = x.size, x.wakeup
	} else {
		// notify the _previous_ write
		x.notify()
	}
	return func(n int, err error) {
		notify := func() (ok bool) {
			if wakeup == nil {
				return
			}
			ok = true
			if size != nil {
				if atomic.AddInt64(size, -int64(n)) <= -int64(n) {
					// was already <= 0, we notify, and return false to also handle x.wakeup
					// (the case where the read started prior to write)
					ok = false
				} else if err == nil {
					// don't notify until the next read
					return
				}
			}
			select {
			case wakeup <- struct{}{}:
			default:
			}
			return
		}
		if notify() {
			return
		}
		x.mu.RLock()
		size, wakeup = x.size, x.wakeup
		x.mu.RUnlock()
		notify()
	}
}

func (x syncPipeReader) Read(p []byte) (n int, err error) {
	err = ErrPanic
	notifyRead := x.notifyRead()
	defer func() { notifyRead(n, err) }()
	return x.r.Read(p)
}

func (x syncPipeReader) Close() error {
	defer x.notifyClose()
	return x.r.Close()
}

func (x syncPipeReader) CloseWithError(err error) error {
	defer x.notifyClose()
	return x.r.CloseWithError(err)
}

func (x syncPipeWriter) Write(p []byte) (n int, err error) {
	sleep := x.sleep(int64(len(p)))
	n, err = x.w.Write(p)
	if err == nil && n > 0 {
		<-sleep
	}
	return
}

func (x syncPipeWriter) Close() error {
	defer x.notifyClose()
	return x.w.Close()
}

func (x syncPipeWriter) CloseWithError(err error) error {
	defer x.notifyClose()
	return x.w.CloseWithError(err)
}

// Close passes through to the receiver.
func (x Closer) Close() error { return x() }

// Once wraps the receiver in a closure guarded by sync.Once, which will record and return any error for subsequent
// close attempts. See also ErrPanic.
func (x Closer) Once() Closer {
	var (
		once sync.Once
		err  = ErrPanic
	)
	return func() error {
		once.Do(func() {
			err = x()
			x = nil
		})
		return err
	}
}

// Comparable wraps the receiver so that it is comparable.
func (x Closer) Comparable() io.Closer { return &comparableCloser{x} }

func (x SequentialClosers) Close() error {
	for _, v := range x {
		if err := v.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Write implements io.Writer, calling the receiver.
// Note that it will panic if the receiver's results are not sane.
func (x ChunkWriter) Write(b []byte) (n int, _ error) {
	for i := 0; i < len(b); i += ChunkSize {
		b := b[i:]
		if len(b) > ChunkSize {
			b = b[:ChunkSize]
		}
		m, err := x(b)
		if m < 0 || m > len(b) {
			panic(fmt.Errorf("sesame/stream: invalid count: %d", m))
		}
		n += m
		if err == nil && m != len(b) {
			err = io.ErrShortWrite
		}
		if err != nil {
			return n, err
		}
	}
	return
}
