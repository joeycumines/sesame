package ionet

import (
	"errors"
	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/stream"
	"io"
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
					values []string
				)
				add = func(v string) {
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

			conn, err := WrapPipe(stream.Pipe{Writer: mockPipeWriter(
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
			)})
			if err != nil {
				t.Fatal(err)
			}
			if n, err := conn.Read([]byte{0}); err != io.EOF || n != 0 {
				t.Error(n, err)
			}

			var wg sync.WaitGroup
			wg.Add(1)
			stop := make(chan struct{})

			for i := 0; i < 50; i++ {
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
			for _, v := range values {
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
					t.Fatal(v)
				default:
					t.Fatal(last)
				}
			}
			if last != `c` {
				t.Error(last)
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
