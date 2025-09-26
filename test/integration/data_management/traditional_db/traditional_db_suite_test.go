//go:build integration
// +build integration

package traditional_db

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-DATA-DB-001: Data Traditional Database Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Data Traditional Database business logic
// Stakeholder Value: Provides executive confidence in Data Traditional Database testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Data Traditional Database capabilities
// Business Impact: Ensures all Data Traditional Database components deliver measurable system reliability
// Business Outcome: Test suite framework enables Data Traditional Database validation

func TestUtraditionalUdb(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Traditional Database Suite")
}
