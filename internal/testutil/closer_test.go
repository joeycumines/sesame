package testutil

import (
	"errors"
	"reflect"
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
	if err := c.Close(); err != errPanicDuringClose {
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
	if closer != closer {
		t.Error(closer)
	}
	m := map[interface{}]struct{}{}
	m[closer] = struct{}{}
	if _, ok := m[closer]; !ok {
		t.Fatal()
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
