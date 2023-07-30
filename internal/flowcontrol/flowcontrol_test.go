package flowcontrol

import (
	"context"
	"errors"
	"fmt"
	"github.com/joeycumines/sesame/internal/testutil"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type (
	mockKey int

	streamCounterMap struct {
		mu   sync.RWMutex
		data map[mockKey]*streamCounters
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
		sendWindowUpdate func(key mockKey, val uint32) (msg any)
		recvWindowUpdate func(msg any) (uint32, bool)
		sendSize         func(msg any) (uint32, bool)
		recvSize         func(msg any) (uint32, bool)
		sendKey          func(msg any) mockKey
		recvKey          func(msg any) mockKey
		sendConfig       func(key mockKey) Config
		recvConfig       func(key mockKey) Config
		streamContext    func(key mockKey) context.Context
	}
	mockInterfaceRecvOut struct {
		msg any
		err error
	}
)

func (x *streamCounterMap) SendLoad(k mockKey) int64 {
	x.mu.RLock()
	defer x.mu.RUnlock()
	if x.data[k] != nil {
		return x.data[k].Send
	}
	return 0
}
func (x *streamCounterMap) RecvLoad(k mockKey) int64 {
	x.mu.RLock()
	defer x.mu.RUnlock()
	if x.data[k] != nil {
		return x.data[k].Recv
	}
	return 0
}
func (x *streamCounterMap) SendStore(k mockKey, v int64) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.data == nil {
		x.data = make(map[mockKey]*streamCounters)
	}
	if x.data[k] == nil {
		x.data[k] = new(streamCounters)
	}
	x.data[k].Send = v
}
func (x *streamCounterMap) RecvStore(k mockKey, v int64) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.data == nil {
		x.data = make(map[mockKey]*streamCounters)
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
	keys := make(map[mockKey]string, len(x.data))
	kv := make([]mockKey, 0, len(x.data))
	for k := range x.data {
		kv = append(kv, k)
		keys[k] = fmt.Sprint(k)
	}
	slices.SortFunc(kv, func(a, b mockKey) int {
		if keys[a] < keys[b] {
			return -1
		} else if keys[a] > keys[b] {
			return 1
		} else {
			return 0
		}
	})
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
func (x *streamCounterMap) equal(data map[mockKey]*streamCounters) bool {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return maps.EqualFunc[map[mockKey]*streamCounters, map[mockKey]*streamCounters](x.data, data, func(v1 *streamCounters, v2 *streamCounters) bool {
		return v1.Send == v2.Send && v1.Recv == v2.Recv
	})
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
func (x *mockInterface) SendWindowUpdate(key mockKey, val uint32) (msg any) {
	return x.sendWindowUpdate(key, val)
}
func (x *mockInterface) RecvWindowUpdate(msg any) (uint32, bool)   { return x.recvWindowUpdate(msg) }
func (x *mockInterface) SendSize(msg any) (uint32, bool)           { return x.sendSize(msg) }
func (x *mockInterface) RecvSize(msg any) (uint32, bool)           { return x.recvSize(msg) }
func (x *mockInterface) SendKey(msg any) mockKey                   { return x.sendKey(msg) }
func (x *mockInterface) RecvKey(msg any) mockKey                   { return x.recvKey(msg) }
func (x *mockInterface) SendConfig(key mockKey) Config             { return x.sendConfig(key) }
func (x *mockInterface) RecvConfig(key mockKey) Config             { return x.recvConfig(key) }
func (x *mockInterface) StreamContext(key mockKey) context.Context { return x.streamContext(key) }

func TestNewConn_direct(t *testing.T) {
	startGoroutine := runtime.NumGoroutine()
	defer testutil.CheckNumGoroutines(t, startGoroutine, false, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type (
		T int
		S struct {
			Key  mockKey
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
		sendWindowUpdate: func(key mockKey, val uint32) (msg any) { return S{key, Que, val} },
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
		sendKey: func(msg any) mockKey { return msg.(S).Key },
		recvKey: func(msg any) mockKey { return msg.(R).Key },
		sendConfig: func(key mockKey) Config {
			return Config{
				MaxWindow:  100,
				MinConsume: sendMinConsume,
			}
		},
		recvConfig: func(key mockKey) Config {
			return Config{
				MaxWindow:  40,
				MinConsume: recvMinConsume,
			}
		},
		streamContext: func(key mockKey) context.Context { return context.Background() },
	}

	var expected map[mockKey]*streamCounters
	asExpected := func() bool { return m.equal(expected) }

	c := NewConn[mockKey, any, any](m)

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
				if err == nil {
					c.UpdateWindow(msg)
				}
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
	expected = nil
	if !asExpected() {
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
	expected = nil
	if !asExpected() {
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
		expected = map[mockKey]*streamCounters{
			2: {0, z.Recv},
		}
		if !asExpected() {
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
		expected = map[mockKey]*streamCounters{
			2: {z.Send, 39},
		}
		if !asExpected() {
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
		expected = map[mockKey]*streamCounters{
			2: {z.Send, 39},
		}
		if !asExpected() {
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
	expected = map[mockKey]*streamCounters{
		2: {-100, 39},
	}
	if !asExpected() {
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
	expected = map[mockKey]*streamCounters{
		2: {-100, 39},
	}
	if !asExpected() {
		t.Fatal(m)
	}
	// ---
	testutil.CheckNumGoroutines(t, startGoroutine, false, 0)

	// we'll start modifying expected in place now... it's a pain to copy and paste

	// send to another stream
	startSend(S{3, Yes, 100}, func(err error) bool {
		if err != nil {
			t.Error(err)
			return false
		}
		return true
	})
	if v := <-m.sendIn; v != (S{3, Yes, 100}) {
		t.Fatal(v)
	}
	m.sendOut <- nil
	if !(<-sendDone) {
		t.Fatal()
	}
	expected[3] = &streamCounters{-100, 0}
	if !asExpected() {
		t.Fatal(m)
	}

	// receive from another stream
	startRecv(func(msg R, err error) bool {
		if msg != (R{4, Yes, 39}) || err != nil {
			t.Error(msg, err)
			return false
		}
		return true
	})
	<-m.recvIn
	m.recvOut <- mockInterfaceRecvOut{R{4, Yes, 39}, nil}
	if !(<-recvDone) {
		t.Fatal()
	}
	expected[4] = &streamCounters{0, 39}
	if !asExpected() {
		t.Fatal(m)
	}

	// receive window update from another stream
	startRecv(func(msg R, err error) bool {
		if msg != (R{5, Que, 233}) || err != nil {
			t.Error(msg, err)
			return false
		}
		return true
	})
	<-m.recvIn
	m.recvOut <- mockInterfaceRecvOut{R{5, Que, 233}, nil}
	if !(<-recvDone) {
		t.Fatal()
	}
	expected[5] = &streamCounters{233, 0}
	if !asExpected() {
		t.Fatal(m)
	}

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
	// note: 2 is still {-100, 39}
	if !asExpected() {
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
	expected[2] = &streamCounters{-100, 0}
	if !asExpected() {
		t.Fatal(m)
	}

	select {
	case <-sendDone:
		t.Fatal()
	case <-m.sendIn:
		t.Fatal()
	default:
	}

	// receive window update - 1 byte, "just enough" to unblock (note: different in this implementation, see below)
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
	if !(<-sendDone) {
		t.Fatal()
	}
	// note: 2 is still {-100, 0} (it increased then decreased by the same)
	if !asExpected() {
		t.Fatal(m)
	}
	testutil.CheckNumGoroutines(t, startGoroutine, false, 0)
	if !asExpected() {
		t.Fatal(m)
	}

	// block again - this time for a larger send
	// (this is where it diverges from the bytes based buffering)
	startSend(S{2, Yes, 399}, func(err error) bool {
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
	// note: 2 is still {-100, 0}
	if !asExpected() {
		t.Fatal(m)
	}

	// annd unblock, with only one byte, leaving it very negative
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
	if v := <-m.sendIn; v != (S{2, Yes, 399}) {
		t.Fatal(v)
	}
	m.sendOut <- nil
	if !(<-sendDone) {
		t.Fatal()
	}
	expected[2] = &streamCounters{-100 + 1 - 399, 0}
	if !asExpected() {
		t.Fatal(m)
	}
	testutil.CheckNumGoroutines(t, startGoroutine, false, 0)
	if !asExpected() {
		t.Fatal(m)
	}

	// recv window update several times
	for _, z := range [...]struct {
		Size uint32
		Send int64
	}{
		{100, -398},
		{100, -298},
		{98, -200},
		{99, -101},
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
		expected[2].Send = z.Send
		if !asExpected() {
			t.Fatal(m)
		}
	}

	// block on send of 0
	startSend(S{2, Yes, 0}, func(err error) bool {
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
	// note: 2 is still {-101, 0}
	if !asExpected() {
		t.Fatal(m)
	}

	// well over unblock, leaving it very positive
	startRecv(func(msg R, err error) bool {
		if msg != (R{2, Que, 9999}) || err != nil {
			t.Error(msg, err)
			return false
		}
		return true
	})
	<-m.recvIn
	m.recvOut <- mockInterfaceRecvOut{R{2, Que, 9999}, nil}
	if !(<-recvDone) {
		t.Fatal()
	}
	if v := <-m.sendIn; v != (S{2, Yes, 0}) {
		t.Fatal(v)
	}
	m.sendOut <- nil
	if !(<-sendDone) {
		t.Fatal()
	}
	expected[2] = &streamCounters{-101 + 9999, 0}
	if !asExpected() {
		t.Fatal(m)
	}
	testutil.CheckNumGoroutines(t, startGoroutine, false, 0)
	if !asExpected() {
		t.Fatal(m)
	}

	// block send then context cancel
	expected[3] = &streamCounters{-100, 0}
	if !asExpected() {
		t.Fatal(m)
	}
	select {
	case <-sendDone:
		t.Fatal()
	case <-m.sendIn:
		t.Fatal()
	default:
	}
	startSend(S{3, Yes, 1}, func(err error) bool {
		if err != context.Canceled {
			t.Error(`unexpected error:`, err)
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
	if !asExpected() {
		t.Fatal(m)
	}
	cancel()
	if !(<-sendDone) {
		t.Fatal()
	}
}

func TestStreamNotFound(t *testing.T) {
	ctx := StreamNotFound
	if ctx == context.Background() {
		t.Error(ctx)
	}
	if ctx != StreamNotFound {
		t.Error(ctx)
	}
	if err := ctx.Err(); err != context.Canceled {
		t.Error(err)
	}
}
