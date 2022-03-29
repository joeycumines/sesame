package testutil

import (
	"github.com/joeycumines/sesame/internal/testing"
)

type (
	T = testing.T

	TG = testing.TG

	TB = testing.TB

	TestingT = testing.TestingT

	Runner = testing.Runner

	RunnerOption = testing.RunnerOption

	RunnerOptions = testing.RunnerOptions

	RunnerTest = testing.RunnerTest

	Unwrapper = testing.Unwrapper

	DepthLimiter = testing.DepthLimiter

	Hook = testing.Hook
)

var (
	OptRunner = testing.OptRunner
	Wrap      = testing.Wrap
	NewRunner = testing.NewRunner
	Parallel  = testing.Parallel
	SkipRegex = testing.SkipRegex
)
