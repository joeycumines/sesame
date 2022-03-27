package testutil

import (
	"github.com/joeycumines/sesame/internal/testing"
	stdtesting "testing"
)

type (
	T = testing.T

	TB = testing.TB

	TestingT = testing.TestingT

	Runner = testing.Runner

	RunnerOption = testing.RunnerOption

	RunnerOptions = testing.RunnerOptions

	RunnerTest = testing.RunnerTest
)

var (
	OptRunner = RunnerOptions{}
)

func WrapT(t *stdtesting.T) T { return testing.WrapT(t) }

func NewRunner(options ...RunnerOption) (*Runner, error) { return testing.NewRunner(options...) }
