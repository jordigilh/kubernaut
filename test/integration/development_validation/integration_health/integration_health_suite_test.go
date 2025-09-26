//go:build integration
// +build integration

package integration_health

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-DEV-HEALTH-001: Development Integration Health Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Development Integration Health business logic
// Stakeholder Value: Provides executive confidence in Development Integration Health testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Development Integration Health capabilities
// Business Impact: Ensures all Development Integration Health components deliver measurable system reliability
// Business Outcome: Test suite framework enables Development Integration Health validation

func TestUintegrationUhealth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Development Integration Health Suite")
}
