//go:build integration
// +build integration

package execution

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUworkflow(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "workflow Suite")
}
