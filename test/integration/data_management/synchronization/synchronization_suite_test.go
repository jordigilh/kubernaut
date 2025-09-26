//go:build integration
// +build integration

package synchronization

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-DATA-SYNC-001: Data Synchronization Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Data Synchronization business logic
// Stakeholder Value: Provides executive confidence in Data Synchronization testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Data Synchronization capabilities
// Business Impact: Ensures all Data Synchronization components deliver measurable system reliability
// Business Outcome: Test suite framework enables Data Synchronization validation

func TestUsynchronization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Synchronization Suite")
}
