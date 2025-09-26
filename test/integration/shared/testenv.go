//go:build integration
// +build integration

package shared

import (
	"github.com/jordigilh/kubernaut/test/integration/shared/testenv"
)

// Re-export types and functions for compatibility

var (
	SetupTestEnvironment = testenv.SetupTestEnvironment
	SetupEnvironment     = testenv.SetupEnvironment
)
