package stream

import (
	"errors"
	"github.com/joeycumines/sesame/internal/testutil"
	"io"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

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
	rErr := errors.New(`r error`)
	rOut <- rErr
	if err := <-wIn; err != rErr {
		t.Error(err)
	}
	wErr := errors.New(`w error`)
	wOut <- wErr
	if err := <-out; err != wErr {
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
	rErr := errors.New(`r error`)
	rOut <- rErr
	if err := <-wIn; err != rErr {
		t.Error(err)
	}
	wErr := errors.New(`w error`)
	wOut <- wErr
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
	wErr := errors.New(`w error`)
	wOut <- wErr
	if err := <-out; err != wErr {
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

func TestPair(t *testing.T) {
	t.Run(`local to remote`, func(t *testing.T) {
		local, remote := Pair(io.Pipe())(io.Pipe())
		testBasicIO(t, local, remote)
	})
	t.Run(`remote to local`, func(t *testing.T) {
		local, remote := Pair(io.Pipe())(io.Pipe())
		testBasicIO(t, remote, local)
	})
}

func TestWrap_basicIO(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)

	initHalfCloser := func(fn func(pipe Pipe) []HalfCloserOption) func(t *testing.T) (io.ReadWriteCloser, io.ReadWriteCloser, func()) {
		return func(t *testing.T) (io.ReadWriteCloser, io.ReadWriteCloser, func()) {
			var (
				localR, localW                     int64
				remoteR, remoteW                   int64
				localHalfCloserR, localHalfCloserW int64
				localPipeR, localPipeW             int64
			)
			local, remote := Pair(SyncPipe(io.Pipe()))(SyncPipe(io.Pipe()))
			localHalfCloser, err := NewHalfCloser(fn(Pipe{
				Reader: trackPipeReaderSize(local.Reader, &localR),
				Writer: trackPipeWriterSize(local.Writer, &localW),
				Closer: local.Closer,
			})...)
			if err != nil {
				t.Fatal(err)
			}
			localPipe := Wrap(io.Pipe())(io.Pipe())(trackReadWriteCloserSize(localHalfCloser, &localHalfCloserR, &localHalfCloserW))
			if localPipe.Closer == nil {
				t.Fatal()
			}
			return trackReadWriteCloserSize(localPipe, &localPipeR, &localPipeW),
				trackReadWriteCloserSize(remote, &remoteR, &remoteW),
				func() {
					t.Logf(
						"localR, localW = %d, %d\nremoteR, remoteW = %d, %d\nlocalHalfCloserR, localHalfCloserW = %d, %d\nlocalPipeR, localPipeW = %d, %d",
						atomic.LoadInt64(&localR), atomic.LoadInt64(&localW),
						atomic.LoadInt64(&remoteR), atomic.LoadInt64(&remoteW),
						atomic.LoadInt64(&localHalfCloserR), atomic.LoadInt64(&localHalfCloserW),
						atomic.LoadInt64(&localPipeR), atomic.LoadInt64(&localPipeW),
					)
				}
		}
	}

	for _, tc := range [...]struct {
		Name string
		Init func(t *testing.T) (io.ReadWriteCloser, io.ReadWriteCloser, func())
	}{
		{
			Name: `wrap io pipe pair`,
			Init: func(t *testing.T) (io.ReadWriteCloser, io.ReadWriteCloser, func()) {
				var (
					localR, localW         int64
					remoteR, remoteW       int64
					localPipeR, localPipeW int64
				)
				local, remote := Pair(SyncPipe(io.Pipe()))(SyncPipe(io.Pipe()))
				localPipe := Wrap(io.Pipe())(io.Pipe())(trackReadWriteCloserSize(naiveHalfCloser{local}, &localR, &localW))
				if localPipe.Closer == nil {
					t.Fatal()
				}
				return trackReadWriteCloserSize(localPipe, &localPipeR, &localPipeW),
					trackReadWriteCloserSize(remote, &remoteR, &remoteW),
					func() {
						t.Logf(
							"localR, localW = %d, %d\nremoteR, remoteW = %d, %d\nlocalPipeR, localPipeW = %d, %d",
							atomic.LoadInt64(&localR), atomic.LoadInt64(&localW),
							atomic.LoadInt64(&remoteR), atomic.LoadInt64(&remoteW),
							atomic.LoadInt64(&localPipeR), atomic.LoadInt64(&localPipeW),
						)
					}
			},
		},
		{
			Name: `half closer default`,
			Init: initHalfCloser(func(pipe Pipe) []HalfCloserOption {
				return []HalfCloserOption{
					OptHalfCloser.Pipe(pipe),
				}
			}),
		},
		{
			Name: `half closer wait`,
			Init: initHalfCloser(func(pipe Pipe) []HalfCloserOption {
				return []HalfCloserOption{
					OptHalfCloser.Pipe(pipe),
					OptHalfCloser.ClosePolicy(WaitRemote{}),
				}
			}),
		},
		{
			Name: `half closer 10s timeout`,
			Init: initHalfCloser(func(pipe Pipe) []HalfCloserOption {
				return []HalfCloserOption{
					OptHalfCloser.Pipe(pipe),
					OptHalfCloser.ClosePolicy(WaitRemoteTimeout(time.Second * 10)),
				}
			}),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			t.Run(`a`, func(t *testing.T) {
				defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
				local, remote, cleanup := tc.Init(t)
				defer cleanup()
				testBasicIO(t, local, remote)
			})

			t.Run(`b`, func(t *testing.T) {
				defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
				local, remote, cleanup := tc.Init(t)
				defer cleanup()
				testBasicIO(t, remote, local)
			})
		})
	}
}
