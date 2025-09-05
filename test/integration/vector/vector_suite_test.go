//go:build integration
// +build integration

package vector

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVectorIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Vector Database Integration Suite")
}
