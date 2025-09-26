//go:build e2e
// +build e2e

package businesscontinuity

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-BUSINESS-CONTINUITY-E2E-SUITE-001: Business Continuity E2E Test Suite Organization
// Business Impact: Ensures comprehensive validation of business continuity and disaster recovery capabilities
// Stakeholder Value: Executive confidence in business resilience and recovery capabilities during system failures

func TestBusinessContinuityE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Business Continuity E2E Workflow Suite")
}
