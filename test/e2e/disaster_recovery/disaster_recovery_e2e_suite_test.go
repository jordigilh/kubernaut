//go:build e2e
// +build e2e

package disasterrecovery

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-DISASTER-RECOVERY-E2E-SUITE-001: Disaster Recovery and Resilience E2E Test Suite Organization
// Business Impact: Ensures comprehensive validation of disaster recovery capabilities and system resilience for business continuity
// Stakeholder Value: Executive confidence in business continuity, disaster recovery capabilities, and disaster preparedness

func TestDisasterRecoveryE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Disaster Recovery and Resilience E2E Business Suite")
}
