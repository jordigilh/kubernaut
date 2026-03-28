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

	"github.com/jordigilh/kubernaut/test/services/mock-llm/fault"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Fault Injection API", func() {

	var server *httptest.Server

	BeforeEach(func() {
		registry := scenarios.DefaultRegistry()
		router := handlers.NewFullRouter(registry, false, "", nil)
		server = httptest.NewServer(router)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("IT-MOCK-050-001: POST /api/test/fault configures fault injection", func() {
		It("should activate fault injection and return configured status code", func() {
			cfg := fault.Config{Enabled: true, StatusCode: 503, Message: "service unavailable"}
			body := marshalReq(cfg)
			resp := postJSON(server.URL+"/api/test/fault", body)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			chatBody := chatRequest("- Signal Name: OOMKilled", nil)
			chatResp := postJSON(server.URL+"/v1/chat/completions", chatBody)
			defer chatResp.Body.Close()
			Expect(chatResp.StatusCode).To(Equal(503))
		})
	})

	Describe("IT-MOCK-050-002: GET /api/test/fault/status returns current fault config", func() {
		It("should return the configured fault state", func() {
			cfg := fault.Config{Enabled: true, StatusCode: 429, Message: "rate limited"}
			postJSON(server.URL+"/api/test/fault", marshalReq(cfg)).Body.Close()

			resp, err := http.Get(server.URL + "/api/test/fault/status")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			var result fault.Config
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Enabled).To(BeTrue())
			Expect(result.StatusCode).To(Equal(429))
		})
	})

	Describe("IT-MOCK-054-001: POST /api/test/fault/reset disables fault injection", func() {
		It("should restore normal operation after reset", func() {
			cfg := fault.Config{Enabled: true, StatusCode: 500, Message: "boom"}
			postJSON(server.URL+"/api/test/fault", marshalReq(cfg)).Body.Close()
			postJSON(server.URL+"/api/test/fault/reset", marshalReq(nil)).Body.Close()

			chatBody := chatRequest("- Signal Name: OOMKilled", nil)
			chatResp := postJSON(server.URL+"/v1/chat/completions", chatBody)
			defer chatResp.Body.Close()
			Expect(chatResp.StatusCode).To(Equal(200))
		})
	})
})
