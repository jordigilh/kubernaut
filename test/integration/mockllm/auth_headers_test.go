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
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Auth Header Endpoints", func() {

	var server *httptest.Server

	BeforeEach(func() {
		registry := scenarios.DefaultRegistry()
		router := handlers.NewFullRouter(registry, false, "authorization,x-kubernaut-token", nil)
		server = httptest.NewServer(router)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("IT-MOCK-006-001: Headers recorded from chat requests", func() {
		It("should capture Authorization and X-Kubernaut-Token headers", func() {
			body := chatRequest("- Signal Name: OOMKilled", nil)
			req, err := http.NewRequest("POST", server.URL+"/v1/chat/completions", body)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-jwt-token")
			req.Header.Set("X-Kubernaut-Token", "kube-auth-abc123")

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			// Query headers API
			hresp, err := http.Get(server.URL + "/api/test/headers")
			Expect(err).NotTo(HaveOccurred())
			defer hresp.Body.Close()

			var result map[string]interface{}
			Expect(json.NewDecoder(hresp.Body).Decode(&result)).To(Succeed())
			headers := result["headers"].(map[string]interface{})
			Expect(headers["Authorization"]).To(Equal("Bearer test-jwt-token"))
			Expect(headers["X-Kubernaut-Token"]).To(Equal("kube-auth-abc123"))
		})
	})

	Describe("IT-MOCK-006-002: Headers cleared after reset", func() {
		It("should clear headers after POST /api/test/reset", func() {
			body := chatRequest("test", nil)
			req, err := http.NewRequest("POST", server.URL+"/v1/chat/completions", body)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer old-token")
			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			// Reset
			postJSON(server.URL+"/api/test/reset", marshalReq(nil)).Body.Close()

			// Headers should be empty
			hresp, err := http.Get(server.URL + "/api/test/headers")
			Expect(err).NotTo(HaveOccurred())
			defer hresp.Body.Close()
			var result map[string]interface{}
			Expect(json.NewDecoder(hresp.Body).Decode(&result)).To(Succeed())
			headers := result["headers"].(map[string]interface{})
			Expect(headers).To(BeEmpty())
		})
	})

	Describe("IT-MOCK-007-001: Headers not recorded when not configured", func() {
		It("should return empty headers when record_headers is empty", func() {
			registry := scenarios.DefaultRegistry()
			router := handlers.NewFullRouter(registry, false, "", nil)
			noHeaderServer := httptest.NewServer(router)
			defer noHeaderServer.Close()

			body := chatRequest("test", nil)
			req, err := http.NewRequest("POST", noHeaderServer.URL+"/v1/chat/completions", body)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer hidden")
			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			hresp, err := http.Get(noHeaderServer.URL + "/api/test/headers")
			Expect(err).NotTo(HaveOccurred())
			defer hresp.Body.Close()
			var result map[string]interface{}
			Expect(json.NewDecoder(hresp.Body).Decode(&result)).To(Succeed())
			headers := result["headers"].(map[string]interface{})
			Expect(headers).To(BeEmpty())
		})
	})

	Describe("IT-MOCK-007-002: Only configured headers are recorded", func() {
		It("should not record unconfigured headers", func() {
			body := chatRequest("test", nil)
			req, err := http.NewRequest("POST", server.URL+"/v1/chat/completions", body)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer tok")
			req.Header.Set("X-Untracked-Header", "should-not-appear")
			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			hresp, err := http.Get(server.URL + "/api/test/headers")
			Expect(err).NotTo(HaveOccurred())
			defer hresp.Body.Close()
			var result map[string]interface{}
			Expect(json.NewDecoder(hresp.Body).Decode(&result)).To(Succeed())
			headers := result["headers"].(map[string]interface{})
			Expect(headers).To(HaveKey("Authorization"))
			Expect(headers).NotTo(HaveKey("X-Untracked-Header"))
		})
	})

	Describe("IT-MOCK-007-003: Last header value wins (multiple requests)", func() {
		It("should overwrite with latest header value", func() {
			for _, token := range []string{"old-token", "new-token"} {
				body := chatRequest("test", nil)
				req, err := http.NewRequest("POST", server.URL+"/v1/chat/completions", body)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+token)
				client := &http.Client{}
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()
			}

			hresp, err := http.Get(server.URL + "/api/test/headers")
			Expect(err).NotTo(HaveOccurred())
			defer hresp.Body.Close()
			var result map[string]interface{}
			Expect(json.NewDecoder(hresp.Body).Decode(&result)).To(Succeed())
			headers := result["headers"].(map[string]interface{})
			Expect(headers["Authorization"]).To(Equal("Bearer new-token"))
		})
	})
})
