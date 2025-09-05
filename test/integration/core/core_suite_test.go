//go:build integration
// +build integration

package integration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCoreIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Core Integration Suite")
}
