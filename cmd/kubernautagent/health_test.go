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

package main

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// AA IT shared-envtest fix (DD-TEST-010 amendment): detectNamespace() must
// support an explicit env var override so multiple per-process KA instances
// sharing a single envtest apiserver can each be scoped to their own
// process-unique namespace, instead of every process falling back to the
// same hardcoded "kubernaut-system" default.
var _ = Describe("detectNamespace", Label("unit", "kubernautagent", "namespace"), func() {
	const envVar = "KUBERNAUT_AGENT_NAMESPACE"

	var savedEnv string
	var savedOK bool

	BeforeEach(func() {
		savedEnv, savedOK = os.LookupEnv(envVar)
		Expect(os.Unsetenv(envVar)).To(Succeed())
	})

	AfterEach(func() {
		if savedOK {
			Expect(os.Setenv(envVar, savedEnv)).To(Succeed())
		} else {
			Expect(os.Unsetenv(envVar)).To(Succeed())
		}
	})

	It("UT-KA-2213-001: returns the env var value when KUBERNAUT_AGENT_NAMESPACE is set", func() {
		Expect(os.Setenv(envVar, "kubernaut-system-p3")).To(Succeed())

		Expect(detectNamespace()).To(Equal("kubernaut-system-p3"),
			"env var override must win over the SA-file/fallback lookup so parallel test processes can each claim a distinct namespace")
	})

	It("UT-KA-2213-002: falls back to existing behavior when KUBERNAUT_AGENT_NAMESPACE is unset", func() {
		Expect(os.Unsetenv(envVar)).To(Succeed())

		Expect(detectNamespace()).To(Equal("kubernaut-system"),
			"unset env var must preserve current production behavior exactly (no mounted SA namespace file in this test binary, so the hardcoded fallback applies)")
	})

	It("UT-KA-2213-003: falls back to existing behavior when KUBERNAUT_AGENT_NAMESPACE is set to empty string", func() {
		Expect(os.Setenv(envVar, "")).To(Succeed())

		Expect(detectNamespace()).To(Equal("kubernaut-system"),
			"an explicitly empty env var must not be treated as a valid override")
	})
})
