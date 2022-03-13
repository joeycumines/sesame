package stream

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/joeycumines/sesame/internal/testutil"
	"io"
	"math"
	"math/rand"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloser_panic(t *testing.T) {
	e := new(struct{})
	c := Closer(func() error { panic(e) }).Once()
	func() {
		defer func() {
			if r := recover(); r != e {
				t.Error(r)
			}
		}()
		_ = c.Close()
		t.Error(`expected panic`)
	}()
	if err := c.Close(); err != ErrPanic {
		t.Error(err)
	}
}

func TestCloser(t *testing.T) {
	var (
		e      = errors.New(`some error`)
		calls  uint32
		closer = Closer(func() error {
			atomic.AddUint32(&calls, 1)
			return e
		})
	)

	if err := closer.Close(); err != e {
		t.Fatal(err)
	}
	if atomic.LoadUint32(&calls) != 1 {
		t.Fatal(calls)
	}

	e = errors.New(`another error`)

	if err := closer.Close(); err != e {
		t.Fatal(err)
	}
	if atomic.LoadUint32(&calls) != 2 {
		t.Fatal(calls)
	}

	// same dealio after wrapping it with Closer.Once
	closer = closer.Once()
	e = errors.New(`yet another error`)

	var (
		start = make(chan struct{})
		wg    sync.WaitGroup
	)
	wg.Add(1)
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := closer.Close(); err != e {
				t.Error(err)
			}
			if atomic.LoadUint32(&calls) != 3 {
				t.Error(calls)
			}
		}()
	}
	wg.Done()
	time.Sleep(time.Millisecond * 100)
	close(start)
	wg.Wait()

	oldE := e
	e = errors.New(`this error wont be returned`)
	if err := closer.Close(); err != oldE {
		t.Error(err)
	}
	if atomic.LoadUint32(&calls) != 3 {
		t.Error(calls)
	}
}

func TestCloser_Comparable(t *testing.T) {
	e := errors.New(`some error`)
	fn := Closer(func() error { return e })
	closer := fn.Comparable()
	if v, ok := closer.(*comparableCloser); !ok || v.ioCloser == nil {
		t.Fatal(v, ok)
	} else if v, ok := v.ioCloser.(Closer); !ok || reflect.ValueOf(v).Pointer() != reflect.ValueOf(fn).Pointer() {
		t.Fatal(v, ok)
	}
	//lint:ignore SA4000 checking if it's comparable with itself
	if closer != closer {
		t.Error(closer)
	}
	m := map[interface{}]struct{}{}
	m[closer] = struct{}{}
	if _, ok := m[closer]; !ok {
		t.Fatal()
	}
}

func TestChunkWriter_Write_rand(t *testing.T) {
	r := rand.New(rand.NewSource(0))
	var data [ChunkSize*3 + ChunkSize*7/11]byte
	for i := range data {
		data[i] = byte(r.Intn(math.MaxUint8))
	}
	var b bytes.Buffer
	type onlyReader struct{ io.Reader }
	if n, err := io.CopyBuffer(ChunkWriter(b.Write), onlyReader{bytes.NewReader(data[:])}, make([]byte, len(data)*2)); err != nil || n != int64(len(data)) {
		t.Error(n, err)
	}
	if !bytes.Equal(b.Bytes(), data[:]) {
		t.Error(b.Bytes())
	}
}

func TestChunkWriter_Write_insane(t *testing.T) {
	for _, tc := range [...]struct {
		B1, B2 int
		N      int
	}{
		{1, 1, -1},
		{92462364, 32768, 32769},
		{5, 5, 6},
		{1, 1, 92462364},
		{1, 1, -92462364},
	} {
		t.Run(fmt.Sprintf(`%d_%d_%d`, tc.B1, tc.B2, tc.N), func(t *testing.T) {
			defer func() {
				r := recover()
				err, _ := r.(error)
				if err == nil || err.Error() != fmt.Sprintf("sesame/stream: invalid count: %d", tc.N) {
					t.Error(r)
				}
			}()
			_, _ = ChunkWriter(func(b []byte) (int, error) {
				if len(b) != tc.B2 {
					t.Error(len(b))
				}
				return tc.N, nil
			}).Write(make([]byte, tc.B1))
			t.Error(`expected panic`)
		})
	}
}

func TestChunkWriter_Write_shortWrite(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
	var (
		in   = make(chan []byte)
		out  = make(chan int)
		n    int
		err  error
		done = make(chan struct{})
	)
	go func() {
		defer close(done)
		n, err = ChunkWriter(func(b []byte) (int, error) {
			in <- b
			return <-out, nil
		}).Write(make([]byte, ChunkSize*3))
	}()
	if b := <-in; len(b) != ChunkSize {
		t.Error(b)
	}
	out <- ChunkSize
	if b := <-in; len(b) != ChunkSize {
		t.Error(b)
	}
	out <- ChunkSize - 1
	<-done
	if n != ChunkSize*2-1 {
		t.Error(n)
	}
	if err != io.ErrShortWrite {
		t.Error(err)
	}
}

func Test_alwaysCallClosersOrdered_overwrite(t *testing.T) {
	var (
		err error
		e1  = errors.New(`some error 1`)
		e2  = errors.New(`some error 2`)
		er  = new(struct{})
	)
	defer func() {
		if err != e2 {
			t.Error(err)
		}
		if r := recover(); r != er {
			t.Error(r)
		}
	}()
	alwaysCallClosersOrdered(
		&err,
		nil,
		nil,
		&mockCloser{func() error { return e1 }},
		nil,
		nil,
		&mockCloser{func() error { panic(er) }},
		nil,
		nil,
		&mockCloser{func() error {
			if err != e1 {
				t.Error(err)
			}
			if r := recover(); r != nil {
				t.Error(r)
			}
			return e2
		}},
		nil,
		&mockCloser{func() error { return nil }},
		nil,
	)
}

func TestSyncPipe_multiWrap(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)

	r, w := SyncPipe(io.Pipe())
	p := r.(syncPipeReader).syncPipe
	if w.(syncPipeWriter).syncPipe != p {
		t.Fatal()
	}

	for i := 0; i < 100; i++ {
		r, w = SyncPipe(r, w)
		if r != (syncPipeReader{syncPipe: p}) || w != (syncPipeWriter{syncPipe: p}) {
			t.Fatal()
		}
	}
}

func TestSyncPipe_readNoWrite(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
	r, w := SyncPipe(io.Pipe())
	go func() {
		time.Sleep(time.Millisecond * 100)
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	}()
	if n, err := r.Read(make([]byte, 1)); err != io.EOF || n != 0 {
		t.Error(n, err)
	}
}

func TestSyncPipe_chunkedRead(t *testing.T) {
	type TestCase struct {
		Close func(w PipeWriter)
		Err   error
	}
	for _, tc := range [...]struct {
		Name string
		Init func(t *testing.T) TestCase
	}{
		{
			Name: `close`,
			Init: func(t *testing.T) TestCase {
				return TestCase{
					Close: func(w PipeWriter) {
						if err := w.Close(); err != nil {
							t.Error(err)
						}
					},
					Err: io.EOF,
				}
			},
		},
		{
			Name: `close with error nil`,
			Init: func(t *testing.T) TestCase {
				return TestCase{
					Close: func(w PipeWriter) {
						if err := w.CloseWithError(nil); err != nil {
							t.Error(err)
						}
					},
					Err: io.EOF,
				}
			},
		},
		{
			Name: `close with error val`,
			Init: func(t *testing.T) TestCase {
				err := errors.New(`some error`)
				return TestCase{
					Close: func(w PipeWriter) {
						if err := w.CloseWithError(err); err != nil {
							t.Error(err)
						}
					},
					Err: err,
				}
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
			tc := tc.Init(t)
			r, w := SyncPipe(io.Pipe())
			p := r.(syncPipeReader).syncPipe
			if w.(syncPipeWriter).syncPipe != p {
				t.Fatal()
			}
			if p.size != nil || p.wakeup != nil {
				t.Fatal()
			}

			want := make([]byte, 1<<20)
			rand.New(rand.NewSource(0)).Read(want)

			got := make([]byte, len(want))
			var size int
			read := func(n int) error {
				t.Helper()
				if size+n > len(got) {
					n = len(got) - size
				}
				n1, err := r.Read(got[size : size+n])
				if n1 != n {
					t.Errorf(`expected n %d got %d`, n, n1)
				}
				size += n1
				return err
			}

			writeDone := make(chan struct{})
			go func() {
				defer close(writeDone)
				if n, err := w.Write(want); n != len(want) || err != nil {
					t.Error(n, err)
				}
				tc.Close(w)
			}()

			// wait for the write to start
			var (
				sp         *int64
				wakeupOrig chan<- struct{}
				wakeup     = make(chan struct{}, 1)
			)
			{
				const (
					d = time.Millisecond
					m = time.Second
				)
				for r := m; r > -d; r -= d {
					p.mu.Lock()
					sp = p.size
					wakeupOrig = p.wakeup
					if sp != nil {
						p.wakeup = wakeup
					}
					p.mu.Unlock()
					if sp != nil {
						if wakeupOrig == nil {
							t.Error()
						}
						break
					} else if wakeupOrig != nil {
						t.Error()
					}
					time.Sleep(d)
				}
			}
			if sp == nil {
				t.Fatal()
			}

			if sp := atomic.LoadInt64(sp); sp != int64(len(want)) {
				t.Errorf(`expected sp %d got %d`, len(want), sp)
			}

			checkStillWriting := func() {
				t.Helper()
				p.mu.RLock()
				defer p.mu.RUnlock()
				if p.size != sp {
					t.Error(`p.size pointer has changed`)
					defer t.FailNow()
				}
				if reflect.ValueOf(p.wakeup).Pointer() != reflect.ValueOf(wakeup).Pointer() {
					t.Error(`p.wakeup pointer has changed`)
					defer t.FailNow()
				}
				if sp, ex := atomic.LoadInt64(sp), int64(len(want)-size); sp != ex || sp <= 0 {
					t.Errorf(`sp != %d: %d`, ex, sp)
					defer t.FailNow()
				}
				select {
				case <-wakeup:
					t.Error(`wakeup received value`)
					defer t.FailNow()
				default:
				}
				select {
				case <-writeDone:
					t.Error(`writeDone is closed`)
					defer t.FailNow()
				default:
				}
			}
			checkStillWriting()

			{
				const maxChunk = 128
				rnd := rand.New(rand.NewSource(1))
				var i int
				for size < len(want)-(maxChunk*2) {
					if err := read(rnd.Intn(maxChunk) + 1); err != nil {
						t.Fatal(err)
					}
					checkStillWriting()
					i++
				}
				t.Logf(`read %d chunks with %d of %d bytes remaining`, i, len(want)-size, len(want))
			}

			time.Sleep(time.Millisecond * 300)
			checkStillWriting()

			if err := read(len(want)); err != nil {
				t.Error(err)
			}

			if !bytes.Equal(want[:size], got[:size]) {
				t.Error(`unexpected data`)
			}
			if size != len(want) {
				t.Errorf(`only read %d of %d`, size, len(want))
			}

			if p.size != sp {
				t.Error(`p.size pointer has changed`)
			}
			if reflect.ValueOf(p.wakeup).Pointer() != reflect.ValueOf(wakeup).Pointer() {
				t.Error(`p.wakeup pointer has changed`)
			}
			if sp := atomic.LoadInt64(sp); sp != 0 {
				t.Errorf(`sp isn't 0: %d`, sp)
			}

			time.Sleep(time.Millisecond * 100)
			select {
			case <-wakeup:
				t.Error(`wakeup received`)
			default:
			}
			select {
			case <-writeDone:
				t.Fatal(`writeDone is closed`)
			default:
			}

			doneWakeup := make(chan struct{})
			go func() {
				defer close(doneWakeup)
				<-wakeup
				wakeupOrig <- struct{}{}
			}()
			if n, err := r.Read(make([]byte, 1)); err != tc.Err || n != 0 {
				t.Error(n, err)
			}
			<-doneWakeup
			<-writeDone
		})
	}
}

func TestSyncPipe_raceReaders(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
	r, w := SyncPipe(io.Pipe())
	const (
		count     = 1 << 8
		countPre  = count / 8
		countPost = count - countPre
	)
	ch := make(chan byte)
	reader := func() {
		var b [1]byte
		if n, err := r.Read(b[:]); err != nil || n != 1 {
			t.Error(n, err)
		}
		ch <- b[0]
	}
	for i := 0; i < countPre; i++ {
		go reader()
	}
	done := make(chan struct{})
	{
		var b [count]byte
		for i := range b {
			b[i] = byte(i)
		}
		go func() {
			defer close(done)
			if n, err := w.Write(b[:]); err != nil || n != count {
				t.Error(n, err)
			}
		}()
	}
	for i := 0; i < countPost; i++ {
		go reader()
	}
	m := make(map[byte]struct{}, count)
	for i := 0; i < count; i++ {
		m[<-ch] = struct{}{}
	}
	if len(m) != count {
		t.Errorf("unexpected map len %d: %+v", len(m), m)
	}
	time.Sleep(time.Millisecond * 10)
	select {
	case <-done:
		t.Fatal(`unexpectedly done`)
	default:
	}
	if err := r.Close(); err != nil {
		t.Error(err)
	}
	<-done
}

func TestSyncPipe_notifyOnReadError(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
	r1, w1 := io.Pipe()
	e := errors.New(`some error`)
	r2, w2 := SyncPipe(mockPipeReader(
		func(b []byte) (n int, err error) { return 0, e },
		r1.Close,
		r1.CloseWithError,
	), w1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if n, err := w2.Write([]byte(`123`)); err != nil || n != 3 {
			t.Error(n, err)
		}
	}()
	if n, err := r1.Read(make([]byte, 1<<16)); err != nil || n != 3 {
		t.Fatal(n, err)
	}
	time.Sleep(time.Millisecond * 50)
	select {
	case <-done:
		t.Fatal(`expected not done`)
	default:
	}
	if n, err := r2.Read(nil); err != e || n != 0 {
		t.Error(n, err)
	}
	<-done
}

func TestSyncPipe_notifyOnPanic(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
	r1, w1 := io.Pipe()
	e := errors.New(`some error`)
	r2, w2 := SyncPipe(mockPipeReader(
		func(b []byte) (n int, err error) { panic(e) },
		r1.Close,
		r1.CloseWithError,
	), w1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if n, err := w2.Write([]byte(`123`)); err != nil || n != 3 {
			t.Error(n, err)
		}
	}()
	if n, err := r1.Read(make([]byte, 1<<16)); err != nil || n != 3 {
		t.Fatal(n, err)
	}
	time.Sleep(time.Millisecond * 50)
	select {
	case <-done:
		t.Fatal(`expected not done`)
	default:
	}
	func() {
		defer func() {
			if r := recover(); r != e {
				t.Error(r)
			}
		}()
		_, _ = r2.Read(nil)
		t.Error(`expected panic`)
	}()
	<-done
}

func TestClosers(t *testing.T) {
	var calls string
	if err := Closers(
		&mockCloser{func() error {
			calls += "1\n"
			return nil
		}},
		&mockCloser{func() error {
			calls += "2\n"
			return nil
		}},
		&mockCloser{func() error {
			calls += "3\n"
			return nil
		}},
	).Close(); err != nil {
		t.Error(err)
	}
	if calls != "1\n2\n3\n" {
		t.Errorf("unexpected calls: %q\n%s", calls, calls)
	}
}
