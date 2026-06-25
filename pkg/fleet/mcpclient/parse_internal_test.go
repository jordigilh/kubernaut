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

package mcpclient

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// UT-FLEET-MCP-PARSE: parseUnstructured / parseUnstructuredList / populateObject
// Authority: BR-FLEET-002 (MCP Gateway client), ADR-068
// FedRAMP: SI-10 (Information Input Validation) -- response parsing correctness
var _ = Describe("UT-FLEET-MCP-PARSE: MCP response parsing", func() {

	Describe("parseUnstructured", func() {
		It("UT-FLEET-MCP-PARSE-001: parses valid JSON into Unstructured", func() {
			obj, err := parseUnstructured(`{"kind":"Pod","metadata":{"name":"nginx","namespace":"default"}}`)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.GetKind()).To(Equal("Pod"))
			Expect(obj.GetName()).To(Equal("nginx"))
			Expect(obj.GetNamespace()).To(Equal("default"))
		})

		It("UT-FLEET-MCP-PARSE-002: returns error for empty string", func() {
			_, err := parseUnstructured("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("empty response"))
		})

		It("UT-FLEET-MCP-PARSE-003: returns error for invalid JSON", func() {
			_, err := parseUnstructured("{not-valid-json}")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unmarshaling resource"))
		})
	})

	Describe("parseUnstructuredList", func() {
		It("UT-FLEET-MCP-PARSE-004: parses K8s-style list with items field", func() {
			items, err := parseUnstructuredList(`{"items":[{"kind":"Pod","metadata":{"name":"a"}},{"kind":"Pod","metadata":{"name":"b"}}]}`)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(2))
			Expect(items[0].GetName()).To(Equal("a"))
			Expect(items[1].GetName()).To(Equal("b"))
		})

		It("UT-FLEET-MCP-PARSE-005: parses raw JSON array", func() {
			items, err := parseUnstructuredList(`[{"kind":"Pod","metadata":{"name":"x"}},{"kind":"Pod","metadata":{"name":"y"}}]`)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(2))
			Expect(items[0].GetName()).To(Equal("x"))
		})

		It("UT-FLEET-MCP-PARSE-006: returns nil for empty string", func() {
			items, err := parseUnstructuredList("")
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(BeNil())
		})

		It("UT-FLEET-MCP-PARSE-007: returns error for completely invalid JSON", func() {
			_, err := parseUnstructuredList("{{{invalid")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unmarshaling list response"))
		})

		It("UT-FLEET-MCP-PARSE-008: wraps single object as one-item list", func() {
			items, err := parseUnstructuredList(`{"kind":"ConfigMap","metadata":{"name":"solo"}}`)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetName()).To(Equal("solo"))
		})
	})

	Describe("parseSelectorToMap", func() {
		It("UT-FLEET-MCP-PARSE-009: parses single key=value", func() {
			m := parseSelectorToMap("app=nginx")
			Expect(m).To(HaveLen(1))
			Expect(m).To(HaveKeyWithValue("app", "nginx"))
		})

		It("UT-FLEET-MCP-PARSE-010: parses multi key=value", func() {
			m := parseSelectorToMap("app=nginx,tier=frontend")
			Expect(m).To(HaveLen(2))
			Expect(m).To(HaveKeyWithValue("app", "nginx"))
			Expect(m).To(HaveKeyWithValue("tier", "frontend"))
		})

		It("UT-FLEET-MCP-PARSE-011: returns nil for empty string", func() {
			m := parseSelectorToMap("")
			Expect(m).To(BeNil())
		})
	})

	Describe("formatLabelSelector", func() {
		It("UT-FLEET-MCP-PARSE-012: formats single label", func() {
			s := formatLabelSelector(map[string]string{"app": "nginx"})
			Expect(s).To(Equal("app=nginx"))
		})
	})

	Describe("ClusterTool", func() {
		It("UT-FLEET-MCP-PARSE-013: prefixes tool with cluster ID", func() {
			Expect(ClusterTool("prod-east", ToolGet)).To(Equal("prod-east__resources_get"))
		})

		It("UT-FLEET-MCP-PARSE-014: prefixes list tool with cluster ID", func() {
			Expect(ClusterTool("staging", ToolList)).To(Equal("staging__resources_list"))
		})
	})

	Describe("ExtractText", func() {
		It("UT-FLEET-MCP-PARSE-015: returns empty for nil result", func() {
			Expect(ExtractText(nil)).To(Equal(""))
		})
	})
})
