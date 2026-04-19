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

package transport_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
)

var _ = Describe("Per-Session StructuredOutputTransport — #700", func() {

	rcaSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"root_cause_analysis": {"type": "object"},
			"confidence": {"type": "number"}
		},
		"required": ["root_cause_analysis", "confidence"]
	}`)

	fullSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"root_cause_analysis": {"type": "object"},
			"selected_workflow": {"type": "object"},
			"confidence": {"type": "number"}
		},
		"required": ["root_cause_analysis", "confidence"]
	}`)

	Describe("UT-KA-700-011: Transport uses schema from context", func() {
		It("should inject output_config.format.schema from context-provided schema", func() {
			var capturedBody map[string]interface{}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				Expect(err).NotTo(HaveOccurred())
				err = json.Unmarshal(body, &capturedBody)
				Expect(err).NotTo(HaveOccurred())
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"content": [{"type": "text", "text": "{}"}]}`))
			}))
			defer server.Close()

			t := transport.NewStructuredOutputTransport(nil, http.DefaultTransport)
			client := &http.Client{Transport: t}

			reqBody := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"test"}],"max_tokens":1024}`
			ctx := transport.WithOutputSchema(context.Background(), rcaSchema)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1/messages", strings.NewReader(reqBody))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			By("verifying output_config is injected with the context schema")
			Expect(capturedBody).To(HaveKey("output_config"))
			outputConfig, ok := capturedBody["output_config"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "output_config must be a map")
			format, ok := outputConfig["format"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "format must be a map")
			Expect(format["type"]).To(Equal("json_schema"))

			schemaMap, ok := format["schema"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "schema must be a map")
			props, ok := schemaMap["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "schema must have properties")
			Expect(props).To(HaveKey("root_cause_analysis"))
			Expect(props).To(HaveKey("confidence"))
		})

		It("should use different schemas for different requests", func() {
			var bodies []map[string]interface{}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]interface{}
				raw, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(raw, &body)
				bodies = append(bodies, body)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			t := transport.NewStructuredOutputTransport(nil, http.DefaultTransport)
			client := &http.Client{Transport: t}

			By("sending request with RCA schema")
			reqBody := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"rca"}],"max_tokens":1024}`
			ctx1 := transport.WithOutputSchema(context.Background(), rcaSchema)
			req1, _ := http.NewRequestWithContext(ctx1, http.MethodPost, server.URL+"/v1/messages", strings.NewReader(reqBody))
			req1.Header.Set("Content-Type", "application/json")
			resp1, err := client.Do(req1)
			Expect(err).NotTo(HaveOccurred())
			resp1.Body.Close()

			By("sending request with full schema")
			reqBody2 := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"workflow"}],"max_tokens":1024}`
			ctx2 := transport.WithOutputSchema(context.Background(), fullSchema)
			req2, _ := http.NewRequestWithContext(ctx2, http.MethodPost, server.URL+"/v1/messages", strings.NewReader(reqBody2))
			req2.Header.Set("Content-Type", "application/json")
			resp2, err := client.Do(req2)
			Expect(err).NotTo(HaveOccurred())
			resp2.Body.Close()

			By("verifying different schemas were injected")
			Expect(bodies).To(HaveLen(2))

			oc1 := bodies[0]["output_config"].(map[string]interface{})
			f1 := oc1["format"].(map[string]interface{})
			s1 := f1["schema"].(map[string]interface{})
			p1 := s1["properties"].(map[string]interface{})
			Expect(p1).NotTo(HaveKey("selected_workflow"),
				"RCA request schema should NOT have selected_workflow")

			oc2 := bodies[1]["output_config"].(map[string]interface{})
			f2 := oc2["format"].(map[string]interface{})
			s2 := f2["schema"].(map[string]interface{})
			p2 := s2["properties"].(map[string]interface{})
			Expect(p2).To(HaveKey("selected_workflow"),
				"workflow request schema SHOULD have selected_workflow")
		})
	})

	Describe("UT-KA-700-012: Transport skips injection when no schema in context", func() {
		It("should pass through request unmodified when no OutputSchema in context", func() {
			var capturedBody map[string]interface{}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &capturedBody)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			t := transport.NewStructuredOutputTransport(nil, http.DefaultTransport)
			client := &http.Client{Transport: t}

			reqBody := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"test"}],"max_tokens":1024}`
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/messages", strings.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(capturedBody).NotTo(HaveKey("output_config"),
				"no output_config should be injected when no schema is in context")
		})
	})

	Describe("UT-KA-700-013: Context functions round-trip correctly", func() {
		It("should store and retrieve OutputSchema via context", func() {
			ctx := transport.WithOutputSchema(context.Background(), rcaSchema)
			retrieved := transport.OutputSchemaFromContext(ctx)
			Expect(retrieved).NotTo(BeNil())
			Expect(json.RawMessage(retrieved)).To(MatchJSON(rcaSchema))
		})

		It("should return nil from empty context", func() {
			retrieved := transport.OutputSchemaFromContext(context.Background())
			Expect(retrieved).To(BeNil())
		})
	})
})
