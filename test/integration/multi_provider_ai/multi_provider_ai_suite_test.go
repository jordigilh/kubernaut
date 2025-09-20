//go:build integration
// +build integration

package multi_provider_ai

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMultiProviderAI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Multi-Provider AI Integration Suite")
}

var integrationSuite *MultiProviderAIIntegrationSuite

var _ = BeforeSuite(func() {
	var err error
	integrationSuite, err = NewMultiProviderAIIntegrationSuite()
	Expect(err).ToNot(HaveOccurred(), "Failed to initialize Multi-Provider AI Integration Suite")
})

var _ = AfterSuite(func() {
	if integrationSuite != nil {
		integrationSuite.Cleanup()
	}
})
