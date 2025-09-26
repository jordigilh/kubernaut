//go:build integration
// +build integration

package recovery

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-PERF-RECOVERY-001: Performance Recovery Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Performance Recovery business logic
// Stakeholder Value: Provides executive confidence in Performance Recovery testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Performance Recovery capabilities
// Business Impact: Ensures all Performance Recovery components deliver measurable system reliability
// Business Outcome: Test suite framework enables Performance Recovery validation

func TestUrecovery(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Performance Recovery Suite")
}
