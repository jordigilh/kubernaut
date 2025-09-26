//go:build e2e
// +build e2e

package workflowengine

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWorkflowEngineE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Engine E2E Suite")
}
