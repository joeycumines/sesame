package pipelistener

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
)

func TestNewPipeListener_factoryError(t *testing.T) {
	e1 := errors.New(`e1`)
	l := NewPipeListener(func(ctx context.Context) (c1, c2 net.Conn, _ error) { return nil, nil, e1 })
	if l.factory == nil || l.stop == nil || l.ch == nil {
		t.Error(`unexpected result`)
	}
	if c, err := l.DialContext(context.Background()); err != e1 || c != nil {
		t.Error(c, err)
	}
	select {
	case <-l.stop:
		t.Error()
	default:
	}
	if err := l.Close(); err != nil {
		t.Error(err)
	}
	select {
	case <-l.stop:
	default:
		t.Error()
	}
	if err := l.Close(); err != nil {
		t.Error(err)
	}
	if c, err := l.DialContext(context.Background()); err != io.ErrClosedPipe || c != nil {
		t.Error(c, err)
	}
	if c, err := l.Accept(); err != io.ErrClosedPipe || c != nil {
		t.Error(c, err)
	}
}

func TestPipeListener_Addr(t *testing.T) {
	a := (*PipeListener)(nil).Addr()
	if a == nil {
		t.Fatal()
	}
	if v := a.Network(); v != `pipe` {
		t.Error(v)
	}
	if v := a.String(); v != `pipe` {
		t.Error(v)
	}
}
