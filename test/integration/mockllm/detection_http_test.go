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
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Scenario Detection over HTTP", func() {

	var server *httptest.Server

	BeforeEach(func() {
		registry := scenarios.DefaultRegistry()
		router := handlers.NewRouter(registry, false)
		server = httptest.NewServer(router)
	})

	AfterEach(func() {
		server.Close()
	})

	DescribeTable("IT-MOCK-026: Scenario detection produces correct RCA content",
		func(content, expectedRCASubstr string) {
			body := chatRequest(content, nil)
			resp := postJSON(server.URL+"/v1/chat/completions", body)
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Choices[0].Message.Content).NotTo(BeNil())
			Expect(*result.Choices[0].Message.Content).To(ContainSubstring(expectedRCASubstr))
		},
		Entry("IT-MOCK-026-001: OOMKilled → memory RCA",
			"- Signal Name: OOMKilled\n- Namespace: default",
			"memory limits"),
		Entry("IT-MOCK-026-002: mock_no_workflow_found → no workflow RCA",
			"mock_no_workflow_found",
			"No suitable workflow"),
		Entry("IT-MOCK-026-003: mock_problem_resolved → self-resolved RCA",
			"mock_problem_resolved",
			"self-resolved"),
		Entry("IT-MOCK-026-004: CrashLoopBackOff → configuration RCA",
			"- Signal Name: CrashLoopBackOff\n- Namespace: staging",
			"missing configuration"),
	)
})
