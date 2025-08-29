//go:build integration
// +build integration

package shared

import (
	"github.com/jordigilh/prometheus-alerts-slm/test/integration/shared/testenv"
)

// Re-export types and functions for backward compatibility
type TestEnvironment = testenv.TestEnvironment

var (
	SetupTestEnvironment = testenv.SetupTestEnvironment
	SetupFakeEnvironment = testenv.SetupFakeEnvironment
)
