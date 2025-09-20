//go:build integration
// +build integration

package ai_pgvector

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAIPgVectorIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI + pgvector Integration Test Suite")
}
