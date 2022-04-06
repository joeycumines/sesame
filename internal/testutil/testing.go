package testutil

import (
	"github.com/joeycumines/sesame/internal/testing"
)

type (
	T = testing.T

	TG = testing.TG

	TB = testing.TB

	TestingT = testing.TestingT

	Unwrapper = testing.Unwrapper

	DepthLimiter = testing.DepthLimiter

	Hook = testing.Hook
)

var (
	Wrap      = testing.Wrap
	Parallel  = testing.Parallel
	SkipRegex = testing.SkipRegex
)
