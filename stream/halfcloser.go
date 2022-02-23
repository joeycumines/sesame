package stream

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

type (
	// HalfCloser may be used to support patterns involving sending of EOF, e.g. to emulate unix-style IO redirection.
	//
	// This implementation addresses multiple aspects of the target problem case. Firstly, it provides controls around
	// half-close behavior, configured via ClosePolicy implementations. Secondly, it provides support for
	// implementations where the act of closing, for send side of the pipe (PipeWriter), must be synchronised with
	// writes. An example scenario for this case is where EOF is implemented as part of an underlying (transport)
	// stream. This allows the HalfCloser.Close to be called concurrently with writes, e.g. as required for
	// implementations of net.Conn.
	HalfCloser struct {
		// pipe is the underlying Pipe, used to model the full connection, and exposed via HalfCloser.Pipe
		pipe Pipe
		// closePolicy configures the strategy for half-close support.
		// E.g. may be configured with a timeout (to close it completely after a given duration).
		closePolicy ClosePolicy
		// writeMu synchronises Write and the actual half-close part of Close.
		writeMu sync.Mutex
		// writingCh will be sent on write, and together with writingCount can be used to reliably determine if
		// there were any writes either in progress at or started after a certain point.
		writingCh chan struct{}
		// writingCount is only used to handle writes in progress, see writingCh.
		writingCount int32
		// writeClosed indicates half-closed.
		writeClosed bool
		// once is used for Close.
		once sync.Once
		// err is any close error (cached).
		err error
	}

	// HalfCloserOption is an option that may be provided to NewHalfCloser.
	HalfCloserOption func(c *halfCloserConfig)

	// HalfCloserOptions exposes HalfCloserOption implementations as methods, which are available via the OptHalfCloser
	// package variable.
	HalfCloserOptions struct{}

	halfCloserConfig struct {
		pipe        *Pipe
		closePolicy ClosePolicy
	}

	// ClosePolicy models one of the available close behaviors, usable with HalfCloser, implemented by this package.
	ClosePolicy interface {
		// closePolicy is unexported, and returns itself, meaning ClosePolicy implementations may be embedded into a
		// struct, to implement ClosePolicy, and still work correctly (with this package).
		// An unexported method, rather than an exported method returning an interface with an unexported method, was
		// chosen in order to better facilitate the use case of implementing external support for ClosePolicy, w/o
		// limiting the ability to extend the behavior (for external packages).
		// See also UnwrapClosePolicy.
		closePolicy() ClosePolicy
	}

	// WaitRemote is a ClosePolicy and the DefaultClosePolicy, which indicates that implementations (e.g. HalfCloser)
	// should completely close the underlying pipe (e.g. Pipe) if writes are being attempted during half-close of the
	// send pipe (e.g. Pipe.Writer), and that half-close should be synchronised with writes.
	WaitRemote struct{}

	// WaitRemoteTimeout is a ClosePolicy, and indicates that implementations should behave as per WaitRemote, with
	// close of the full pipe (e.g. Pipe) also triggered by the specified timeout from the start of the first half-close
	// attempt, if not otherwise closed prior.
	WaitRemoteTimeout time.Duration
)

var (
	// OptHalfCloser exposes all the options for NewHalfCloser, available as methods.
	OptHalfCloser HalfCloserOptions

	// DefaultClosePolicy is the default behavior used by HalfCloser.
	DefaultClosePolicy ClosePolicy = WaitRemote{}

	// compile time assertions

	_ io.Reader   = (*HalfCloser)(nil)
	_ PipeWriter  = (*HalfCloser)(nil)
	_ ClosePolicy = WaitRemote{}
	_ ClosePolicy = WaitRemoteTimeout(0)
)

func NewHalfCloser(options ...HalfCloserOption) (*HalfCloser, error) {
	var c halfCloserConfig
	for _, o := range options {
		o(&c)
	}

	var r HalfCloser

	// TODO add support for more options that can configure r.pipe

	switch {
	case c.pipe != nil:
		r.pipe = *c.pipe
	}

	if r.pipe.Writer == nil {
		return nil, errors.New(`sesame/stream: half closer requires a pipe writer`)
	}

	r.closePolicy = UnwrapClosePolicy(c.closePolicy)
	r.writingCh = make(chan struct{}, 1)

	return &r, nil
}

// UnwrapClosePolicy is provided for use with other packages' structs, with embedded ClosePolicy implementations.
// Note that panics, within this function, will be recovered, in order to default nil ClosePolicy values (policy, or a
// field of that type embedded in policy) to DefaultClosePolicy.
// This method enables other packages to implement extended ClosePolicy support, for their own use. Note that this
// cannot be used to extend the behavior of implementations in this package.
// See also ClosePolicy and HalfCloser.
func UnwrapClosePolicy(policy ClosePolicy) (unwrapped ClosePolicy) {
	defer func() {
		recover()
		if unwrapped == nil {
			unwrapped = DefaultClosePolicy
		}
	}()
	unwrapped = policy.closePolicy()
	return
}

// Pipe exposes the "full" pipe, used internally by the receiver.
func (x *HalfCloser) Pipe() Pipe { return x.pipe }

func (x *HalfCloser) Read(b []byte) (int, error) { return x.pipe.Read(b) }

func (x *HalfCloser) Write(b []byte) (int, error) {
	// writingCount must be on the outside, so that writingCh can be cleared
	// prior to checking writingCount
	atomic.AddInt32(&x.writingCount, 1)
	defer atomic.AddInt32(&x.writingCount, -1)
	select {
	case x.writingCh <- struct{}{}:
	default:
	}
	x.writeMu.Lock()
	defer x.writeMu.Unlock()
	if x.writeClosed {
		return 0, io.ErrClosedPipe
	}
	return x.pipe.Write(b)
}

func (x *HalfCloser) Close() error { return x.CloseWithError(nil) }

func (x *HalfCloser) CloseWithError(err error) error {
	x.once.Do(func() {
		var success bool
		closePipe := func() func(force bool) {
			var once sync.Once
			return func(force bool) {
				once.Do(func() {
					if force || x.err != nil {
						// note: we only TRY to avoid calling x.pipe.Writer.CloseWithError twice
						pipe := x.pipe
						if success {
							pipe.Writer = nil
						}
						_ = pipe.Close()
					}
				})
			}
		}()
		defer closePipe(false)
		defer func() {
			if !success {
				x.err = ErrPanic
			}
		}()

		var timeout time.Duration
		if policy, ok := x.closePolicy.(WaitRemoteTimeout); ok {
			if policy > 0 {
				timeout = time.Duration(policy)
			}
		} else {
			timeout = -1
		}

		// calls x.pipe.Close if there is either no timeout or if the were/are any writes in progress
		// this is necessary to avoid a deadlock involving the write mutex
		select {
		case <-x.writingCh:
		default:
		}
		if atomic.LoadInt32(&x.writingCount) != 0 || timeout == 0 {
			closePipe(true)
		} else {
			// note: the below is basically an escape hatch for the x.writeMu.Lock call
			var (
				timer   *time.Timer
				timerCh <-chan time.Time
				done    = make(chan struct{})
			)
			defer close(done)
			if timeout > 0 {
				timer = time.NewTimer(timeout)
				timerCh = timer.C
			}
			go func() {
				if timer != nil {
					defer timer.Stop()
				}
				select {
				case <-done:
				case <-x.writingCh:
				case <-timerCh:
				}
				select {
				case <-done:
					// we don't need to close, we've already stopped trying to send EOF
					return
				default:
				}
				closePipe(true)
			}()
		}

		x.writeMu.Lock()
		defer x.writeMu.Unlock()

		x.writeClosed = true

		x.err = x.pipe.Writer.CloseWithError(err)
		success = true
	})

	return x.err
}

// Pipe provides an underlying Pipe for the HalfCloser, note that Pipe.Writer is required.
//
// This method may be accessed via the OptHalfCloser package variable.
func (HalfCloserOptions) Pipe(pipe Pipe) HalfCloserOption {
	return func(c *halfCloserConfig) { c.pipe = &pipe }
}

// ClosePolicy configures the ClosePolicy for the HalfCloser, note that, like the Context option, unless
// ContextWithCancel or/and CloseOnCancel are set, it will not have any significant impact on behavior.
//
// This method may be accessed via the OptHalfCloser package variable.
func (HalfCloserOptions) ClosePolicy(policy ClosePolicy) HalfCloserOption {
	return func(c *halfCloserConfig) { c.closePolicy = policy }
}

func (x WaitRemoteTimeout) closePolicy() ClosePolicy { return x }

func (x WaitRemote) closePolicy() ClosePolicy { return x }
