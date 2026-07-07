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
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("buildDSBaseTransport", func() {
	It("returns a custom non-default transport when a valid CA file is set", func() {
		caPath := generateTestCACert(GinkgoTB(), "DS Test CA")

		transport, err := buildDSBaseTransport(caPath, types.LLMCircuitBreaker{})
		Expect(err).NotTo(HaveOccurred())
		Expect(transport).NotTo(BeNil())
		Expect(transport).NotTo(BeIdenticalTo(http.DefaultTransport))
	})

	It("returns a non-nil default-fallback transport when caFile is empty", func() {
		transport, err := buildDSBaseTransport("", types.LLMCircuitBreaker{})
		Expect(err).NotTo(HaveOccurred())
		Expect(transport).NotTo(BeNil())
	})

	It("returns an error for an invalid CA file path", func() {
		_, err := buildDSBaseTransport("/nonexistent/ca.crt", types.LLMCircuitBreaker{})
		Expect(err).To(HaveOccurred())
	})
})
