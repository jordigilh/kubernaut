//go:build integration
// +build integration

package end_to_end

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestE2EIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "End-to-End Integration Suite")
}
