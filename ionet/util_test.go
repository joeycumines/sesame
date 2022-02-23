package ionet

import (
	"github.com/joeycumines/sesame/stream"
)

type (
	mockCloser struct {
		close func() error
	}

	mockWriter struct {
		write func(b []byte) (n int, err error)
	}

	mockPipeCloser struct {
		closeWithError func(err error) error
	}
)

func (x *mockCloser) Close() error { return x.close() }

func (x *mockWriter) Write(b []byte) (int, error) { return x.write(b) }

func (x *mockPipeCloser) CloseWithError(err error) error { return x.closeWithError(err) }

func mockPipeWriter(
	write func(b []byte) (n int, err error),
	close func() error,
	closeWithError func(err error) error,
) stream.PipeWriter {
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
