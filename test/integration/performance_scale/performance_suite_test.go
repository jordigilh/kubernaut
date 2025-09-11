//go:build integration
// +build integration

package performance_scale

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPerformanceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Performance Integration Suite")
}
