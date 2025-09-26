//go:build integration
// +build integration

package notifications

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-INT-NOTIFY-001: Integration Notifications Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Integration Notifications business logic
// Stakeholder Value: Provides executive confidence in Integration Notifications testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Integration Notifications capabilities
// Business Impact: Ensures all Integration Notifications components deliver measurable system reliability
// Business Outcome: Test suite framework enables Integration Notifications validation

func TestUnotifications(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Notifications Suite")
}
