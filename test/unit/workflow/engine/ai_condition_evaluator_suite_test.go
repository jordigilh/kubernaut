package engine

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-AI-CONDITION-EVALUATOR-SUITE-001: AI Condition Evaluator Unit Test Suite Organization
// Business Impact: Ensures comprehensive validation of AI condition evaluation business logic
// Stakeholder Value: Operations teams can trust AI-driven workflow condition evaluation

func TestAIConditionEvaluator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI Condition Evaluator Unit Tests Suite")
}
