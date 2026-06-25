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
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MCP Response Parsing (BR-INTEGRATION-054)", func() {
	Describe("UT-FLEET-PARSE-001 [SI-10]: MCP response parser handles all K8s MCP Server response formats", func() {
		Context("parseUnstructured", func() {
			It("returns error for empty text", func() {
				obj, err := parseUnstructured("")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("empty response"))
				Expect(obj).To(BeNil())
			})

			It("parses valid JSON into Unstructured object", func() {
				input := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"nginx","namespace":"default"}}`
				obj, err := parseUnstructured(input)
				Expect(err).ToNot(HaveOccurred())
				Expect(obj.GetKind()).To(Equal("Pod"))
				Expect(obj.GetName()).To(Equal("nginx"))
				Expect(obj.GetNamespace()).To(Equal("default"))
			})

			It("returns error for invalid JSON", func() {
				obj, err := parseUnstructured("not-json{{{")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unmarshaling resource"))
				Expect(obj).To(BeNil())
			})
		})

		Context("parseUnstructuredList", func() {
			It("returns nil,nil for empty text", func() {
				items, err := parseUnstructuredList("")
				Expect(err).ToNot(HaveOccurred())
				Expect(items).To(BeNil())
			})

			It("parses map with items key into list of Unstructured", func() {
				input := `{"items":[{"kind":"Pod","metadata":{"name":"pod-1"}},{"kind":"Pod","metadata":{"name":"pod-2"}}]}`
				items, err := parseUnstructuredList(input)
				Expect(err).ToNot(HaveOccurred())
				Expect(items).To(HaveLen(2))
				Expect(items[0].GetName()).To(Equal("pod-1"))
				Expect(items[1].GetName()).To(Equal("pod-2"))
			})

			It("parses raw JSON array into list of Unstructured", func() {
				input := `[{"kind":"Pod","metadata":{"name":"arr-1"}},{"kind":"Pod","metadata":{"name":"arr-2"}}]`
				items, err := parseUnstructuredList(input)
				Expect(err).ToNot(HaveOccurred())
				Expect(items).To(HaveLen(2))
				Expect(items[0].GetName()).To(Equal("arr-1"))
				Expect(items[1].GetName()).To(Equal("arr-2"))
			})

			It("falls back to single-object wrapping when map has no items key", func() {
				input := `{"kind":"Namespace","metadata":{"name":"default"}}`
				items, err := parseUnstructuredList(input)
				Expect(err).ToNot(HaveOccurred())
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetName()).To(Equal("default"))
			})

			It("returns error for invalid JSON", func() {
				items, err := parseUnstructuredList("invalid{json")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unmarshaling list response"))
				Expect(items).To(BeNil())
			})
		})
	})

	Describe("UT-FLEET-PARSE-002 [SI-10]: ExtractText handles all content types", func() {
		It("returns empty string for nil result", func() {
			Expect(ExtractText(nil)).To(BeEmpty())
		})

		It("returns empty string for result with empty content", func() {
			result := &mcp.CallToolResult{Content: []mcp.Content{}}
			Expect(ExtractText(result)).To(BeEmpty())
		})

		It("extracts text from single TextContent", func() {
			result := &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: `{"kind":"Pod"}`},
				},
			}
			Expect(ExtractText(result)).To(Equal(`{"kind":"Pod"}`))
		})

		It("joins multiple TextContent with newlines", func() {
			result := &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "line1"},
					&mcp.TextContent{Text: "line2"},
				},
			}
			Expect(ExtractText(result)).To(Equal("line1\nline2"))
		})

		It("falls back to JSON serialization for non-text content", func() {
			result := &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.ImageContent{
						MIMEType: "image/png",
						Data:     []byte("base64data"),
					},
				},
			}
			text := ExtractText(result)
			Expect(text).ToNot(BeEmpty())
			var parsed []any
			Expect(json.Unmarshal([]byte(text), &parsed)).To(Succeed())
		})
	})
})
