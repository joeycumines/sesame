// Package testing extends the core testing package.
package testing

import (
	"testing"
	"time"
)

type (
	// T is used in place of *testing.T in order to support more fine-grained control the concurrency of the test cases.
	T interface {
		TG
		Run(name string, f func(t T)) bool
	}

	tI = T

	// TG is all of T except the recursive method.
	// It's to deal with Go 1.18's limited generic support.
	TG interface {
		TB
		Parallel()
		Deadline() (deadline time.Time, ok bool)
	}

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

	// TestingT implements T using testing.T
	TestingT struct {
		*testing.T
	}
)

var (
	// compile time assertions

	_ T = TestingT{}
)

// Wrap wraps testing.T to implement T.
// TODO add support to get a T w/o implementing it yourself, requires (unsupported) self-referential type constraints
func Wrap(t *testing.T) T { return TestingT{t} }

func (t TestingT) Run(name string, f func(t T)) bool {
	return t.T.Run(name, func(t *testing.T) { f(Wrap(t)) })
}
