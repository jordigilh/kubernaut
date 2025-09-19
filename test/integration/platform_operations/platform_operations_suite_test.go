//go:build integration
// +build integration

package platform_operations

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestPlatformOperations runs the Phase 1 Platform Operations Concurrent Execution integration test suite
func TestPlatformOperations(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Phase 1: Platform Operations Concurrent Execution Suite")
}