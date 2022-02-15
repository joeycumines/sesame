package stream

import (
	"errors"
	"github.com/joeycumines/sesame/internal/testutil"
	"io"
	"runtime"
	"testing"
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
func (x *mockCloser) Close() error                       { return x.close() }
func (x *mockReader) Read(b []byte) (int, error)         { return x.read(b) }
func (x *mockWriter) Write(b []byte) (int, error)        { return x.write(b) }
func (x *mockPipeCloser) CloseWithError(err error) error { return x.closeWithError(err) }

func TestPipe_Close_noFieldsSet(t *testing.T) {
	if err := (&Pipe{}).Close(); err != nil {
		t.Error(err)
	}
}

func TestPipe_Close_allFieldsSet(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)

	rIn := make(chan error)
	rOut := make(chan error)
	r := mockPipeReader(nil, nil, func(err error) error {
		rIn <- err
		return <-rOut
	})

	wIn := make(chan error)
	wOut := make(chan error)
	w := mockPipeWriter(nil, nil, func(err error) error {
		wIn <- err
		return <-wOut
	})

	cIn := make(chan struct{})
	cOut := make(chan error)
	c := &mockCloser{func() error {
		cIn <- struct{}{}
		return <-cOut
	}}

	out := make(chan error)
	go func() {
		out <- (&Pipe{
			Reader: r,
			Writer: w,
			Closer: c,
		}).Close()
	}()

	<-cIn
	e := errors.New(`some error`)
	cOut <- e
	if err := <-rIn; err != e {
		t.Error(err)
	}
	rOut <- errors.New(`r error`)
	if err := <-wIn; err != e {
		t.Error(err)
	}
	wOut <- errors.New(`w error`)
	if err := <-out; err != e {
		t.Error(err)
	}
}

func TestPipe_Close_panic(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)

	rIn := make(chan error)
	rOut := make(chan error)
	r := mockPipeReader(nil, nil, func(err error) error {
		rIn <- err
		return <-rOut
	})

	wIn := make(chan error)
	wOut := make(chan error)
	w := mockPipeWriter(nil, nil, func(err error) error {
		wIn <- err
		return <-wOut
	})

	cIn := make(chan struct{})
	cOut := make(chan interface{})
	c := &mockCloser{func() error {
		cIn <- struct{}{}
		panic(<-cOut)
	}}

	out := make(chan interface{})
	go func() {
		defer func() { out <- recover() }()
		_ = (&Pipe{
			Reader: r,
			Writer: w,
			Closer: c,
		}).Close()
		t.Error(`expected a panic`)
	}()

	<-cIn
	e := errors.New(`some error`)
	cOut <- e
	if err := <-rIn; err != ErrPanic {
		t.Error(err)
	}
	rOut <- errors.New(`r error`)
	if err := <-wIn; err != ErrPanic {
		t.Error(err)
	}
	wOut <- errors.New(`w error`)
	if err := <-out; err != e {
		t.Error(err)
	}
}

func TestHandlerFunc_Handle(t *testing.T) {
	er := mockPipeReader(nil, nil, nil)
	ew := mockPipeWriter(nil, nil, nil)
	ec := &mockCloser{}
	if c := HandlerFunc(func(r PipeReader, w PipeWriter) io.Closer {
		if r != er {
			t.Error(r)
		}
		if w != ew {
			t.Error(w)
		}
		return ec
	}).Handle(er, ew); c != ec {
		t.Error(c)
	}
}

func TestPipe_CloseWithError_noFieldsSet(t *testing.T) {
	if err := (&Pipe{}).CloseWithError(errors.New(`some error`)); err != nil {
		t.Error(err)
	}
}

func TestPipe_CloseWithError_allFieldsSet(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)

	rIn := make(chan error)
	rOut := make(chan error)
	r := mockPipeReader(nil, nil, func(err error) error {
		rIn <- err
		return <-rOut
	})

	wIn := make(chan error)
	wOut := make(chan error)
	w := mockPipeWriter(nil, nil, func(err error) error {
		wIn <- err
		return <-wOut
	})

	cIn := make(chan struct{})
	cOut := make(chan error)
	c := &mockCloser{func() error {
		cIn <- struct{}{}
		return <-cOut
	}}

	e1 := errors.New(`some error`)
	out := make(chan error)
	go func() {
		out <- (&Pipe{
			Reader: r,
			Writer: w,
			Closer: c,
		}).CloseWithError(e1)
	}()

	<-cIn
	e2 := errors.New(`c error`)
	cOut <- e2
	if err := <-rIn; err != e1 {
		t.Error(err)
	}
	rOut <- errors.New(`r error`)
	if err := <-wIn; err != e1 {
		t.Error(err)
	}
	wOut <- errors.New(`w error`)
	if err := <-out; err != e2 {
		t.Error(err)
	}
}

func TestPipe_Read_nil(t *testing.T) {
	b := [...]byte{123}
	if n, err := (Pipe{}).Read(b[:]); err != io.EOF || n != 0 {
		t.Error(n, err)
	}
	if b[0] != 123 {
		t.Error(b)
	}
}

func TestPipe_Write_nil(t *testing.T) {
	b := [...]byte{123}
	if n, err := (Pipe{}).Write(b[:]); err != io.ErrClosedPipe || n != 0 {
		t.Error(n, err)
	}
	if b[0] != 123 {
		t.Error(b)
	}
}

func TestPipe_Read_pass(t *testing.T) {
	b := [...]byte{123, 7}
	if n, err := (Pipe{
		Reader: mockPipeReader(func(b []byte) (n int, err error) {
			if len(b) != 2 || b[0] != 123 || b[1] != 7 {
				t.Error(b)
			} else {
				b[0] = 5
			}
			return 1, io.ErrUnexpectedEOF
		}, nil, nil),
	}).Read(b[:]); err != io.ErrUnexpectedEOF || n != 1 {
		t.Error(n, err)
	}
	if b[0] != 5 || b[1] != 7 {
		t.Error(b)
	}
}

func TestPipe_Write_pass(t *testing.T) {
	b := [...]byte{123, 7}
	if n, err := (Pipe{
		Writer: mockPipeWriter(func(b []byte) (n int, err error) {
			if len(b) != 2 || b[0] != 123 || b[1] != 7 {
				t.Error(b)
			}
			return 1, io.ErrShortWrite
		}, nil, nil),
	}).Write(b[:]); err != io.ErrShortWrite || n != 1 {
		t.Error(n, err)
	}
	if b[0] != 123 || b[1] != 7 {
		t.Error(b)
	}
}
