<<<<<<< HEAD
=======
/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

>>>>>>> crd_implementation
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
