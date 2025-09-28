//go:build unit
// +build unit

package adaptive_orchestration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestRunner bootstraps the Ginkgo test suite
func TestUorchestrationUsuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UorchestrationUsuite Suite")
}
