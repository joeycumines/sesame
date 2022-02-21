// Copyright (c) 2009 The Go Authors. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//    * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//    * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

// See also pipe.go.

package ionet

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/joeycumines/sesame/stream"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type (
	ioAttempt struct {
		b   []byte
		n   int
		err error
	}

	PipeFactory func() (stream.PipeReader, stream.PipeWriter)
)

var (
	pipeFactories = map[string]PipeFactory{
		`io.Pipe`: func() (stream.PipeReader, stream.PipeWriter) { return io.Pipe() },
		`ConnPipe.SendPipe`: func() (stream.PipeReader, stream.PipeWriter) {
			c, _ := Pipe()
			r, w := c.ReceivePipe()
			_ = r.Close()
			_ = w.Close()
			return c.SendPipe()
		},
		`ConnPipe.ReceivePipe`: func() (stream.PipeReader, stream.PipeWriter) {
			c, _ := Pipe()
			r, w := c.SendPipe()
			_ = r.Close()
			_ = w.Close()
			return c.ReceivePipe()
		},
	}
)

func testPipeIO(t *testing.T, r io.Reader, w io.Writer, wAttempts []ioAttempt, rAttempts []ioAttempt) {
	t.Helper()
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []string
	)
	wg.Add(2)
	go func() {
		t.Helper()
		defer wg.Done()
		for i, a := range wAttempts {
			n, err := w.Write(a.b)
			if (err == nil) != (a.err == nil) || (err != nil && err.Error() != a.err.Error()) || n != a.n {
				mu.Lock()
				errs = append(errs, fmt.Sprintf(`testPipeIO: unexpected write: %d %d %v`, i, n, err))
				mu.Unlock()
			}
		}
	}()
	go func() {
		t.Helper()
		defer wg.Done()
		for i, a := range rAttempts {
			b := make([]byte, len(a.b))
			n, err := r.Read(b)
			if (err == nil) != (a.err == nil) || (err != nil && err.Error() != a.err.Error()) || a.n != n || !bytes.Equal(b, a.b) {
				mu.Lock()
				errs = append(errs, fmt.Sprintf(`testPipeIO: unexpected read: %d %d %v %q`, i, n, err, b))
				mu.Unlock()
			}
		}
	}()
	done := make(chan struct{})
	go func() {
		timer := time.NewTimer(time.Second * 5)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			mu.Lock()
			for _, v := range errs {
				t.Error(v)
			}
			mu.Unlock()
			panic(`testPipeIO: timed out`)
		}
	}()
	wg.Wait()
	close(done)
	for _, v := range errs {
		t.Error(v)
	}
}

func testPipeIOBasic(t *testing.T, r io.Reader, w io.Writer) {
	t.Helper()
	testPipeIO(
		t,
		r,
		w,
		[]ioAttempt{
			{[]byte(`pasdhjUASDHsg`), 13, nil},
		},
		[]ioAttempt{
			{[]byte(`p`), 1, nil},
			{[]byte(`a`), 1, nil},
			{[]byte(`sdh`), 3, nil},
			{append([]byte(`jUASDHsg`), 0, 0), 8, nil},
		},
	)
}

func TestPipe_pipeIOBasic(t *testing.T) {
	c1, c2 := Pipe()
	testPipeIOBasic(t, c1, c2)
	testPipeIOBasic(t, c2, c1)
}

func TestConnPipe_SendPipe_pipeIOBasic(t *testing.T) {
	c1, c2 := Pipe()
	for _, c := range [...]*ConnPipe{c1, c2} {
		r, w := c.SendPipe()
		testPipeIOBasic(t, r, w)
		testPipeIOBasic(t, r, c)
	}
}

func TestConnPipe_ReceivePipe_pipeIOBasic(t *testing.T) {
	c1, c2 := Pipe()
	for _, c := range [...]*ConnPipe{c1, c2} {
		r, w := c.ReceivePipe()
		testPipeIOBasic(t, r, w)
		testPipeIOBasic(t, c, w)
	}
}

func TestConnPipe_SendPipe(t *testing.T) {
	c1, c2 := Pipe()
	r, w := c2.SendPipe()
	if er, ew := c1.ReceivePipe(); r != er || w != ew {
		t.Error(r, w, er, ew)
	}
	if r == nil || w == nil {
		t.Fatal(r, w)
	}
	if r.p != c1 {
		t.Error()
	}
	if w.p != c2 {
		t.Error()
	}
}

func TestConnPipe_ReceivePipe(t *testing.T) {
	c1, c2 := Pipe()
	r, w := c2.ReceivePipe()
	if er, ew := c1.SendPipe(); r != er || w != ew {
		t.Error(r, w, er, ew)
	}
	if r == nil || w == nil {
		t.Fatal(r, w)
	}
	if r.p != c2 {
		t.Error()
	}
	if w.p != c1 {
		t.Error()
	}
}

func checkWrite(t *testing.T, w io.Writer, data []byte, c chan int) {
	n, err := w.Write(data)
	if err != nil {
		t.Errorf("write: %v", err)
	}
	if n != len(data) {
		t.Errorf("short write: %d != %d", n, len(data))
	}
	c <- 0
}

func testPipeFactories(t *testing.T, fn func(t *testing.T, factory PipeFactory)) {
	for k, v := range pipeFactories {
		v := v
		t.Run(k, func(t *testing.T) { fn(t, v) })
	}
}

// Test a single read/write pair.
func testPipe1(t *testing.T, factory PipeFactory) {
	c := make(chan int)
	r, w := factory()
	var buf = make([]byte, 64)
	go checkWrite(t, w, []byte("hello, world"), c)
	n, err := r.Read(buf)
	if err != nil {
		t.Errorf("read: %v", err)
	} else if n != 12 || string(buf[0:12]) != "hello, world" {
		t.Errorf("bad read: got %q", buf[0:n])
	}
	<-c
	if err := r.Close(); err != nil {
		t.Error(err)
	}
	if err := w.Close(); err != nil {
		t.Error(err)
	}
}

func TestPipe1(t *testing.T) { testPipeFactories(t, testPipe1) }

func reader(t *testing.T, r io.Reader, c chan int) {
	var buf = make([]byte, 64)
	for {
		n, err := r.Read(buf)
		if err == io.EOF {
			c <- 0
			break
		}
		if err != nil {
			t.Errorf("read: %v", err)
		}
		c <- n
	}
}

// Test a sequence of read/write pairs.
func testPipe2(t *testing.T, factory PipeFactory) {
	c := make(chan int)
	r, w := factory()
	go reader(t, r, c)
	var buf = make([]byte, 64)
	for i := 0; i < 5; i++ {
		p := buf[0 : 5+i*10]
		n, err := w.Write(p)
		if n != len(p) {
			t.Errorf("wrote %d, got %d", len(p), n)
		}
		if err != nil {
			t.Errorf("write: %v", err)
		}
		nn := <-c
		if nn != n {
			t.Errorf("wrote %d, read got %d", n, nn)
		}
	}
	_ = w.Close()
	nn := <-c
	if nn != 0 {
		t.Errorf("final read got %d", nn)
	}
}

func TestPipe2(t *testing.T) { testPipeFactories(t, testPipe2) }

type pipeReturn struct {
	n   int
	err error
}

// Test a large write that requires multiple reads to satisfy.
func writer(w io.WriteCloser, buf []byte, c chan pipeReturn) {
	n, err := w.Write(buf)
	w.Close()
	c <- pipeReturn{n, err}
}

func testPipe3(t *testing.T, factory PipeFactory) {
	c := make(chan pipeReturn)
	r, w := factory()
	var wdat = make([]byte, 128)
	for i := 0; i < len(wdat); i++ {
		wdat[i] = byte(i)
	}
	go writer(w, wdat, c)
	var rdat = make([]byte, 1024)
	tot := 0
	for n := 1; n <= 256; n *= 2 {
		nn, err := r.Read(rdat[tot : tot+n])
		if err != nil && err != io.EOF {
			t.Fatalf("read: %v", err)
		}

		// only final two reads should be short - 1 byte, then 0
		expect := n
		if n == 128 {
			expect = 1
		} else if n == 256 {
			expect = 0
			if err != io.EOF {
				t.Fatalf("read at end: %v", err)
			}
		}
		if nn != expect {
			t.Fatalf("read %d, expected %d, got %d", n, expect, nn)
		}
		tot += nn
	}
	pr := <-c
	if pr.n != 128 || pr.err != nil {
		t.Fatalf("write 128: %d, %v", pr.n, pr.err)
	}
	if tot != 128 {
		t.Fatalf("total read %d != 128", tot)
	}
	for i := 0; i < 128; i++ {
		if rdat[i] != byte(i) {
			t.Fatalf("rdat[%d] = %d", i, rdat[i])
		}
	}
}

// Test read after/before writer close.
func TestPipe3(t *testing.T) { testPipeFactories(t, testPipe3) }

type closer interface {
	CloseWithError(error) error
	Close() error
}

type pipeTest struct {
	async          bool
	err            error
	closeWithError bool
}

func (p pipeTest) String() string {
	return fmt.Sprintf("async=%v err=%v closeWithError=%v", p.async, p.err, p.closeWithError)
}

var pipeTests = []pipeTest{
	{true, nil, false},
	{true, nil, true},
	{true, io.ErrShortWrite, true},
	{false, nil, false},
	{false, nil, true},
	{false, io.ErrShortWrite, true},
}

func delayClose(t *testing.T, cl closer, ch chan int, tt pipeTest) {
	time.Sleep(1 * time.Millisecond)
	var err error
	if tt.closeWithError {
		err = cl.CloseWithError(tt.err)
	} else {
		err = cl.Close()
	}
	if err != nil {
		t.Errorf("delayClose: %v", err)
	}
	ch <- 0
}

func testPipeReadClose(t *testing.T, factory PipeFactory) {
	for _, tt := range pipeTests {
		c := make(chan int, 1)
		r, w := factory()
		if tt.async {
			go delayClose(t, w, c, tt)
		} else {
			delayClose(t, w, c, tt)
		}
		var buf = make([]byte, 64)
		n, err := r.Read(buf)
		<-c
		want := tt.err
		if want == nil {
			want = io.EOF
		}
		if !errors.Is(err, want) {
			t.Errorf("read from closed pipe: %q want %q", fmt.Sprint(err), fmt.Sprint(want))
		}
		if n != 0 {
			t.Errorf("read on closed pipe returned %d", n)
		}
		if err = r.Close(); err != nil {
			t.Errorf("r.Close: %v", err)
		}
	}
}

func TestPipeReadClose(t *testing.T) { testPipeFactories(t, testPipeReadClose) }

// Test close on Read side during Read.
func testPipeReadClose2(t *testing.T, factory PipeFactory) {
	c := make(chan int, 1)
	r, _ := factory()
	go delayClose(t, r, c, pipeTest{})
	n, err := r.Read(make([]byte, 64))
	<-c
	if n != 0 || err != io.ErrClosedPipe {
		t.Errorf("read from closed pipe: %v, %v want %v, %v", n, err, 0, io.ErrClosedPipe)
	}
}

func TestPipeReadClose2(t *testing.T) { testPipeFactories(t, testPipeReadClose2) }

// Test write after/before reader close.
func testPipeWriteClose(t *testing.T, factory PipeFactory) {
	for _, tt := range pipeTests {
		c := make(chan int, 1)
		r, w := factory()
		if tt.async {
			go delayClose(t, r, c, tt)
		} else {
			delayClose(t, r, c, tt)
		}
		n, err := io.WriteString(w, "hello, world")
		<-c
		expect := tt.err
		if expect == nil {
			expect = io.ErrClosedPipe
		}
		if !errors.Is(err, expect) {
			t.Errorf("write on closed pipe: %q want %q", fmt.Sprint(err), fmt.Sprint(expect))
		}
		if n != 0 {
			t.Errorf("write on closed pipe returned %d", n)
		}
		if err = w.Close(); err != nil {
			t.Errorf("w.Close: %v", err)
		}
	}
}

func TestPipeWriteClose(t *testing.T) { testPipeFactories(t, testPipeWriteClose) }

// Test close on Write side during Write.
func testPipeWriteClose2(t *testing.T, factory PipeFactory) {
	c := make(chan int, 1)
	_, w := factory()
	go delayClose(t, w, c, pipeTest{})
	n, err := w.Write(make([]byte, 64))
	<-c
	if n != 0 || err != io.ErrClosedPipe {
		t.Errorf("write to closed pipe: %v, %v want %v, %v", n, err, 0, io.ErrClosedPipe)
	}
}

func TestPipeWriteClose2(t *testing.T) { testPipeFactories(t, testPipeWriteClose2) }

func testPipeWriteEmpty(t *testing.T, factory PipeFactory) {
	r, w := factory()
	go func() {
		w.Write([]byte{})
		w.Close()
	}()
	var b [2]byte
	io.ReadFull(r, b[0:2])
	r.Close()
}

func TestPipeWriteEmpty(t *testing.T) { testPipeFactories(t, testPipeWriteEmpty) }

func testPipeWriteNil(t *testing.T, factory PipeFactory) {
	r, w := factory()
	go func() {
		w.Write(nil)
		w.Close()
	}()
	var b [2]byte
	io.ReadFull(r, b[0:2])
	r.Close()
}

func TestPipeWriteNil(t *testing.T) { testPipeFactories(t, testPipeWriteNil) }

func testPipeWriteAfterWriterClose(t *testing.T, factory PipeFactory) {
	r, w := factory()

	done := make(chan bool)
	var writeErr error
	go func() {
		_, err := w.Write([]byte("hello"))
		if err != nil {
			t.Errorf("got error: %q; expected none", err)
		}
		w.Close()
		_, writeErr = w.Write([]byte("world"))
		done <- true
	}()

	buf := make([]byte, 100)
	var result string
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		t.Fatalf("got: %q; want: %q", err, io.ErrUnexpectedEOF)
	}
	result = string(buf[0:n])
	<-done

	if result != "hello" {
		t.Errorf("got: %q; want: %q", result, "hello")
	}
	if writeErr != io.ErrClosedPipe {
		t.Errorf("got: %q; want: %q", writeErr, io.ErrClosedPipe)
	}
}

func TestWriteAfterWriterClose(t *testing.T) { testPipeFactories(t, testPipeWriteAfterWriterClose) }

func testPipeCloseError(t *testing.T, factory PipeFactory) {
	type testError1 struct{ error }
	type testError2 struct{ error }

	r, w := factory()
	r.CloseWithError(testError1{})
	if _, err := w.Write(nil); !errors.Is(err, testError1{}) {
		t.Errorf("Write error: got %T, want testError1", err)
	}
	r.CloseWithError(testError2{})
	if _, err := w.Write(nil); !errors.Is(err, testError1{}) {
		t.Errorf("Write error: got %T, want testError1", err)
	}

	r, w = factory()
	w.CloseWithError(testError1{})
	if _, err := r.Read(nil); !errors.Is(err, testError1{}) {
		t.Errorf("Read error: got %T, want testError1", err)
	}
	w.CloseWithError(testError2{})
	if _, err := r.Read(nil); !errors.Is(err, testError1{}) {
		t.Errorf("Read error: got %T, want testError1", err)
	}
}

func TestPipeCloseError(t *testing.T) { testPipeFactories(t, testPipeCloseError) }

func testPipeConcurrent(t *testing.T, factory PipeFactory) {
	const (
		input    = "0123456789abcdef"
		count    = 8
		readSize = 2
	)

	t.Run("Write", func(t *testing.T) {
		r, w := factory()

		for i := 0; i < count; i++ {
			go func() {
				time.Sleep(time.Millisecond) // Increase probability of race
				if n, err := w.Write([]byte(input)); n != len(input) || err != nil {
					t.Errorf("Write() = (%d, %v); want (%d, nil)", n, err, len(input))
				}
			}()
		}

		buf := make([]byte, count*len(input))
		for i := 0; i < len(buf); i += readSize {
			if n, err := r.Read(buf[i : i+readSize]); n != readSize || err != nil {
				t.Errorf("Read() = (%d, %v); want (%d, nil)", n, err, readSize)
			}
		}

		// Since each Write is fully gated, if multiple Read calls were needed,
		// the contents of Write should still appear together in the output.
		got := string(buf)
		want := strings.Repeat(input, count)
		if got != want {
			t.Errorf("got: %q; want: %q", got, want)
		}
	})

	t.Run("Read", func(t *testing.T) {
		r, w := factory()

		c := make(chan []byte, count*len(input)/readSize)
		for i := 0; i < cap(c); i++ {
			go func() {
				time.Sleep(time.Millisecond) // Increase probability of race
				buf := make([]byte, readSize)
				if n, err := r.Read(buf); n != readSize || err != nil {
					t.Errorf("Read() = (%d, %v); want (%d, nil)", n, err, readSize)
				}
				c <- buf
			}()
		}

		for i := 0; i < count; i++ {
			if n, err := w.Write([]byte(input)); n != len(input) || err != nil {
				t.Errorf("Write() = (%d, %v); want (%d, nil)", n, err, len(input))
			}
		}

		// Since each read is independent, the only guarantee about the output
		// is that it is a permutation of the input in readSized groups.
		got := make([]byte, 0, count*len(input))
		for i := 0; i < cap(c); i++ {
			got = append(got, <-c...)
		}
		got = sortBytesInGroups(got, readSize)
		want := bytes.Repeat([]byte(input), count)
		want = sortBytesInGroups(want, readSize)
		if string(got) != string(want) {
			t.Errorf("got: %q; want: %q", got, want)
		}
	})
}

func TestPipeConcurrent(t *testing.T) { testPipeFactories(t, testPipeConcurrent) }

func sortBytesInGroups(b []byte, n int) []byte {
	var groups [][]byte
	for len(b) > 0 {
		groups = append(groups, b[:n])
		b = b[n:]
	}
	sort.Slice(groups, func(i, j int) bool { return bytes.Compare(groups[i], groups[j]) < 0 })
	return bytes.Join(groups, nil)
}
