package orchestration_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOrchestration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Orchestration Suite")
}

var _ = BeforeSuite(func() {
	// Global test setup
	By("Setting up test environment for Orchestration suite")
})

var _ = AfterSuite(func() {
	// Global test cleanup
	By("Cleaning up test environment for Orchestration suite")
})
