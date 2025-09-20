//go:build integration
// +build integration

package external_services

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExternalServices(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "External Services Integration Suite")
}

var integrationSuite *ExternalServicesIntegrationSuite

var _ = BeforeSuite(func() {
	var err error
	integrationSuite, err = NewExternalServicesIntegrationSuite()
	Expect(err).ToNot(HaveOccurred(), "Failed to initialize External Services Integration Suite")
})

var _ = AfterSuite(func() {
	if integrationSuite != nil {
		integrationSuite.Cleanup()
	}
})
