package stream

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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
