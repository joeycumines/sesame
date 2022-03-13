package stream

import (
	"errors"
	"github.com/joeycumines/sesame/internal/testutil"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewGracefulCloser_nilCloser(t *testing.T) {
	defer func() {
		if r := recover(); r != `sesame/stream: nil closer` {
			t.Error(r)
		}
	}()
	NewGracefulCloser(Pipe{}, nil)
	t.Error(`expected panic`)
}

func TestNewGracefulCloser_empty(t *testing.T) {
	var closes int
	e := errors.New(`some error`)
	c := &mockCloser{close: func() error {
		closes++
		return e
	}}
	g := NewGracefulCloser(Pipe{}, c)
	p := g.pipe
	if p.Writer != nil || p.Reader != nil || p.Closer != nil {
		t.Fatal(g)
	}
	p.Closer = gracefulCloserWrapper{g}
	if g.Pipe() != p {
		t.Fatal()
	}
	if g.pipe != (Pipe{Writer: p.Writer, Reader: p.Reader}) {
		t.Fatal()
	}
	if g.closer != c {
		t.Fatal(g.closer)
	}
	if g.ok != 0 {
		t.Fatal(g.ok)
	}
	g.Enable()
	if g.ok != 1 {
		t.Fatal(g.ok)
	}
	if err := p.Close(); err != e {
		t.Error(err)
	}
	if err := p.Close(); err != nil {
		t.Error(err)
	}
	g.Disable()
	g.Disable()
	if closes != 1 {
		t.Error(closes)
	}
}

func TestGracefulCloser_Disable_basic(t *testing.T) {
	g := NewGracefulCloser(Pipe{}, &mockCloser{close: func() error {
		t.Error(`bad`)
		return errors.New(`bad`)
	}})
	p := g.Pipe()
	g.Enable()
	g.Disable()
	if err := p.Close(); err != nil {
		t.Error(err)
	}
}

func TestGracefulCloser_neverEnabled(t *testing.T) {
	g := NewGracefulCloser(Pipe{}, &mockCloser{close: func() error {
		t.Error(`bad`)
		return errors.New(`bad`)
	}})
	p := g.Pipe()
	if err := p.Close(); err != nil {
		t.Error(err)
	}
}

func TestGracefulCloser_Disable_noBlock(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
	in := make(chan struct{})
	out := make(chan struct{})
	g := NewGracefulCloser(Pipe{}, &mockCloser{close: func() error {
		in <- struct{}{}
		<-out
		return nil
	}})
	p := g.Pipe()
	g.Enable()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	}()
	<-in
	time.Sleep(time.Millisecond * 30)
	select {
	case <-done:
		t.Fatal()
	default:
	}
	g.Disable()
	out <- struct{}{}
	<-done
}

func TestNewGracefulCloser_closerOrder(t *testing.T) {
	var i int
	e1 := errors.New(`e1`)
	e2 := errors.New(`e2`)
	g := NewGracefulCloser(
		Pipe{Closer: Closer(func() error {
			if i != 1 {
				t.Error(i)
			}
			i++
			return e2
		})},
		&mockCloser{close: func() error {
			if i != 0 {
				t.Error(i)
			}
			i++
			return e1
		}},
	)
	g.Enable()
	if err := g.Pipe().Close(); err != e2 {
		t.Error(err)
	}
	if i != 2 {
		t.Error(i)
	}
}

func TestGracefulCloser_EnableOnWriterClose_nilWriter(t *testing.T) {
	g := NewGracefulCloser(Pipe{}, &mockCloser{func() error { return nil }})
	defer func() {
		if r := recover(); r != `sesame/stream: nil writer` {
			t.Error(r)
		}
	}()
	g.EnableOnWriterClose()
	t.Error(`expected panic`)
}

func TestGracefulCloser_EnableOnWriterClose(t *testing.T) {
	ch := make(chan int, 10)
	e := errors.New(`nah`)
	e2 := errors.New(`in`)
	w := mockPipeWriter(
		nil,
		func() error {
			ch <- 1
			return e
		},
		func(err error) error {
			if err != e2 {
				t.Error(err)
			}
			ch <- 2
			return e
		},
	)
	g := NewGracefulCloser(Pipe{Writer: w}, &mockCloser{})
	c := g.Pipe().Closer
	if w != g.Pipe().Writer {
		t.Error(g.Pipe())
	}
	g.EnableOnWriterClose()
	if c != g.Pipe().Closer {
		t.Error(g.Pipe())
	}
	a := g.Pipe().Writer.(*gracefulCloserWriter)
	if a.ok != &g.ok {
		t.Error(a.ok)
	}
	if a.pipeWriterI != w {
		t.Error(w)
	}
	select {
	case v := <-ch:
		t.Fatal(v)
	default:
	}

	if err := g.Pipe().Writer.Close(); err != e {
		t.Fatal(err)
	}
	select {
	case v := <-ch:
		if v != 1 {
			t.Fatal(v)
		}
	default:
		t.Fatal()
	}
	if v := atomic.LoadInt32(&g.ok); v != 0 {
		t.Fatal(v)
	}

	if err := g.Pipe().Writer.CloseWithError(e2); err != e {
		t.Fatal(err)
	}
	select {
	case v := <-ch:
		if v != 2 {
			t.Fatal(v)
		}
	default:
		t.Fatal()
	}
	if v := atomic.LoadInt32(&g.ok); v != 0 {
		t.Fatal(v)
	}

	e = nil

	if err := g.Pipe().Writer.Close(); err != e {
		t.Fatal(err)
	}
	select {
	case v := <-ch:
		if v != 1 {
			t.Fatal(v)
		}
	default:
		t.Fatal()
	}
	if v := atomic.LoadInt32(&g.ok); v != 1 {
		t.Fatal(v)
	}

	g.ok = 0

	if err := g.Pipe().Writer.CloseWithError(e2); err != e {
		t.Fatal(err)
	}
	select {
	case v := <-ch:
		if v != 2 {
			t.Fatal(v)
		}
	default:
		t.Fatal()
	}
	if v := atomic.LoadInt32(&g.ok); v != 1 {
		t.Fatal(v)
	}

	close(ch)
	if v := <-ch; v != 0 {
		t.Error(v)
	}
}
