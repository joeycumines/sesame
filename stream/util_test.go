package stream

import (
	"io"
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
