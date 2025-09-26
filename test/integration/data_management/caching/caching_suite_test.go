//go:build integration
// +build integration

package caching

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-DATA-CACHE-001: Data Caching Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Data Caching business logic
// Stakeholder Value: Provides executive confidence in Data Caching testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Data Caching capabilities
// Business Impact: Ensures all Data Caching components deliver measurable system reliability
// Business Outcome: Test suite framework enables Data Caching validation

func TestUcaching(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Caching Suite")
}
