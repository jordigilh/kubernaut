//go:build integration
// +build integration

package integration

import (
	"github.com/jordigilh/prometheus-alerts-slm/test/integration/testenv"
)

// Re-export types and functions for backward compatibility
type TestEnvironment = testenv.TestEnvironment

var (
	SetupTestEnvironment = testenv.SetupTestEnvironment
	SetupFakeEnvironment = testenv.SetupFakeEnvironment
)
