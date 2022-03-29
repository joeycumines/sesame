package flowcontrol

import (
	"context"
	"errors"
	"fmt"
	"github.com/joeycumines/sesame/internal/testutil"
	"golang.org/x/exp/slices"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type (
	streamCounterMap struct {
		mu   sync.RWMutex
		data map[any]*streamCounters
	}

	streamCounters struct {
		Send int64
		Recv int64
	}

	mockInterface struct {
		streamCounterMap
		ctx              context.Context
		sendIn           chan any
		sendOut          chan error
		recvIn           chan struct{}
		recvOut          chan mockInterfaceRecvOut
		sendWindowUpdate func(key any, val uint32) (msg any)
		recvWindowUpdate func(msg any) (uint32, bool)
		sendSize         func(msg any) (uint32, bool)
		recvSize         func(msg any) (uint32, bool)
		sendKey          func(msg any) any
		recvKey          func(msg any) any
		sendMaxWindow    func() uint32
		recvMaxWindow    func() uint32
		sendMinConsume   func() uint32
		recvMinConsume   func() uint32
	}
	mockInterfaceRecvOut struct {
		msg any
		err error
	}
)

func (x *streamCounterMap) SendLoad(k any) int64 {
	x.mu.RLock()
	defer x.mu.RUnlock()
	if x.data[k] != nil {
		return x.data[k].Send
	}
	return 0
}
func (x *streamCounterMap) RecvLoad(k any) int64 {
	x.mu.RLock()
	defer x.mu.RUnlock()
	if x.data[k] != nil {
		return x.data[k].Recv
	}
	return 0
}
func (x *streamCounterMap) SendStore(k any, v int64) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.data == nil {
		x.data = make(map[any]*streamCounters)
	}
	if x.data[k] == nil {
		x.data[k] = new(streamCounters)
	}
	x.data[k].Send = v
}
func (x *streamCounterMap) RecvStore(k any, v int64) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.data == nil {
		x.data = make(map[any]*streamCounters)
	}
	if x.data[k] == nil {
		x.data[k] = new(streamCounters)
	}
	x.data[k].Recv = v
}
func (x *streamCounterMap) String() string {
	x.mu.RLock()
	defer x.mu.RUnlock()
	var b strings.Builder
	keys := make(map[any]string, len(x.data))
	kv := make([]any, 0, len(x.data))
	for k := range x.data {
		kv = append(kv, k)
		keys[k] = fmt.Sprint(k)
	}
	slices.SortFunc(kv, func(a, b any) bool { return keys[a] < keys[b] })
	for _, k := range kv {
		if b.Len() != 0 {
			b.WriteByte('\n')
		}
		b.WriteString(keys[k])
		b.WriteString(`: `)
		b.WriteString(`s=`)
		b.WriteString(strconv.FormatInt(x.data[k].Send, 10))
		b.WriteString(` `)
		b.WriteString(`r=`)
		b.WriteString(strconv.FormatInt(x.data[k].Recv, 10))
	}
	return b.String()
}
func (x *streamCounterMap) equal(data map[any]*streamCounters) bool {
	x.mu.RLock()
	defer x.mu.RUnlock()
	if len(x.data) != len(data) {
		return false
	}
	for k, v1 := range x.data {
		if v2, ok := data[k]; !ok ||
			v1.Send != v2.Send ||
			v1.Recv != v2.Recv {
			return false
		}
	}
	return true
}

func (x *mockInterface) Context() context.Context { return x.ctx }
func (x *mockInterface) Send(msg any) error {
	x.sendIn <- msg
	return <-x.sendOut
}
func (x *mockInterface) Recv() (msg any, err error) {
	x.recvIn <- struct{}{}
	v := <-x.recvOut
	return v.msg, v.err
}
func (x *mockInterface) SendWindowUpdate(key any, val uint32) (msg any) {
	return x.sendWindowUpdate(key, val)
}
func (x *mockInterface) RecvWindowUpdate(msg any) (uint32, bool) { return x.recvWindowUpdate(msg) }
func (x *mockInterface) SendSize(msg any) (uint32, bool)         { return x.sendSize(msg) }
func (x *mockInterface) RecvSize(msg any) (uint32, bool)         { return x.recvSize(msg) }
func (x *mockInterface) SendKey(msg any) any                     { return x.sendKey(msg) }
func (x *mockInterface) RecvKey(msg any) any                     { return x.recvKey(msg) }
func (x *mockInterface) SendMaxWindow() uint32                   { return x.sendMaxWindow() }
func (x *mockInterface) RecvMaxWindow() uint32                   { return x.recvMaxWindow() }
func (x *mockInterface) SendMinConsume() uint32                  { return x.sendMinConsume() }
func (x *mockInterface) RecvMinConsume() uint32                  { return x.recvMinConsume() }

func TestNewConn_direct(t *testing.T) {
	startGoroutine := runtime.NumGoroutine()
	defer testutil.CheckNumGoroutines(t, startGoroutine, false, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type (
		K int
		T int
		S struct {
			Key  K
			Type T
			Size uint32
		}
		R S
	)

	const (
		_ T = iota
		Yes
		No
		Que
	)

	var (
		sendMinConsume uint32 = 5
		recvMinConsume uint32 = 5
	)

	m := &mockInterface{
		ctx:              ctx,
		sendIn:           make(chan any),
		sendOut:          make(chan error),
		recvIn:           make(chan struct{}),
		recvOut:          make(chan mockInterfaceRecvOut),
		sendWindowUpdate: func(key any, val uint32) (msg any) { return S{key.(K), Que, val} },
		recvWindowUpdate: func(msg any) (uint32, bool) {
			v := msg.(R)
			if v.Type == Que {
				return v.Size, true
			}
			return 0, false
		},
		sendSize: func(msg any) (uint32, bool) {
			v := msg.(S)
			if v.Type == Yes {
				return v.Size, true
			}
			return 0, false
		},
		recvSize: func(msg any) (uint32, bool) {
			v := msg.(R)
			if v.Type == Yes {
				return v.Size, true
			}
			return 0, false
		},
		sendKey:        func(msg any) any { return msg.(S).Key },
		recvKey:        func(msg any) any { return msg.(R).Key },
		sendMaxWindow:  func() uint32 { return 100 },
		recvMaxWindow:  func() uint32 { return 40 },
		sendMinConsume: func() uint32 { return sendMinConsume },
		recvMinConsume: func() uint32 { return recvMinConsume },
	}

	equal := func(v map[K]*streamCounters) bool {
		c := make(map[any]*streamCounters, len(v))
		for k, v := range v {
			c[k] = v
		}
		return m.equal(c)
	}

	c := NewConn[any, any, any, Interface[any, any, any]](m)

	if v := c.Context(); v != ctx {
		t.Error(v)
	}

	var (
		sendDone  = make(chan bool, 1)
		startSend = func(msg S, fn func(err error) bool) {
			go func() {
				var ok bool
				defer func() { sendDone <- ok }()
				err := c.Send(msg)
				ok = fn(err)
			}()
		}
		recvDone  = make(chan bool, 1)
		startRecv = func(fn func(msg R, err error) bool) {
			go func() {
				var ok bool
				defer func() { recvDone <- ok }()
				msg, err := c.Recv()
				ok = fn(msg.(R), err)
			}()
		}
	)

	// initial send and receive, with send error, and no flow control
	startRecv(func(msg R, err error) bool {
		if msg != (R{1, No, 0}) || err != nil {
			t.Error(msg, err)
			return false
		}
		return true
	})
	e1 := errors.New(`e1`)
	startSend(S{0, No, 0}, func(err error) bool {
		if err != e1 {
			t.Error(err)
			return false
		}
		return true
	})
	time.Sleep(time.Millisecond * 5)
	<-m.recvIn
	if v := <-m.sendIn; v != (S{0, No, 0}) {
		t.Fatal(v)
	}
	m.sendOut <- e1
	if !(<-sendDone) {
		t.Fatal()
	}
	m.recvOut <- mockInterfaceRecvOut{R{1, No, 0}, nil}
	if !(<-recvDone) {
		t.Fatal()
	}

	// receive with error
	if !equal(nil) {
		t.Fatal(m)
	}
	startRecv(func(msg R, err error) bool {
		if msg != (R{}) || err != e1 {
			t.Error(msg, err)
			return false
		}
		return true
	})
	<-m.recvIn
	m.recvOut <- mockInterfaceRecvOut{R{}, e1}
	if !(<-recvDone) {
		t.Fatal()
	}
	if !equal(nil) {
		t.Fatal(m)
	}

	// recv success
	for _, z := range [...]struct {
		Size uint32
		Recv int64
	}{
		{1, 5},
		{5, 10},
		{6, 16},
		{0, 21},
		{18, 39},
	} {
		startRecv(func(msg R, err error) bool {
			if msg != (R{2, Yes, z.Size}) || err != nil {
				t.Error(msg, err)
				return false
			}
			return true
		})
		<-m.recvIn
		m.recvOut <- mockInterfaceRecvOut{R{2, Yes, z.Size}, nil}
		if !(<-recvDone) {
			t.Fatal()
		}
		if !equal(map[K]*streamCounters{
			2: {0, z.Recv},
		}) {
			t.Fatal(m)
		}
	}

	// recv window updates
	for _, z := range [...]struct {
		Size uint32
		Send int64
	}{
		{3, 3},
		{1, 4},
	} {
		startRecv(func(msg R, err error) bool {
			if msg != (R{2, Que, z.Size}) || err != nil {
				t.Error(msg, err)
				return false
			}
			return true
		})
		<-m.recvIn
		m.recvOut <- mockInterfaceRecvOut{R{2, Que, z.Size}, nil}
		if !(<-recvDone) {
			t.Fatal()
		}
		if !equal(map[K]*streamCounters{
			2: {z.Send, 39},
		}) {
			t.Fatal(m)
		}
	}

	// send with flow control
	for _, z := range [...]struct {
		Size uint32
		Send int64
	}{
		{3, -1}, // send is 4-5=-1
		{0, -6},
		{34, -40},
		{60, -100}, // we've hit but not exceeded the limit
	} {
		startSend(S{2, Yes, z.Size}, func(err error) bool {
			if err != nil {
				t.Error(err)
				return false
			}
			return true
		})
		if v := <-m.sendIn; v != (S{2, Yes, z.Size}) {
			t.Fatal(v)
		}
		m.sendOut <- nil
		if !(<-sendDone) {
			t.Fatal()
		}
		if !equal(map[K]*streamCounters{
			2: {z.Send, 39},
		}) {
			t.Fatal(m)
		}
	}

	// min consume allow 0...
	sendMinConsume = 0
	recvMinConsume = 0
	// ---
	startSend(S{2, Yes, 0}, func(err error) bool {
		if err != nil {
			t.Error(err)
			return false
		}
		return true
	})
	if v := <-m.sendIn; v != (S{2, Yes, 0}) {
		t.Fatal(v)
	}
	m.sendOut <- nil
	if !(<-sendDone) {
		t.Fatal()
	}
	if !equal(map[K]*streamCounters{
		2: {-100, 39},
	}) {
		t.Fatal(m)
	}
	// ---
	startRecv(func(msg R, err error) bool {
		if msg != (R{2, Yes, 0}) || err != nil {
			t.Error(msg, err)
			return false
		}
		return true
	})
	<-m.recvIn
	m.recvOut <- mockInterfaceRecvOut{R{2, Yes, 0}, nil}
	if !(<-recvDone) {
		t.Fatal()
	}
	if !equal(map[K]*streamCounters{
		2: {-100, 39},
	}) {
		t.Fatal(m)
	}
	// ---
	testutil.CheckNumGoroutines(t, startGoroutine, false, 0)

	// block send
	startSend(S{2, Yes, 1}, func(err error) bool {
		if err != nil {
			t.Error(err)
			return false
		}
		return true
	})
	time.Sleep(time.Millisecond * 200)
	select {
	case <-sendDone:
		t.Fatal()
	case <-m.sendIn:
		t.Fatal()
	default:
	}
	if !equal(map[K]*streamCounters{
		2: {-100, 39},
	}) {
		t.Fatal(m)
	}

	// receive one more byte, which will trigger sending a window update
	startRecv(func(msg R, err error) bool {
		if msg != (R{2, Yes, 1}) || err != nil {
			t.Error(msg, err)
			return false
		}
		return true
	})
	<-m.recvIn
	m.recvOut <- mockInterfaceRecvOut{R{2, Yes, 1}, nil}
	if !(<-recvDone) {
		t.Fatal()
	}
	if v := <-m.sendIn; v != (S{2, Que, 40}) {
		t.Fatal(v)
	}
	m.sendOut <- nil
	if !equal(map[K]*streamCounters{
		2: {-100, 0},
	}) {
		t.Fatal(m)
	}

	select {
	case <-sendDone:
		t.Fatal()
	case <-m.sendIn:
		t.Fatal()
	default:
	}

	// receive window update - 1 byte, just enough, triggers send
	startRecv(func(msg R, err error) bool {
		if msg != (R{2, Que, 1}) || err != nil {
			t.Error(msg, err)
			return false
		}
		return true
	})
	<-m.recvIn
	m.recvOut <- mockInterfaceRecvOut{R{2, Que, 1}, nil}
	if !(<-recvDone) {
		t.Fatal()
	}
	if v := <-m.sendIn; v != (S{2, Yes, 1}) {
		t.Fatal(v)
	}
	m.sendOut <- nil
	if !equal(map[K]*streamCounters{
		2: {-100, 0},
	}) {
		t.Fatal(m)
	}

	// TODO test unblock but not enough

	// TODO test over unblock

	// TODO test send block context cancel
}
