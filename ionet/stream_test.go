package ionet

import (
	"context"
	"errors"
	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/stream"
	"golang.org/x/net/nettest"
	"io"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestWrapPipe_pipeWriterSafe(t *testing.T) {
	for _, tc := range [...]struct {
		Name string
		Init func() (add func(v string), get func() []string)
	}{
		{
			Name: `racey`,
			Init: func() (add func(v string), get func() []string) {
				var (
					mu     sync.RWMutex
					values []string
				)
				add = func(v string) {
					if v == `c` {
						mu.Lock()
						defer mu.Unlock()
					} else {
						mu.RLock()
						defer mu.RUnlock()
					}
					values = append(values, v)
				}
				get = func() []string {
					return values
				}
				return
			},
		},
		{
			Name: `mutex`,
			Init: func() (add func(v string), get func() []string) {
				var (
					mu     sync.Mutex
					values []string
				)
				add = func(v string) {
					mu.Lock()
					defer mu.Unlock()
					values = append(values, v)
				}
				get = func() []string {
					mu.Lock()
					defer mu.Unlock()
					return values
				}
				return
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)

			add, get := tc.Init()

			someErr := errors.New(`some error`)

			hasWritten := make(chan struct{}, 1)

			conn, err := WrapPipe(stream.OptHalfCloser.Pipe(stream.Pipe{Writer: mockPipeWriter(
				func(b []byte) (n int, err error) {
					add(string(b))
					select {
					case hasWritten <- struct{}{}:
					default:
					}
					return len(b), nil
				},
				func() error {
					t.Error()
					return nil
				},
				func(err error) error {
					if err != nil {
						t.Error(err)
					}
					add(`c`)
					return someErr
				},
			)}))
			if err != nil {
				t.Fatal(err)
			}
			if n, err := conn.Read([]byte{0}); err != io.EOF || n != 0 {
				t.Error(n, err)
			}

			var wg sync.WaitGroup
			wg.Add(1)
			stop := make(chan struct{})

			const countWriters = 50

			for i := 0; i < countWriters; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-stop:
							return
						default:
						}
						if n, err := conn.Write([]byte(`w`)); !((err == nil && n == 1) || (err == io.ErrClosedPipe && n == 0)) {
							t.Error(n, err)
							panic(`ded`)
						}
						time.Sleep(time.Millisecond)
					}
				}()
			}

			for i := 0; i < 100; i++ {
				<-hasWritten
			}

			startClose := make(chan struct{})
			for i := 0; i < 50; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-startClose
					if err := conn.Close(); err != someErr {
						t.Error(err)
					}
				}()
			}
			time.Sleep(time.Millisecond * 100)
			close(startClose)

			wg.Done()
			close(stop)
			wg.Wait()

			if err := conn.Close(); err != someErr {
				t.Error(err)
			}

			values := get()
			if len(values) == 0 {
				t.Fatal()
			}
			last := `w`
			var offset *int
			for _, v := range values {
				if v == `c` {
					if offset != nil {
						t.Error(`multiple closes`)
					} else {
						offset = new(int)
					}
				}
				if offset != nil {
					*offset++
				}
				switch last {
				case `w`:
					switch v {
					case `w`:
					case `c`:
						last = v
					default:
						t.Fatal(v)
					}
				case `c`:
					// can happen occasionally - is not a problem
					if v != `w` {
						t.Error(v)
					}
				default:
					t.Fatal(last)
				}
			}
			if offset == nil || *offset > countWriters+1 {
				var ov int
				if offset != nil {
					ov = *offset
				}
				var recent []string
				for i := len(values) - 1; i >= 0 && len(recent) < 5; i-- {
					recent = append(recent, values[i])
				}
				t.Errorf(`unexpected last value %q (offset %d) in recent values: %q`, last, ov, recent)
			}
		})
	}
}

func TestWrap_noWriterNoReader(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
	conn := Wrap(stream.Pipe{})
	if n, err := conn.Read([]byte{0}); err != io.EOF || n != 0 {
		t.Error(n, err)
	}
	if n, err := conn.Write([]byte{0}); err != io.ErrClosedPipe || n != 0 {
		t.Error(n, err)
	}
	_ = conn.SetDeadline(time.Time{})
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWrap_nettest(t *testing.T) {
	defer testutil.CheckNumGoroutines(t, runtime.NumGoroutine(), false, 0)
	wrapN := func(stream io.ReadWriteCloser, n int) (conn net.Conn) {
		if n <= 0 {
			panic(n)
		}
		for i := 0; i < n; i++ {
			conn = Wrap(stream)
			stream = conn
		}
		return
	}
	wrapPipeGracefulProxy := func(cancelOnStop bool) func(t *testing.T) nettest.MakePipe {
		return func(t *testing.T) nettest.MakePipe {
			return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
				ctx, cancel := context.WithCancel(context.Background())
				targetLocal, targetRemote := net.Pipe()
				sendReader, sendWriter := io.Pipe()
				receiveReader, receiveWriter := io.Pipe()
				go func() {
					var err error
					defer func() {
						_ = targetLocal.Close()
						_ = receiveWriter.CloseWithError(err)
						_ = sendReader.CloseWithError(err)
					}()
					type (
						ioReader   io.Reader
						ioWriter   io.Writer
						readWriter struct {
							ioReader
							ioWriter
						}
					)
					err = stream.Proxy(ctx, readWriter{sendReader, receiveWriter}, targetLocal)
					if err == nil {
						err = targetLocal.Close()
					}
				}()
				options := []stream.HalfCloserOption{
					stream.OptHalfCloser.Pipe(stream.Pipe{
						Reader: receiveReader,
						Writer: sendWriter,
						Closer: stream.Closer(func() error {
							cancel()
							return nil
						}),
					}),
					stream.OptHalfCloser.CloseGuard(func() bool { return ctx.Err() == nil }),
				}

				// deal with some tests, particularly b/RacyRead, not necessarily draining targetRemote, and therefore
				// at risk of not actually processing write being closed
				if cancelOnStop {
					stop = cancel
				} else {
					options = append(options, stream.OptHalfCloser.ClosePolicy(stream.WaitRemoteTimeout(time.Second*5)))
				}

				wrapped, err := WrapPipeGraceful(options...)
				if err != nil {
					return nil, nil, nil, err
				}
				c1, c2 = wrapped, targetRemote
				return
			}
		}
	}
	for _, tc := range [...]struct {
		Name string
		Stop bool
		Init func(t *testing.T) nettest.MakePipe
	}{
		{
			Name: `pipe rwc`,
			Init: func(t *testing.T) nettest.MakePipe {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					type c interface {
						io.ReadWriteCloser
					}
					type C struct{ c }
					a, b := net.Pipe()
					c1, c2 = Wrap(C{a}), Wrap(C{b})
					stop = func() {
						_ = c1.Close()
						_ = c2.Close()
					}
					return
				}
			},
		},
		{
			Name: `pipe dual deep nested`,
			Init: func(t *testing.T) nettest.MakePipe {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					a, b := Pipe()
					c1, c2 = wrapN(a, 20), wrapN(b, 50)
					stop = func() {
						_ = c1.Close()
						_ = c2.Close()
					}
					return
				}
			},
		},
		{
			Name: `pipe rwc+writeto dual`,
			Init: func(t *testing.T) nettest.MakePipe {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					type c interface {
						io.ReadWriteCloser
						io.WriterTo
					}
					type C struct{ c }
					a, b := Pipe()
					c1, c2 = Wrap(C{a}), Wrap(C{b})
					stop = func() {
						_ = c1.Close()
						_ = c2.Close()
					}
					return
				}
			},
		},
		{
			Name: `pipe rwc+writeto single`,
			Stop: true,
			Init: func(t *testing.T) nettest.MakePipe {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					type c interface {
						io.ReadWriteCloser
						io.WriterTo
					}
					type C struct{ c }
					a, b := Pipe()
					c1, c2 = Wrap(C{a}), b
					return
				}
			},
		},
		{
			Name: `stream pair io pipe`,
			Init: func(t *testing.T) nettest.MakePipe {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					a, b := stream.Pair(io.Pipe())(io.Pipe())
					c1, c2 = Wrap(a), Wrap(b)
					stop = func() {
						_ = c1.Close()
						_ = c2.Close()
					}
					return
				}
			},
		},
		{
			Name: `stream pair io pipe half closer`,
			Init: func(t *testing.T) nettest.MakePipe {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					a, b := stream.Pair(io.Pipe())(io.Pipe())
					ac, err := WrapPipe(stream.OptHalfCloser.Pipe(a))
					if err != nil {
						t.Fatal(err)
					}
					bc, err := WrapPipe(stream.OptHalfCloser.Pipe(b))
					if err != nil {
						t.Fatal(err)
					}
					c1, c2 = ac, bc
					stop = func() {
						_ = c1.Close()
						_ = c2.Close()
					}
					return
				}
			},
		},
		{
			// same as above just using this package's pipe impl
			Name: `stream pair ionet pipe half closer`,
			Stop: true,
			Init: func(t *testing.T) nettest.MakePipe {
				return func() (c1 net.Conn, c2 net.Conn, stop func(), _ error) {
					p, _ := Pipe()
					a, b := stream.Pair(p.SendPipe())(p.ReceivePipe())
					ac, err := WrapPipe(stream.OptHalfCloser.Pipe(a))
					if err != nil {
						t.Fatal(err)
					}
					bc, err := WrapPipe(stream.OptHalfCloser.Pipe(b))
					if err != nil {
						t.Fatal(err)
					}
					c1, c2 = ac, bc
					return
				}
			},
		},
		{
			Name: `wrap pipe graceful proxy`,
			Stop: true,
			Init: wrapPipeGracefulProxy(true),
		},
		{
			Name: `wrap pipe graceful proxy no cancel`,
			Stop: true,
			Init: wrapPipeGracefulProxy(false),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			testutil.TestConn(t, tc.Stop, tc.Init)
		})
	}
}
