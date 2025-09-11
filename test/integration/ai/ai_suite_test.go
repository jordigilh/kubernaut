//go:build integration
// +build integration

package ai

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAIIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI Integration & Performance Test Suite")
}
