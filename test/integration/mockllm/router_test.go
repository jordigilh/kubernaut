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
package mockllm_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Router Integration", func() {

	var server *httptest.Server

	BeforeEach(func() {
		registry := scenarios.DefaultRegistry()
		router := handlers.NewRouter(registry, false)
		server = httptest.NewServer(router)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("IT-MOCK-003-001: Health endpoint returns 200 with status ok", func() {
		It("should return 200 with JSON status ok", func() {
			resp, err := http.Get(server.URL + "/health")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			var body map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["status"]).To(Equal("ok"))
		})
	})

	Describe("IT-MOCK-003-002: Models endpoint returns model list", func() {
		It("should return models list at /v1/models", func() {
			resp, err := http.Get(server.URL + "/v1/models")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			var body map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			models := body["models"].([]interface{})
			Expect(models).To(HaveLen(1))
		})

		It("should return models list at /api/tags (Ollama compat)", func() {
			resp, err := http.Get(server.URL + "/api/tags")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))
		})
	})

	Describe("IT-MOCK-005-001: Strict routing returns 404 for unknown paths", func() {
		It("should return 404 for GET /unknown/path", func() {
			resp, err := http.Get(server.URL + "/unknown/path")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(404))
			body, _ := io.ReadAll(resp.Body)
			Expect(string(body)).To(ContainSubstring("not found"))
		})
	})
})
