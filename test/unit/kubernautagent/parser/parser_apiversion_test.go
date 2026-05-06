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

package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
)

var _ = Describe("Parser apiVersion Extraction — Issue #1040", func() {
	var rp *parser.ResultParser

	BeforeEach(func() {
		rp = parser.NewResultParser()
	})

	Describe("UT-KA-1040-001: Parser extracts api_version from LLM JSON", func() {
		It("should capture api_version in RemediationTarget when present in nested RCA", func() {
			jsonStr := `{
				"root_cause_analysis": {
					"summary": "Route misconfigured due to wrong backend service",
					"severity": "high",
					"remediation_target": {
						"kind": "Route",
						"name": "storefront",
						"namespace": "demo-route",
						"api_version": "route.openshift.io/v1"
					}
				},
				"confidence": 0.92
			}`

			result, err := rp.Parse(jsonStr)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.RemediationTarget.Kind).To(Equal("Route"))
			Expect(result.RemediationTarget.Name).To(Equal("storefront"))
			Expect(result.RemediationTarget.Namespace).To(Equal("demo-route"))
			Expect(result.RemediationTarget.APIVersion).To(Equal("route.openshift.io/v1"),
				"UT-KA-1040-001: api_version must be captured from LLM JSON")
		})

		It("should capture api_version from flat remediation_target path", func() {
			jsonStr := `{
				"rca_summary": "Route misconfigured",
				"remediation_target": {
					"kind": "Route",
					"name": "storefront",
					"namespace": "demo-route",
					"api_version": "route.openshift.io/v1"
				},
				"confidence": 0.92
			}`

			result, err := rp.Parse(jsonStr)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.RemediationTarget.APIVersion).To(Equal("route.openshift.io/v1"),
				"UT-KA-1040-001: api_version must be captured from flat path")
		})
	})

	Describe("UT-KA-1040-002: Parser handles missing api_version (backwards compat)", func() {
		It("should leave APIVersion empty when not present in LLM JSON", func() {
			jsonStr := `{
				"root_cause_analysis": {
					"summary": "OOMKilled due to memory limit",
					"remediation_target": {
						"kind": "Deployment",
						"name": "api-server",
						"namespace": "production"
					}
				},
				"confidence": 0.85
			}`

			result, err := rp.Parse(jsonStr)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))
			Expect(result.RemediationTarget.APIVersion).To(BeEmpty(),
				"UT-KA-1040-002: APIVersion must be empty when not provided by LLM")
		})
	})
})
