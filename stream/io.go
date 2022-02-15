package stream

import (
	"fmt"
	"io"
	"sync"
)

type (
	// Closer implements io.Closer, also supporting wrapping to be idempotent/cached, via the Once method.
	Closer func() error

	// ChunkWriter implements io.Writer by wrapping another writer (e.g. io.Writer.Write), such that calls will be
	// chunked, at a maximum of ChunkSize bytes per chunk.
	ChunkWriter func(b []byte) (int, error)
)

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
