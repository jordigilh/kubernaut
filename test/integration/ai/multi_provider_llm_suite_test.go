//go:build integration
// +build integration

package ai

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestMultiProviderLLMProduction runs the Phase 2 Multi-Provider LLM Integration test suite
func TestMultiProviderLLMProduction(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Phase 2: Multi-Provider LLM Integration Suite")
}
