//go:build e2e
// +build e2e

package security

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-SECURITY-PERFORMANCE-E2E-SUITE-001: Security and Performance E2E Test Suite Organization
// Business Impact: Ensures comprehensive validation of security controls and performance characteristics for business operations
// Stakeholder Value: Executive confidence in secure and performant automated business operations

func TestSecurityPerformanceE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security and Performance E2E Business Workflow Suite")
}
