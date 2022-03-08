package testutil

import (
	"errors"
	"io"
	"sync"
)

type (
	// Closer implements io.Closer, also supporting wrapping to be idempotent/cached, via the Once method.
	//
	// Note this is a copy of stream.Closer.
	Closer func() error

	ioCloser io.Closer

	comparableCloser struct{ ioCloser }
)

var (
	errPanicDuringClose = errors.New(`panic during close`)
)

// Closers combines a set of closers, always calling them in order, and using the last as the result.
//
// Note this is a copy of stream.Closers.
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
						*err = errPanicDuringClose
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

// Close passes through to the receiver.
func (x Closer) Close() error { return x() }

// Once wraps the receiver in a closure guarded by sync.Once, which will record and return any error for subsequent
// close attempts. See also ErrPanic.
func (x Closer) Once() Closer {
	var (
		once sync.Once
		err  = errPanicDuringClose
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
