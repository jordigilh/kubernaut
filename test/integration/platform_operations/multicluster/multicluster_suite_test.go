//go:build integration
// +build integration

package multicluster

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-PLAT-MULTICLUSTER-001: Platform Multicluster Operations Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Platform Multicluster Operations business logic
// Stakeholder Value: Provides executive confidence in Platform Multicluster Operations testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Platform Multicluster Operations capabilities
// Business Impact: Ensures all Platform Multicluster Operations components deliver measurable system reliability
// Business Outcome: Test suite framework enables Platform Multicluster Operations validation

func TestUmulticluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Platform Multicluster Operations Suite")
}
