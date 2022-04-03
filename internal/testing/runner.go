package testing

import (
	"math"
	"sync"
	"sync/atomic"
)

type (
	// Runner implements an internalised test runner.
	Runner struct {
		tI
		mu    sync.RWMutex
		wg    *sync.WaitGroup
		tests map[*RunnerTest]struct{}
	}

	RunnerOption func(c *runnerConfig)

	runnerConfig struct {
		t T
	}

	RunnerOptions struct{}

	RunnerTest struct {
		tI
		runner   *Runner
		runCh    chan bool // gets sent the return value for Runner.Run
		parallel int32
		failures int32
	}
)

var (
	OptRunner = RunnerOptions{}

	// compile time assertions
	_ T = (*RunnerTest)(nil)
	_ T = (*Runner)(nil)
)

func NewRunner(options ...RunnerOption) (*Runner, error) {
	var c runnerConfig
	for _, o := range options {
		o(&c)
	}
	r := Runner{
		tI:    c.t,
		wg:    new(sync.WaitGroup),
		tests: make(map[*RunnerTest]struct{}),
	}
	r.wg.Add(1)
	return &r, nil
}

func (RunnerOptions) T(t T) RunnerOption { return func(c *runnerConfig) { c.t = t } }

func (x *Runner) Wait() {
	x.mu.Lock()
	defer x.mu.Unlock()
	if wg := x.wg; wg != nil {
		x.wg = nil
		wg.Done()
		wg.Wait()
	}
}

func (x *Runner) Failures() (n int64) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	for t := range x.tests {
		n += t.Failures()
		if n < 0 {
			return math.MaxInt64
		}
	}
	return
}

func (x *Runner) Run(name string, f func(t T)) bool { return x.run(func(t *RunnerTest) { f(t) }) }

func (x *Runner) run(f func(t *RunnerTest)) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	t := RunnerTest{
		tI:     x,
		runner: x,
		runCh:  make(chan bool, 1),
	}
	x.addTest(&t)
	go x.runTest(&t, f)
	return <-t.runCh
}

func (x *Runner) addTest(t *RunnerTest) {
	x.wg.Add(1)
	if _, ok := x.tests[t]; ok {
		panic(`sesame/internal/testing: runner test added multiple times`)
	}
	x.tests[t] = struct{}{}
}

func (x *Runner) runTest(t *RunnerTest, f func(t *RunnerTest)) {
	// should be locked (until signaled parallel or done)
	if _, ok := x.tests[t]; !ok {
		panic(`sesame/internal/testing: runner test ran but not added`)
	}
	var fine bool
	defer func() {
		if !fine {
			t.fail()
		}
		select {
		case t.runCh <- !x.Failed():
		default:
		}
		close(t.runCh)
		x.wg.Done()
	}()
	f(t)
	fine = true
}

func (x *RunnerTest) Parallel() {
	if atomic.AddInt32(&x.parallel, 1) != 1 {
		atomic.StoreInt32(&x.parallel, 1)
		panic(`sesame/internal/testing: runner t.Parallel called multiple times`)
	}
	select {
	case x.runCh <- true:
	default:
	}
}

func (x *RunnerTest) fail() {
	if atomic.AddInt32(&x.failures, 1) < 0 {
		atomic.StoreInt32(&x.failures, math.MaxInt32)
	}
}

func (x *RunnerTest) Failures() (n int64) {
	n = int64(atomic.LoadInt32(&x.failures))
	if n < 0 {
		n = math.MaxInt32
	}
	return
}

func (x *RunnerTest) Failed() bool { return atomic.LoadInt32(&x.failures) != 0 }

// TODO implement wrappers for all failure methods
