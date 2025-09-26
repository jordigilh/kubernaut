//go:build integration
// +build integration

package failover

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-PERF-FAILOVER-001: Performance Failover Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Performance Failover business logic
// Stakeholder Value: Provides executive confidence in Performance Failover testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Performance Failover capabilities
// Business Impact: Ensures all Performance Failover components deliver measurable system reliability
// Business Outcome: Test suite framework enables Performance Failover validation

func TestUfailover(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Performance Failover Suite")
}
