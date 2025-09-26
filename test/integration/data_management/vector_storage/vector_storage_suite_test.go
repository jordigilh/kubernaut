//go:build integration
// +build integration

package vector_storage

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-DATA-VECTOR-001: Data Vector Storage Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Data Vector Storage business logic
// Stakeholder Value: Provides executive confidence in Data Vector Storage testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Data Vector Storage capabilities
// Business Impact: Ensures all Data Vector Storage components deliver measurable system reliability
// Business Outcome: Test suite framework enables Data Vector Storage validation

func TestUvectorUstorage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Vector Storage Suite")
}
