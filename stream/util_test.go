package stream

import (
	"io"
	"math/rand"
	"sync"
	"sync/atomic"
)

type (
	mockCloser struct {
		close func() error
	}

	mockReader struct {
		read func(b []byte) (n int, err error)
	}

	mockWriter struct {
		write func(b []byte) (n int, err error)
	}

	mockPipeCloser struct {
		closeWithError func(err error) error
	}

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

	naiveHalfCloser struct{ Pipe }
)

func mockPipeReader(
	read func(b []byte) (n int, err error),
	close func() error,
	closeWithError func(err error) error,
) PipeReader {
	return &struct {
		mockReader
		mockCloser
		mockPipeCloser
	}{
		mockReader{read},
		mockCloser{close},
		mockPipeCloser{closeWithError},
	}
}

func mockPipeWriter(
	write func(b []byte) (n int, err error),
	close func() error,
	closeWithError func(err error) error,
) PipeWriter {
	return &struct {
		mockWriter
		mockCloser
		mockPipeCloser
	}{
		mockWriter{write},
		mockCloser{close},
		mockPipeCloser{closeWithError},
	}
}

func (x *mockCloser) Close() error { return x.close() }

func (x *mockReader) Read(b []byte) (int, error) { return x.read(b) }

func (x *mockWriter) Write(b []byte) (int, error) { return x.write(b) }

func (x *mockPipeCloser) CloseWithError(err error) error { return x.closeWithError(err) }

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

func (x *lockedRand) Seed(seed int64) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.rand.Seed(seed)
}

func (x *lockedRand) Read(p []byte) (n int, err error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.rand.Read(p)
}

func (x *lockedRand) Intn(n int) int {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.rand.Intn(n)
}

func (x naiveHalfCloser) Close() error { return x.Pipe.Writer.Close() }
