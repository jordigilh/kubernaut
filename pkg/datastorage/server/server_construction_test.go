/*
Copyright 2026 Jordi Gil.

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

// White-box unit test (package server, not server_test) because
// validateServerDeps is unexported. Go convention places such tests in the
// same package; the project's test/unit/ convention applies to black-box
// (external) tests only.
package server

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// validServerDepsForValidation returns a ServerDeps satisfying every
// DD-AUTH-014 check so each test below can isolate a single missing field.
func validServerDepsForValidation() ServerDeps {
	return ServerDeps{
		Authenticator: &auth.MockAuthenticator{},
		Authorizer:    &auth.MockAuthorizer{},
		AuthNamespace: "datastorage-test",
	}
}

var _ = Describe("Issue #1661 Phase 55: validateServerDeps", func() {
	Context("K8sRestConfig is mandatory (DD-WORKFLOW-018: etcd is the sole source of truth)", func() {
		It("UT-DS-1661-P55-001: rejects ServerDeps with a nil K8sRestConfig", func() {
			deps := validServerDepsForValidation()
			deps.K8sRestConfig = nil

			err := validateServerDeps(deps)

			Expect(err).To(HaveOccurred(),
				"NewServer must fail hard without a K8s rest.Config -- the workflow cache "+
					"is the only source of workflow/action-type data now that Postgres's "+
					"catalog tables are gone")
			Expect(err.Error()).To(ContainSubstring("k8sRestConfig"))
		})

		It("UT-DS-1661-P55-002: accepts ServerDeps with a non-nil K8sRestConfig", func() {
			deps := validServerDepsForValidation()
			deps.K8sRestConfig = &rest.Config{}

			Expect(validateServerDeps(deps)).To(Succeed())
		})
	})
})
