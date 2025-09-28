//go:build unit
// +build unit

package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-AI-SERVICE-SUITE-001: AI Service Microservice Test Suite Organization
// Business Impact: Ensures comprehensive validation of AI service business logic
// Stakeholder Value: Provides executive confidence in AI service capabilities and business continuity
//
// Business Scenario: Executive stakeholders need confidence in AI service capabilities
// Business Impact: Ensures all AI service components deliver measurable business value
// Business Outcome: Test suite framework enables AI service business requirement validation

func TestAIService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI Service Microservice Test Suite")
}
