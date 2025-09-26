//go:build e2e
// +build e2e

package platform

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-PLATFORM-OPERATIONS-E2E-SUITE-001: Platform Operations E2E Test Suite Organization
// Business Impact: Ensures comprehensive validation of platform operations and multi-cluster capabilities
// Stakeholder Value: Executive confidence in platform-wide automation and resource management for business operations

func TestPlatformOperationsE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Platform Operations E2E Business Suite")
}
