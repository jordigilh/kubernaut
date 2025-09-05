//go:build integration
// +build integration

package shared

import (
	"github.com/jordigilh/kubernaut/test/integration/shared/testenv"
)

// Re-export types and functions for compatibility
type TestEnvironment = testenv.TestEnvironment

var (
	SetupTestEnvironment = testenv.SetupTestEnvironment
	SetupFakeEnvironment = testenv.SetupFakeEnvironment
)
