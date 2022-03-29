// Package flowcontrol implements message-based flow control.
package flowcontrol

import (
	"context"
	"sync"
)

type (
	Conn[K any, S any, R any, I Interface[K, S, R]] struct {
		i  Interface[K, S, R]
		ch chan struct{}
		mu sync.Mutex
		rw sync.RWMutex
	}

	Interface[K any, S any, R any] interface {
		Context() context.Context
		Send(msg S) error
		Recv() (msg R, err error)

		SendWindowUpdate(key K, val uint32) (msg S)
		RecvWindowUpdate(msg R) (uint32, bool)

		SendSize(msg S) (uint32, bool)
		RecvSize(msg R) (uint32, bool)

		SendKey(msg S) K
		RecvKey(msg R) K

		SendLoad(key K) int64
		RecvLoad(key K) int64

		SendStore(key K, val int64)
		RecvStore(key K, val int64)

		SendMaxWindow() uint32
		RecvMaxWindow() uint32

		SendMinConsume() uint32
		RecvMinConsume() uint32
	}
)

func NewConn[K any, S any, R any, I Interface[K, S, R]](i Interface[K, S, R]) *Conn[K, S, R, I] {
	return &Conn[K, S, R, I]{
		i:  i,
		ch: make(chan struct{}, 1),
	}
}

func (x *Conn[K, S, R, I]) Context() context.Context { return x.i.Context() }

func (x *Conn[K, S, R, I]) Send(msg S) error {
	size, ok := x.i.SendSize(msg)
	if !ok {
		// not flow controlled
		return x.send(&x.rw, msg)
	}
	if min := x.i.SendMinConsume(); size < min {
		size = min
	}
	var (
		key = x.i.SendKey(msg)
		val int64
		ctx context.Context
	)
	for {
		func() {
			x.mu.Lock()
			defer x.mu.Unlock()
			val = x.i.SendLoad(key)
			val -= int64(size)
			ok = val+int64(x.i.SendMaxWindow()) >= 0
			if ok {
				x.i.SendStore(key, val)
			} else {
				// we're going to need to block
				// drain the channel while holding the mutex
				select {
				case <-x.ch:
				default:
				}
			}
		}()
		if ok {
			break
		}
		if ctx == nil {
			ctx = x.i.Context()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-x.ch:
			// recheck...
		}
	}
	// this send uses the read lock to enforce lower priority
	// (callers should not use this concurrently, in the first place)
	return x.send(x.rw.RLocker(), msg)
}

func (x *Conn[K, S, R, I]) Recv() (msg R, err error) {
	msg, err = x.i.Recv()
	if err != nil {
		return
	}
	if update, ok := x.i.RecvWindowUpdate(msg); ok {
		key := x.i.RecvKey(msg)
		x.mu.Lock()
		defer x.mu.Unlock()
		val := x.i.SendLoad(key)
		// TODO handle overflow/misuse
		val += int64(update)
		x.i.SendStore(key, val)
		select {
		case x.ch <- struct{}{}:
		default:
		}
		return
	}
	size, ok := x.i.RecvSize(msg)
	if !ok {
		// not flow controlled
		return
	}
	if min := x.i.RecvMinConsume(); size < min {
		size = min
	}
	key := x.i.RecvKey(msg)
	x.mu.Lock()
	defer x.mu.Unlock()
	val := x.i.RecvLoad(key)
	val += int64(size)
	if val >= int64(x.i.RecvMaxWindow()) {
		// inform the other end how much we've received
		for val > 0 {
			const max = 1<<32 - 1
			var update uint32
			if val > max {
				update = max
			} else {
				update = uint32(val)
			}
			val -= int64(update)
			// TODO this would be much better off in a worker
			go x.send(&x.rw, x.i.SendWindowUpdate(key, update))
		}
	}
	x.i.RecvStore(key, val)
	return
}

func (x *Conn[K, S, R, I]) send(l sync.Locker, m S) error {
	l.Lock()
	defer l.Unlock()
	return x.i.Send(m)
}
