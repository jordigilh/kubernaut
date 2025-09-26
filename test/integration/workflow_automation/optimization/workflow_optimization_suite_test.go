//go:build integration
// +build integration

package optimization

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUworkflowUoptimization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "workflow optimization Suite")
}
