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
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/fault"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Enhanced Fault Injection", func() {

	var server *httptest.Server

	BeforeEach(func() {
		registry := scenarios.DefaultRegistry()
		router := handlers.NewFullRouter(registry, false, "", nil)
		server = httptest.NewServer(router)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("IT-MOCK-565-001: Fault injection applies to Ollama endpoints", func() {
		It("should return fault error on /api/chat when fault is active", func() {
			cfg := fault.Config{Enabled: true, StatusCode: 502, Message: "bad gateway"}
			postJSON(server.URL+"/api/test/fault", marshalReq(cfg)).Body.Close()

			body := ollamaChatRequest("- Signal Name: OOMKilled")
			resp, err := http.Post(server.URL+"/api/chat", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(502))
		})
	})

	Describe("IT-MOCK-565-002: Fault injection with delay", func() {
		It("should delay the response by configured delay_ms", func() {
			cfg := fault.Config{Enabled: true, StatusCode: 500, Message: "delayed error", DelayMs: 100}
			postJSON(server.URL+"/api/test/fault", marshalReq(cfg)).Body.Close()

			start := time.Now()
			chatBody := chatRequest("- Signal Name: OOMKilled", nil)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", chatBody)
			elapsed := time.Since(start)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(500))
			Expect(elapsed).To(BeNumerically(">=", 80*time.Millisecond))
		})
	})

	Describe("IT-MOCK-565-003: Intermittent fault with count", func() {
		It("should fail for N requests then resume normal operation", func() {
			cfg := fault.Config{Enabled: true, StatusCode: 503, Message: "intermittent", Count: 2}
			postJSON(server.URL+"/api/test/fault", marshalReq(cfg)).Body.Close()

			// First 2 requests should fail
			for i := 0; i < 2; i++ {
				chatBody := chatRequest("- Signal Name: OOMKilled", nil)
				resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", chatBody)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(503))
			}

			// Third request should succeed
			chatBody := chatRequest("- Signal Name: OOMKilled", nil)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", chatBody)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))
		})
	})
})
