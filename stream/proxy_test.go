package stream

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/joeycumines/sesame/internal/testutil"
	"io"
	"math/rand"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProxy_bidirectionalHalfClose(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)

	rando := &lockedRand{rand: rand.New(rand.NewSource(12345))}

	// local ->
	a1, a2 := Pair(io.Pipe())(io.Pipe())
	// -> remote (and vice versa)
	b1, b2 := Pair(io.Pipe())(io.Pipe())

	halfCloser, err := NewHalfCloser(OptHalfCloser.Pipe(b1))
	if err != nil {
		t.Fatal(err)
	}

	out := make(chan error)
	go func() {
		err := Proxy(context.Background(), a2, halfCloser)
		_ = a2.Writer.CloseWithError(err)
		out <- err
	}()

	send := func(w io.Writer, line string) {
		t.Helper()
		line += "\n"
		var splits []int
		const (
			max     = 5
			ratio   = 0.6 // target split vs not per send
			divisor = ratio / max
		)
		for i := 0; i < 5; i++ {
			if split := rando.Intn(int(float64(len(line)) / divisor)); split < len(line) {
				splits = append(splits, split)
			}
		}
		sort.Ints(splits)
		for b := []byte(line); len(b) > 0; {
			var d []byte
			if len(splits) > 0 {
				offset := splits[0] - (len(line) - len(b))
				d, b, splits = b[:offset], b[offset:], splits[1:]
			} else {
				d, b = b, nil
			}
			if len(d) > 0 {
				if n, err := w.Write(d); err != nil || n != len(d) {
					t.Fatal(n, err)
				}
			}
		}
	}

	receive := func(r *bufio.Scanner, line string) {
		t.Helper()
		if !r.Scan() {
			t.Fatal(r.Err())
		}
		if v := r.Text(); v != line {
			t.Fatalf("unexpected line: %q\n%s", v, strings.TrimSpace(v))
		}
	}

	// variables referring to the endpoints of the pipes
	localScanner := bufio.NewScanner(a1)
	localPipe := a1
	remoteScanner := bufio.NewScanner(b2)
	remotePipe := b2

	wantSend := make([]byte, 1<<12)
	_, _ = rando.Read(wantSend)
	wantSend = bytes.ReplaceAll(wantSend, []byte{'\n'}, []byte{0})
	wantReceive := make([]byte, 1<<12)
	_, _ = rando.Read(wantReceive)
	wantReceive = bytes.ReplaceAll(wantReceive, []byte{'\n'}, []byte{0})

	lastReceive := make([]byte, 1<<12)
	lastReceive = bytes.ReplaceAll(lastReceive, []byte{'\n'}, []byte{0})

	done := make(chan struct{})
	go func() {
		defer close(done)

		// handle remote
		receive(remoteScanner, "hello remote")
		send(remotePipe, "hello local")
		{
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				receive(remoteScanner, string(wantSend))
			}()
			go func() {
				defer wg.Done()
				send(remotePipe, string(wantReceive))
			}()
			wg.Wait()
		}
		if remoteScanner.Scan() {
			err := fmt.Sprintf(`expected of got %q\n%s`, remoteScanner.Bytes(), remoteScanner.Bytes())
			t.Error(err)
			panic(err)
		}
		if err := remoteScanner.Err(); err != nil {
			t.Error(err)
			panic(err)
		}
		send(remotePipe, string(lastReceive))
		select {
		case err := <-out:
			t.Error(err)
		default:
		}
		if err := remotePipe.Close(); err != nil {
			t.Error(err)
		}
	}()

	// handle local
	send(localPipe, "hello remote")
	receive(localScanner, "hello local")
	{
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			send(localPipe, string(wantSend))
		}()
		go func() {
			defer wg.Done()
			receive(localScanner, string(wantReceive))
		}()
		wg.Wait()
	}
	select {
	case err := <-out:
		t.Fatal(err)
	case <-done:
		t.Fatal()
	default:
	}
	if err := localPipe.Writer.Close(); err != nil {
		t.Error(err)
	}
	receive(localScanner, string(lastReceive))
	if localScanner.Scan() {
		t.Fatalf(`expected of got %q\n%s`, localScanner.Bytes(), localScanner.Bytes())
	}
	if err := localScanner.Err(); err != nil {
		t.Error(err)
	}

	<-done

	if err := <-out; err != nil {
		t.Error(err)
	}
}

func TestProxy_bidirectionalReceiveEOF(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)

	rando := &lockedRand{rand: rand.New(rand.NewSource(523132))}

	// local ->
	a1, a2 := Pair(io.Pipe())(io.Pipe())
	// -> remote (and vice versa)
	b1, b2 := Pair(io.Pipe())(io.Pipe())

	out := make(chan error)
	go func() {
		err := Proxy(context.Background(), a2, b1)
		_ = a2.Writer.CloseWithError(err)
		out <- err
	}()

	send := func(w io.Writer, line string) {
		t.Helper()
		line += "\n"
		var splits []int
		const (
			max     = 5
			ratio   = 0.6 // target split vs not per send
			divisor = ratio / max
		)
		for i := 0; i < 5; i++ {
			if split := rando.Intn(int(float64(len(line)) / divisor)); split < len(line) {
				splits = append(splits, split)
			}
		}
		sort.Ints(splits)
		for b := []byte(line); len(b) > 0; {
			var d []byte
			if len(splits) > 0 {
				offset := splits[0] - (len(line) - len(b))
				d, b, splits = b[:offset], b[offset:], splits[1:]
			} else {
				d, b = b, nil
			}
			if len(d) > 0 {
				if n, err := w.Write(d); err != nil || n != len(d) {
					t.Fatal(n, err)
				}
			}
		}
	}

	receive := func(r *bufio.Scanner, line string) {
		t.Helper()
		if !r.Scan() {
			if line != `` {
				t.Errorf(`unexpected scan: %v`, r.Err())
			}
			return
		}
		if v := r.Text(); v != line {
			t.Errorf("unexpected line: %q\n%s", v, strings.TrimSpace(v))
		}
	}

	// variables referring to the endpoints of the pipes
	localScanner := bufio.NewScanner(a1)
	localPipe := a1
	remoteScanner := bufio.NewScanner(b2)
	remotePipe := b2

	wantSend := make([]byte, 1<<12)
	_, _ = rando.Read(wantSend)
	wantSend = bytes.ReplaceAll(wantSend, []byte{'\n'}, []byte{0})
	wantReceive := make([]byte, 1<<12)
	_, _ = rando.Read(wantReceive)
	wantReceive = bytes.ReplaceAll(wantReceive, []byte{'\n'}, []byte{0})

	done := make(chan struct{})
	go func() {
		defer close(done)

		// handle local
		receive(localScanner, "hello local")
		send(localPipe, "hello remote")
		{
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				send(localPipe, string(wantSend))
			}()
			go func() {
				defer wg.Done()
				receive(localScanner, string(wantReceive))
			}()
			wg.Wait()
		}
		if localScanner.Scan() {
			t.Errorf(`expected of got %q\n%s`, localScanner.Bytes(), localScanner.Bytes())
		}
		if err := localScanner.Err(); err != nil {
			t.Error(err)
		}
		// this one works because the local -> remote copy is still running, blocked on read
		if n, err := localPipe.Write([]byte("more data\n")); err != nil || n != 10 {
			t.Error(n, err)
		}
		// this one fails because it blocks, after the send copy fails
		if n, err := localPipe.Write([]byte("and again\n")); err != io.ErrClosedPipe || n != 0 {
			t.Error(n, err)
		}
	}()

	// handle remote
	send(remotePipe, "hello local")
	receive(remoteScanner, "hello remote")
	{
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			receive(remoteScanner, string(wantSend))
		}()
		go func() {
			defer wg.Done()
			send(remotePipe, string(wantReceive))
		}()
		wg.Wait()
	}
	select {
	case err := <-out:
		t.Error(err)
	default:
	}
	select {
	case <-done:
		t.Error()
	default:
	}
	if err := remotePipe.Writer.Close(); err != nil {
		t.Error(err)
	}

	if err := <-out; err != nil {
		t.Error(err)
	}

	time.Sleep(time.Millisecond * 100)

	select {
	case <-done:
		t.Error()
	default:
	}

	receive(remoteScanner, "")

	// unblock the copy
	if err := localPipe.Close(); err != nil {
		t.Error(err)
	}

	<-done
}
