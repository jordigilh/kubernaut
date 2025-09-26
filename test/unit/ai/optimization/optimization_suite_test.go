//go:build unit
// +build unit

package optimization

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-AI-OPTIMIZATION-SUITE-001: AI Optimization Test Suite Organization
// Business Impact: Ensures proper test suite organization for AI optimization algorithms
// Stakeholder Value: Provides executive confidence in test organization and maintainability

func TestOptimization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI Optimization Unit Tests Suite")
}
