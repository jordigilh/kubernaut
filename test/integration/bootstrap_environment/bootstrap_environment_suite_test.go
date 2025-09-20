//go:build integration
// +build integration

package bootstrap_environment

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBootstrapEnvironmentIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bootstrap Environment Integration Test Suite")
}
