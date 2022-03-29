package pipelistener

import (
	"context"
	"io"
	"net"
	"sync"
)

type (
	// PipeListener implements net.Listener for in-memory pipes.
	PipeListener struct {
		factory PipeFactory
		once    sync.Once
		stop    chan struct{}
		ch      chan net.Conn
	}

	// PipeFactory models an implementation that can construct pipes.
	PipeFactory func(ctx context.Context) (listener, dialer net.Conn, _ error)

	pipeAddr struct{}
)

var (
	// compile time assertions

	_ net.Listener = (*PipeListener)(nil)
)

// NewPipeListener returns a net.Listener that can only be contacted by its own Dialers and
// creates buffered connections between the two.
func NewPipeListener(factory PipeFactory) *PipeListener {
	if factory == nil {
		panic(`sesame/ionet: nil pipe factory`)
	}
	return &PipeListener{
		factory: factory,
		stop:    make(chan struct{}),
		ch:      make(chan net.Conn),
	}
}

func (x *PipeListener) DialContext(ctx context.Context) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-x.stop:
		return nil, io.ErrClosedPipe
	default:
	}
	listener, dialer, err := x.factory(ctx)
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-x.stop:
		return nil, io.ErrClosedPipe
	case x.ch <- listener:
		return dialer, nil
	}
}

func (x *PipeListener) Accept() (net.Conn, error) {
	select {
	case <-x.stop:
		return nil, io.ErrClosedPipe
	default:
	}
	select {
	case <-x.stop:
		return nil, io.ErrClosedPipe
	case c := <-x.ch:
		return c, nil
	}
}

func (x *PipeListener) Close() error {
	x.once.Do(func() { close(x.stop) })
	return nil
}

func (x *PipeListener) Addr() net.Addr { return pipeAddr{} }

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }
