// Package testing extends the core testing package.
package testing

import (
	"testing"
	"time"
)

type (
	// T is used in place of *testing.T in order to support more fine-grained control the concurrency of the test cases.
	T interface {
		TB
		Run(name string, f func(t T)) bool
		Parallel()
		Deadline() (deadline time.Time, ok bool)
	}

	tI = T

	// TB is a copy of testing.TB w/o the unexported method.
	TB interface {
		Cleanup(func())
		Error(args ...any)
		Errorf(format string, args ...any)
		Fail()
		FailNow()
		Failed() bool
		Fatal(args ...any)
		Fatalf(format string, args ...any)
		Helper()
		Log(args ...any)
		Logf(format string, args ...any)
		Name() string
		Setenv(key, value string)
		Skip(args ...any)
		SkipNow()
		Skipf(format string, args ...any)
		Skipped() bool
		TempDir() string
	}

	wrappedT struct {
		*testing.T
	}
)

var (
	// compile time assertions

	_ T = wrappedT{}
)

// TODO add a wrapper to get a T w/o implementing it yourself, requires (unsupported) self-referential type constraints

// WrapT wraps testing.T to implement T
func WrapT(t *testing.T) T { return wrappedT{t} }

func (t wrappedT) Run(name string, f func(t T)) bool {
	return t.T.Run(name, func(t *testing.T) { f(WrapT(t)) })
}
