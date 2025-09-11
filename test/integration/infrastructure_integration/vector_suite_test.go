//go:build integration
// +build integration

package infrastructure_integration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVectorIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Vector Database Integration Suite")
}
