//go:build unit
// +build unit

package rules

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-RULES-SUITE-001: Workflow Rules Test Suite Organization
// Business Impact: Ensures comprehensive validation of workflow rules business logic
// Stakeholder Value: Provides executive confidence in business rule automation and workflow governance

func TestRules(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Rules Unit Tests Suite")
}
