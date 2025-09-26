//go:build e2e
// +build e2e

package toolsetserver

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-TOOLSET-E2E-SUITE-001: Dynamic Toolset Server E2E Test Suite Organization
// Business Impact: Ensures comprehensive validation of dynamic toolset server business workflows
// Stakeholder Value: Executive confidence in HolmesGPT integration and toolset management capabilities

func TestDynamicToolsetServerE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dynamic Toolset Server E2E Business Workflow Suite")
}
