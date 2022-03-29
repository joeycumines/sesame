package testutil

import (
	"bytes"
	"fmt"
	"runtime"
	"runtime/pprof"
	"time"
)

const (
	waitNumGoroutinesDefault     = time.Second
	waitNumGoroutinesNumerator   = 1
	waitNumGoroutinesDenominator = 1
	waitNumGoroutinesMin         = time.Millisecond * 50
)

type (
	GoroutineChecker struct {
		T
		Increase bool
		Wait     time.Duration
	}
)

var (
	// compile time assertions

	_ T         = GoroutineChecker{}
	_ Unwrapper = GoroutineChecker{}
)

func WaitNumGoroutines(wait time.Duration, fn func(n int) bool) (n int) {
	if wait == 0 {
		wait = waitNumGoroutinesDefault
	}
	wait *= waitNumGoroutinesNumerator
	wait /= waitNumGoroutinesDenominator
	if wait < waitNumGoroutinesMin {
		wait = waitNumGoroutinesMin
	}
	count := int(wait / waitNumGoroutinesMin)
	wait /= time.Duration(count)
	n = runtime.NumGoroutine()
	for i := 0; i < count && !fn(n); i++ {
		time.Sleep(wait)
		runtime.GC()
		n = runtime.NumGoroutine()
	}
	return
}

func CheckNumGoroutines(t TB, start int, increase bool, wait time.Duration) {
	if t != nil {
		t.Helper()
	}
	errorf := func(format string, values ...interface{}) {
		err := fmt.Errorf(format, values...)
		if t != nil {
			t.Error(err)
		} else {
			panic(err)
		}
	}
	var fn func(n int) bool
	if increase {
		fn = func(n int) bool { return start < n }
	} else {
		fn = func(n int) bool { return start >= n }
	}
	if now := WaitNumGoroutines(wait, fn); increase {
		if start >= now {
			errorf("too few goroutines: -%d\n%s", start-now+1, DumpGoroutineStacktrace())
		}
	} else if start < now {
		errorf("too many goroutines: +%d\n%s", now-start, DumpGoroutineStacktrace())
	}
}

func CleanupCheckNumGoroutines(t TB, start int, increase bool, wait time.Duration) {
	t.Cleanup(func() { CheckNumGoroutines(t, start, increase, wait) })
}

func DumpGoroutineStacktrace() string {
	var b bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&b, 1)
	return b.String()
}

func (x GoroutineChecker) Unwrap() T { return x.T }

func (x GoroutineChecker) Run(name string, f func(t T)) bool {
	return x.T.Run(name, func(t T) {
		CleanupCheckNumGoroutines(t, runtime.NumGoroutine(), x.Increase, x.Wait)
		wt := x
		wt.T = t
		f(wt)
	})
}
