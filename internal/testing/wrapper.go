package testing

import (
	"fmt"
	"regexp"
)

type (
	Unwrapper interface {
		Unwrap() T
	}

	DepthLimiter struct {
		T
		Depth int
	}

	// Hook will run Func before each subtest.
	Hook struct {
		T
		Func func(t T)
	}
)

var (
	// compile time assertions

	_ T         = DepthLimiter{}
	_ T         = Hook{}
	_ Unwrapper = Hook{}
)

// Parallel will call T.Parallel on every subtest.
// WARNING remember to ensure tests are parallel safe, e.g. validate iterated variables and goroutine checks.
func Parallel(t T) T {
	return Hook{
		T:    t,
		Func: func(t T) { t.Parallel() },
	}
}

func SkipRegex(t T, regex *regexp.Regexp) T {
	return Hook{
		T: t,
		Func: func(t T) {
			fmt.Println(t.Name())
			if name := t.Name(); regex.MatchString(name) {
				t.Skipf(`skipped test %s: matched regex %s`, name, regex)
			}
		},
	}
}

func (x DepthLimiter) Run(name string, f func(t T)) bool {
	return x.T.Run(name, func(t T) {
		if depth := x.Depth - 1; depth > 0 {
			wt := x
			wt.T = t
			wt.Depth = depth
			t = wt
		} else if tw, ok := t.(Unwrapper); ok {
			t = tw.Unwrap()
		}
		f(t)
	})
}

// Unwrap is disabled as it's problematic to have multiple unwrappers.
func (x DepthLimiter) Unwrap() {
	panic(`unwrap not supported`)
}

func (x Hook) Unwrap() T { return x.T }

func (x Hook) Run(name string, f func(t T)) bool {
	return x.T.Run(name, func(t T) {
		x.Func(t)
		wt := x
		wt.T = t
		f(wt)
	})
}
