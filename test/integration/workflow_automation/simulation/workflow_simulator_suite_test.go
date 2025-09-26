//go:build integration
// +build integration

package simulation

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUworkflowUsimulator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "workflow simulator Suite")
}
