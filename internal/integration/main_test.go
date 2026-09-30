package integration_test

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

var runIntegration = flag.Bool("integration", false, "run integration tests")

func TestMain(m *testing.M) {
	flag.Parse()
	if !*runIntegration {
		fmt.Println("skipping integration tests: pass -integration to enable")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func skipUnlessIntegration(t *testing.T) {
	t.Helper()
	if !*runIntegration {
		t.Skip("skipping integration test: pass -integration to enable")
	}
}

func TestGating(t *testing.T) {
	skipUnlessIntegration(t)
	t.Log("integration test executed successfully")
}
