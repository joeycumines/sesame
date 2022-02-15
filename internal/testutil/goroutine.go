package testutil

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

const (
	waitNumGoroutinesDefault     = time.Second
	waitNumGoroutinesNumerator   = 1
	waitNumGoroutinesDenominator = 1
	waitNumGoroutinesMin         = time.Millisecond * 50
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

func CheckNumGoroutines(t *testing.T, start int, increase bool, wait time.Duration) {
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
			errorf("too few goroutines: -%d", start-now+1)
		}
	} else if start < now {
		errorf("too many goroutines: +%d", now-start)
	}
}
