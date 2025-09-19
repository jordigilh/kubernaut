//go:build integration
// +build integration

package kubernetes_operations

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestKubernetesOperations runs the Phase 1 Kubernetes Operations Safety integration test suite
func TestKubernetesOperations(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Phase 1: Kubernetes Operations Safety Suite")
}