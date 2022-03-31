// Package flowcontrol implements message-based flow control.
package flowcontrol

import (
	"context"
	"sync"
)

type (
	// Conn implements message based flow control.
	//
	// Usage notes:
	//
	//   - When sending flow-controlled messages, callers MUST synchronise calls to Send, on a per-stream basis
	//   - Send and Recv on the Interface are NOT synchronised by this implementation
	//   - Send MUST be safe to call concurrently
	Conn[K comparable, S any, R any] struct {
		i       Interface[K, S, R]
		waiters map[K]chan struct{}
		mu      sync.Mutex
	}

	Interface[K any, S any, R any] interface {
		Context() context.Context
		Send(msg S) error
		Recv() (msg R, err error)

		StreamContext(key K) context.Context

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

		SendConfig(key K) Config
		RecvConfig(key K) Config
	}

	Config struct {
		MaxWindow  uint32
		MinConsume uint32
	}
)

var (
	// StreamNotFound is a sentinel value for use with Interface.StreamContext.
	StreamNotFound = func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}()
)

func NewConn[K comparable, S any, R any](i Interface[K, S, R]) *Conn[K, S, R] {
	return &Conn[K, S, R]{
		i:       i,
		waiters: make(map[K]chan struct{}),
	}
}

func (x *Conn[K, S, R]) Context() context.Context { return x.i.Context() }

func (x *Conn[K, S, R]) Send(msg S) error {
	ctx := x.i.Context()
	if err := ctx.Err(); err != nil {
		return err
	}

	key := x.i.SendKey(msg)

	ctxStream := x.i.StreamContext(key)
	if err := ctxStream.Err(); err != nil {
		return err
	}

	size, ok := x.i.SendSize(msg)
	if !ok {
		// not flow controlled
		return x.i.Send(msg)
	}

	var (
		addedWaiter   bool
		deletedWaiter bool
	)
	defer func() {
		if !addedWaiter || deletedWaiter {
			return
		}
		// on context cancel or (potentially) panic
		x.mu.Lock()
		defer x.mu.Unlock()
		x.deleteWaiter(key)
	}()
	for {
		// config may change
		config := x.i.SendConfig(key)

		size := size
		if size < config.MinConsume {
			size = config.MinConsume
		}

		ch := func() chan struct{} {
			x.mu.Lock()
			defer x.mu.Unlock()

			val := x.i.SendLoad(key) - int64(size)

			// because this is message based, and because we enforce a min consume per message,
			// we allow if EITHER there's space OR if there's ANY space BEFORE calculating the size
			// (the buffering is expected to be per-message, e.g. channels)
			if adj := val + int64(config.MaxWindow); adj >= 0 || adj+int64(size) > 0 {
				x.deleteWaiter(key)
				deletedWaiter = true

				x.i.SendStore(key, val)

				return nil
			}

			if x.waiters[key] == nil {
				x.waiters[key] = make(chan struct{}, 1)
				addedWaiter = true
			}

			return x.waiters[key]
		}()
		if ch == nil {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ctxStream.Done():
			return ctxStream.Err()

		case <-ch:
			// recheck...
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if err := ctxStream.Err(); err != nil {
			return err
		}
	}

	return x.i.Send(msg)
}

func (x *Conn[K, S, R]) Recv() (msg R, err error) {
	if err = x.i.Context().Err(); err != nil {
		return
	}

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
		case x.waiters[key] <- struct{}{}:
		default:
		}
		return
	}

	size, ok := x.i.RecvSize(msg)
	if !ok {
		// not flow controlled
		return
	}

	key := x.i.RecvKey(msg)

	config := x.i.RecvConfig(key)

	if size < config.MinConsume {
		size = config.MinConsume
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	val := x.i.RecvLoad(key) + int64(size)

	if val >= int64(config.MaxWindow) && val > 0 && x.i.StreamContext(key).Err() == nil {
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
			// don't use x.Send due to the risk of it being erroneously detected as a flow controlled message
			// (e.g. due to programmer error / misuse)
			// if that happens, it may break the assumption of no concurrent (stream) sends
			go x.i.Send(x.i.SendWindowUpdate(key, update))
		}
	}

	x.i.RecvStore(key, val)

	return
}

func (x *Conn[K, S, R]) deleteWaiter(key K) {
	if waiter, ok := x.waiters[key]; ok {
		delete(x.waiters, key)
		if waiter != nil {
			close(waiter)
		}
	}
}
