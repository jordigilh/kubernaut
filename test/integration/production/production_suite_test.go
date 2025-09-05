//go:build integration
// +build integration

package production

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProductionIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Production Integration Suite")
}
