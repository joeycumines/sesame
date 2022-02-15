package stream

import (
	"errors"
)

const (
	// ChunkSize is 32 kilobytes, and is provided for use in situations where an arbitrary chunk size is required.
	ChunkSize = 1 << 15
)

var (
	// ErrPanic indicates that a panic took place, and is used as a sentinel value in certain cases, e.g. as the result
	// of Closer.Close following a panic during the first attempt, for caching / idempotent closers generated using
	// Closer.Once.
	ErrPanic = errors.New(`sesame/stream: unhandled panic`)
)
