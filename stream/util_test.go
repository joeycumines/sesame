package stream

import (
	"io"
	"math/rand"
	"sync"
	"sync/atomic"
)

type (
	atomicReaderSize struct {
		reader io.Reader
		size   *int64
	}

	atomicWriterSize struct {
		writer io.Writer
		size   *int64
	}

	lockedRand struct {
		mu   sync.Mutex
		rand *rand.Rand
	}
)

func trackReadWriteCloserSize(p io.ReadWriteCloser, r, w *int64) io.ReadWriteCloser {
	type (
		pi io.ReadWriteCloser
		ps struct{ pi }
	)
	return struct {
		ps
		atomicReaderSize
		atomicWriterSize
	}{
		ps: ps{pi: p},
		atomicReaderSize: atomicReaderSize{
			reader: p,
			size:   r,
		},
		atomicWriterSize: atomicWriterSize{
			writer: p,
			size:   w,
		},
	}
}

func trackPipeReaderSize(p PipeReader, size *int64) PipeReader {
	type (
		pi PipeReader
		ps struct{ pi }
	)
	return struct {
		ps
		atomicReaderSize
	}{
		ps: ps{pi: p},
		atomicReaderSize: atomicReaderSize{
			reader: p,
			size:   size,
		},
	}
}

func trackPipeWriterSize(p PipeWriter, size *int64) PipeWriter {
	type (
		pi PipeWriter
		ps struct{ pi }
	)
	return struct {
		ps
		atomicWriterSize
	}{
		ps: ps{pi: p},
		atomicWriterSize: atomicWriterSize{
			writer: p,
			size:   size,
		},
	}
}

func (x atomicReaderSize) Read(b []byte) (n int, err error) {
	n, err = x.reader.Read(b)
	atomic.AddInt64(x.size, int64(n))
	return
}

func (x atomicWriterSize) Write(b []byte) (n int, err error) {
	n, err = x.writer.Write(b)
	atomic.AddInt64(x.size, int64(n))
	return
}

func (r *lockedRand) Seed(seed int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rand.Seed(seed)
}

func (r *lockedRand) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rand.Read(p)
}

func (r *lockedRand) Intn(n int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rand.Intn(n)
}
