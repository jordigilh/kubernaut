//go:build integration
// +build integration

package api_database

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAPIDatabaseIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API + Database Integration Suite")
}

var integrationSuite *APIDatabaseIntegrationSuite

var _ = BeforeSuite(func() {
	var err error
	integrationSuite, err = NewAPIDatabaseIntegrationSuite()
	Expect(err).ToNot(HaveOccurred(), "Failed to initialize API + Database Integration Suite")
})

var _ = AfterSuite(func() {
	if integrationSuite != nil {
		integrationSuite.Cleanup()
	}
})
