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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
)

var _ = Describe("StructuredOutputTransport — BR-TESTING-001", func() {

	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"rca_summary": {"type": "string"},
			"workflow_id": {"type": "string"}
		},
		"required": ["rca_summary"]
	}`)

	Describe("UT-KA-SO-001: Injects output_config.format into Anthropic Messages API request", func() {
		It("should add output_config with json_schema when body has messages field", func() {
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

			t := transport.NewStructuredOutputTransport(schema, http.DefaultTransport)
			client := &http.Client{Transport: t}

			reqBody := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"test"}],"max_tokens":1024}`
			req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/messages", strings.NewReader(reqBody))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(capturedBody).To(HaveKey("output_config"))
			outputConfig, ok := capturedBody["output_config"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "output_config must be a map")
			Expect(outputConfig).To(HaveKey("format"))
			format, ok := outputConfig["format"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "format must be a map")
			Expect(format["type"]).To(Equal("json_schema"))
			Expect(format).To(HaveKey("schema"))
		})
	})

	Describe("UT-KA-SO-002: Preserves original request fields", func() {
		It("should not remove or alter existing fields like model, messages, max_tokens", func() {
			var capturedBody map[string]interface{}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &capturedBody)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			t := transport.NewStructuredOutputTransport(schema, http.DefaultTransport)
			client := &http.Client{Transport: t}

			reqBody := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"test"}],"max_tokens":1024,"temperature":0.7}`
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/messages", strings.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(capturedBody["model"]).To(Equal("claude-sonnet-4-20250514"))
			Expect(capturedBody["max_tokens"]).To(BeNumerically("==", 1024))
			Expect(capturedBody["temperature"]).To(BeNumerically("~", 0.7, 0.01))
			Expect(capturedBody).To(HaveKey("messages"))
		})
	})

	Describe("UT-KA-SO-003: Passes non-Anthropic requests through unmodified", func() {
		It("should not inject output_config when body has no messages field (non-Anthropic format)", func() {
			var capturedBody map[string]interface{}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &capturedBody)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			t := transport.NewStructuredOutputTransport(schema, http.DefaultTransport)
			client := &http.Client{Transport: t}

			reqBody := `{"prompt":"test","max_tokens":100}`
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/complete", strings.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(capturedBody).NotTo(HaveKey("output_config"),
				"non-Messages API requests must pass through unmodified")
		})
	})

	Describe("UT-KA-SO-005: Passes GET requests through unmodified", func() {
		It("should not modify non-POST requests", func() {
			var receivedMethod string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			t := transport.NewStructuredOutputTransport(schema, http.DefaultTransport)
			client := &http.Client{Transport: t}

			req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/models", nil)
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(receivedMethod).To(Equal(http.MethodGet))
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Describe("UT-KA-SO-006: Passes through requests with nil body", func() {
		It("should not panic or error on nil body POST", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			t := transport.NewStructuredOutputTransport(schema, http.DefaultTransport)
			client := &http.Client{Transport: t}

			req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/messages", nil)
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Describe("UT-KA-SO-004: Does not overwrite existing output_config", func() {
		It("should preserve caller-supplied output_config if already present", func() {
			var capturedBody map[string]interface{}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &capturedBody)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			t := transport.NewStructuredOutputTransport(schema, http.DefaultTransport)
			client := &http.Client{Transport: t}

			reqBody := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"test"}],"max_tokens":1024,"output_config":{"format":{"type":"text"}}}`
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/messages", strings.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			outputConfig := capturedBody["output_config"].(map[string]interface{})
			format := outputConfig["format"].(map[string]interface{})
			Expect(format["type"]).To(Equal("text"),
				"existing output_config must not be overwritten")
		})
	})
})
