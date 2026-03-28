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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Server Config Integration", func() {

	Describe("IT-MOCK-032-001: Server respects MOCK_LLM_FORCE_TEXT env var", func() {
		It("should force text mode when MOCK_LLM_FORCE_TEXT=true is loaded from config", func() {
			os.Setenv("MOCK_LLM_FORCE_TEXT", "true")
			defer os.Unsetenv("MOCK_LLM_FORCE_TEXT")

			cfg := config.LoadFromEnv()
			Expect(cfg.ForceText).To(BeTrue())

			registry := scenarios.DefaultRegistry()
			router := handlers.NewRouter(registry, cfg.ForceText)
			server := httptest.NewServer(router)
			defer server.Close()

			body := chatRequest("- Signal Name: OOMKilled", []string{"search_workflow_catalog"})
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			// Should return text, not tool call
			var raw map[string]interface{}
			Expect(decodeJSON(resp.Body, &raw)).To(Succeed())
			choices := raw["choices"].([]interface{})
			choice := choices[0].(map[string]interface{})
			Expect(choice["finish_reason"]).To(Equal("stop"))
		})
	})
})

func decodeJSON(r interface{ Read([]byte) (int, error) }, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}
