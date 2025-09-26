//go:build integration
// +build integration

package bootstrap

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-DEV-BOOTSTRAP-001: Development Bootstrap Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Development Bootstrap business logic
// Stakeholder Value: Provides executive confidence in Development Bootstrap testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Development Bootstrap capabilities
// Business Impact: Ensures all Development Bootstrap components deliver measurable system reliability
// Business Outcome: Test suite framework enables Development Bootstrap validation

func TestUbootstrap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Development Bootstrap Suite")
}
