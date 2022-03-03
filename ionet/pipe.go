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

// This pipe implementation is based off net.Pipe, extended with parts of io.Pipe, in order to add better close
// functionality. Both implementations are very similar, though they are structured differently (I believe the current
// iteration of io.Pipe is based off the current iteration of net.Pipe). The resultant (merged) implementation
// has the "remote" fields moved to the other side of the pipe. Internal state is on the write side, though, with the
// read implementation still on the opposite side, and largely the same as the original net.Pipe.
//
// This implementation should be updated with on any relevant changes to the original implementations.
// https://github.com/golang/go/compare/851ecea4cc99ab276109493477b2c7e30c253ea8...master
// https://github.com/golang/go/blob/master/src/io/pipe.go
// https://github.com/golang/go/blob/master/src/io/pipe_test.go
// https://github.com/golang/go/blob/master/src/net/pipe.go
// https://github.com/golang/go/blob/master/src/net/pipe_test.go

package ionet

import (
	"bytes"
	"github.com/joeycumines/sesame/stream"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

type (
	ConnPipe struct {
		wrMu  sync.Mutex
		wrMsg *wrMsg
		wrCh  chan []byte
		rdCh  chan rdMsg

		once sync.Once // Protects closing done
		done chan struct{}
		rerr onceError
		werr onceError

		readDeadline  pipeDeadline
		writeDeadline pipeDeadline

		remote       *ConnPipe
		localWriter  *ConnPipeWriter
		remoteReader *ConnPipeReader
	}

	ConnPipeReader struct{ p *ConnPipe }

	ConnPipeWriter struct{ p *ConnPipe }

	// pipeDeadline is an abstraction for handling timeouts.
	pipeDeadline struct {
		mu     sync.Mutex // Guards timer and cancel
		timer  *time.Timer
		cancel chan struct{} // Must be non-nil
	}

	// onceError is an object that will only store an error once.
	onceError struct {
		sync.Mutex // guards following
		err        error
	}

	pipeAddr struct{}

	rdMsg struct {
		n   int
		err error
	}

	wrMsg struct {
		wr   []byte
		rd   rdMsg
		done chan struct{}
	}
)

var (
	_ net.Conn          = (*ConnPipe)(nil)
	_ io.WriterTo       = (*ConnPipe)(nil)
	_ io.WriterTo       = (*ConnPipeReader)(nil)
	_ stream.PipeReader = (*ConnPipeReader)(nil)
	_ stream.PipeWriter = (*ConnPipeWriter)(nil)
)

// Pipe creates a synchronous, in-memory, full duplex
// network connection; both ends implement the Conn interface.
// Reads on one end are matched with writes on the other,
// copying data directly between the two; there is no internal
// buffering.
func Pipe() (c1 *ConnPipe, c2 *ConnPipe) {
	c1 = &ConnPipe{
		wrCh:          make(chan []byte),
		rdCh:          make(chan rdMsg),
		done:          make(chan struct{}),
		readDeadline:  makePipeDeadline(),
		writeDeadline: makePipeDeadline(),
	}
	c2 = &ConnPipe{
		wrCh:          make(chan []byte),
		rdCh:          make(chan rdMsg),
		done:          make(chan struct{}),
		readDeadline:  makePipeDeadline(),
		writeDeadline: makePipeDeadline(),
	}
	c1.remote, c2.remote = c2, c1
	c1.localWriter, c1.remoteReader = &ConnPipeWriter{p: c1}, &ConnPipeReader{p: c2}
	c2.localWriter, c2.remoteReader = &ConnPipeWriter{p: c2}, &ConnPipeReader{p: c1}
	return
}

func (p *ConnPipe) pipe() (*ConnPipeReader, *ConnPipeWriter) { return p.remoteReader, p.localWriter }

func (p *ConnPipe) SendPipe() (*ConnPipeReader, *ConnPipeWriter) { return p.pipe() }

func (p *ConnPipe) ReceivePipe() (*ConnPipeReader, *ConnPipeWriter) { return p.remote.pipe() }

func makePipeDeadline() pipeDeadline {
	return pipeDeadline{cancel: make(chan struct{})}
}

// set sets the point in time when the deadline will time out.
// A timeout event is signaled by closing the channel returned by waiter.
// Once a timeout has occurred, the deadline can be refreshed by specifying a
// t value in the future.
//
// A zero value for t prevents timeout.
func (d *pipeDeadline) set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil && !d.timer.Stop() {
		<-d.cancel // Wait for the timer callback to finish and close cancel
	}
	d.timer = nil

	// Time is zero, then there is no deadline.
	closed := isClosedChan(d.cancel)
	if t.IsZero() {
		if closed {
			d.cancel = make(chan struct{})
		}
		return
	}

	// Time in the future, setup a timer to cancel in the future.
	if dur := time.Until(t); dur > 0 {
		if closed {
			d.cancel = make(chan struct{})
		}
		d.timer = time.AfterFunc(dur, func() {
			close(d.cancel)
		})
		return
	}

	// Time in the past, so close immediately.
	if !closed {
		close(d.cancel)
	}
}

// wait returns a channel that is closed when the deadline is exceeded.
func (d *pipeDeadline) wait() chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cancel
}

func isClosedChan(c <-chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

func (*ConnPipe) LocalAddr() net.Addr  { return pipeAddr{} }
func (*ConnPipe) RemoteAddr() net.Addr { return pipeAddr{} }

func (p *ConnPipe) WriteTo(w io.Writer) (n int64, err error) {
	type R struct {
		N   int64
		Err error
	}
	ch := make(chan R)
	for err == nil {
		var (
			nr int64
			ok bool
		)
		nr, err = p.read(func(b []byte) (int64, error) {
			ok = true
			go func() {
				r := R{0, stream.ErrPanic}
				defer func() { ch <- r }()
				r.N, r.Err = bytes.NewReader(b).WriteTo(w)
			}()
			select {
			case <-p.remote.done:
				// the write side of the pipe is closed - there's no more data to be sent, so we need to immediately
				// unblock the writer, otherwise it may deadlock
				return int64(len(b)), nil
			case r := <-ch:
				ok = false
				return r.N, r.Err
			}
		})
		if ok {
			r := <-ch
			nr, err = r.N, r.Err
		}
		n += nr
	}
	return
}

func (p *ConnPipe) Read(b []byte) (int, error) {
	n, err := p.read(func(bw []byte) (int64, error) { return int64(copy(b, bw)), nil })
	return int(n), err
}

// read is implemented on the opposite side, due to the way the deadline and pipe state is structured
func (p *ConnPipe) read(fn func(b []byte) (int64, error)) (n int64, err error) {
	var noWrap bool
	defer func() {
		if !noWrap && err != nil && err != io.EOF && err != io.ErrClosedPipe {
			err = &net.OpError{Op: "read", Net: "pipe", Err: err}
		}
	}()

	switch {
	case isClosedChan(p.remote.done):
		return 0, p.remote.remoteReadError()
	case isClosedChan(p.readDeadline.wait()):
		return 0, os.ErrDeadlineExceeded
	}

	select {
	case b := <-p.remote.wrCh:
		n, err = fn(b)
		if err != nil {
			noWrap = true
		}
		p.remote.rdCh <- rdMsg{int(n), err}
		return
	case <-p.remote.done:
		return 0, p.remote.remoteReadError()
	case <-p.readDeadline.wait():
		return 0, os.ErrDeadlineExceeded
	}
}

func (p *ConnPipe) Write(b []byte) (int, error) {
	n, err := p.write(b)
	if err != nil && err != io.ErrClosedPipe {
		err = &net.OpError{Op: "write", Net: "pipe", Err: err}
	}
	return n, err
}

func (p *ConnPipe) write(b []byte) (n int, err error) {
	switch {
	case isClosedChan(p.done):
		return 0, p.localWriteError()
	case isClosedChan(p.writeDeadline.wait()):
		return 0, os.ErrDeadlineExceeded
	}

	p.wrMu.Lock() // Ensure entirety of b is written together
	defer p.wrMu.Unlock()

	if p.wrMsg != nil {
		select {
		case <-p.done:
			return 0, p.localWriteError()
		case <-p.writeDeadline.wait():
			return 0, os.ErrDeadlineExceeded
		case <-p.wrMsg.done:
		}
		p.wrMsg = nil
	}

	{
		c := make([]byte, len(b))
		copy(c, b)
		b = c
	}

	for once := true; err == nil && (once || len(b) > 0); once = false {
		p.wrMsg = &wrMsg{
			wr:   b,
			done: make(chan struct{}),
		}
		go p.handleWrite(p.wrMsg)
		select {
		case <-p.wrMsg.done:
			b = b[p.wrMsg.rd.n:]
			n += p.wrMsg.rd.n
			err = p.wrMsg.rd.err
			p.wrMsg = nil
		case <-p.done:
			// if the other end is also closing, we can wait for that as well
			select {
			case <-p.remote.done:
				select {
				case <-p.wrMsg.done:
					b = b[p.wrMsg.rd.n:]
					n += p.wrMsg.rd.n
					err = p.wrMsg.rd.err
					p.wrMsg = nil
				case <-p.writeDeadline.wait():
					err = os.ErrDeadlineExceeded
				}
			default:
				err = p.localWriteError()
			}
		case <-p.writeDeadline.wait():
			err = os.ErrDeadlineExceeded
		}
	}

	return
}

func (p *ConnPipe) handleWrite(msg *wrMsg) {
	defer func() {
		msg.wr = nil
		close(msg.done)
	}()
	select {
	case p.wrCh <- msg.wr:
		msg.rd = <-p.rdCh
	case <-p.done:
		msg.rd = rdMsg{0, p.localWriteError()}
	case <-p.writeDeadline.wait():
		msg.rd = rdMsg{0, os.ErrDeadlineExceeded}
	}
}

func (p *ConnPipe) SetDeadline(t time.Time) (err error) {
	err = io.ErrClosedPipe
	if p.SetReadDeadline(t) == nil {
		err = nil
	}
	if p.SetWriteDeadline(t) == nil {
		err = nil
	}
	return
}

func (p *ConnPipe) SetReadDeadline(t time.Time) error {
	if isClosedChan(p.remote.done) {
		return io.ErrClosedPipe
	}
	p.readDeadline.set(t)
	return nil
}

func (p *ConnPipe) SetWriteDeadline(t time.Time) error {
	if isClosedChan(p.done) {
		return io.ErrClosedPipe
	}
	p.writeDeadline.set(t)
	return nil
}

func (p *ConnPipe) Close() error {
	// close local write and remote read
	// (closes pipes in both directions)
	// NOTE it's important to close remote read first, due to the behavior of write
	_ = p.remote.closeRemoteRead(nil)
	_ = p.closeLocalWrite(nil)
	return nil
}

func (p *ConnPipe) closeLocalWrite(err error) error {
	if err == nil {
		err = io.EOF
	}
	p.werr.Store(err)
	p.closeDone()
	return nil
}

func (p *ConnPipe) closeRemoteRead(err error) error {
	if err == nil {
		err = io.ErrClosedPipe
	}
	p.rerr.Store(err)
	p.closeDone()
	return nil
}

func (p *ConnPipe) closeDone() { p.once.Do(func() { close(p.done) }) }

func (p *ConnPipe) localWriteError() error {
	werr := p.werr.Load()
	if rerr := p.rerr.Load(); werr == nil && rerr != nil {
		return rerr
	}
	return io.ErrClosedPipe
}

func (p *ConnPipe) remoteReadError() error {
	rerr := p.rerr.Load()
	if werr := p.werr.Load(); rerr == nil && werr != nil {
		return werr
	}
	return io.ErrClosedPipe
}

func (p *ConnPipeWriter) Write(b []byte) (n int, err error) { return p.p.Write(b) }

func (p *ConnPipeWriter) Close() error { return p.CloseWithError(nil) }

func (p *ConnPipeWriter) CloseWithError(err error) error { return p.p.closeLocalWrite(err) }

func (p *ConnPipeReader) Read(b []byte) (n int, err error) { return p.p.Read(b) }

func (p *ConnPipeReader) Close() error { return p.CloseWithError(nil) }

func (p *ConnPipeReader) CloseWithError(err error) error { return p.p.remote.closeRemoteRead(err) }

func (p *ConnPipeReader) WriteTo(w io.Writer) (n int64, err error) { return p.p.WriteTo(w) }

func (a *onceError) Store(err error) {
	a.Lock()
	defer a.Unlock()
	if a.err != nil {
		return
	}
	a.err = err
}

func (a *onceError) Load() error {
	a.Lock()
	defer a.Unlock()
	return a.err
}
