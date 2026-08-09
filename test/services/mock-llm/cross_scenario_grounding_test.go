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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

// UT-ML-2023-101/102: regression for a cross-scenario interaction found in
// CI run 31325053872 (PR #2026, E2E-AF-1395-001). #2023's grounding guard
// requires AF E2E tests to run an unrelated "grounding" tool call
// (kubernaut_investigate) earlier in the SAME A2A session before the
// scenario under test's first turn. hasFunctionResults in handleGemini is
// computed over the WHOLE request's Contents, not scoped to the
// currently-matched scenario -- so a later, different scenario's first-ever
// match in that session sees hasFunctionResults=true (from the unrelated
// grounding call) and, without repeat_tool_call, falls straight to a
// text-only response instead of firing its own tool call.
var _ = Describe("Cross-Scenario Grounding + hasFunctionResults Interaction (issue #2023 E2E regression)", func() {

	// buildTwoScenarioOverrides models structured_decision_e2e_test.go's
	// groundSession pattern: one keyword scenario ("af_ground") whose tool
	// call's FunctionResponse is already in history, and a second, different
	// keyword scenario ("af_decision") being matched for the first time in
	// that same conversation.
	buildTwoScenarioOverrides := func(decisionRepeat bool) *config.Overrides {
		return &config.Overrides{
			Scenarios: map[string]config.ScenarioOverride{},
			KeywordScenarios: []config.KeywordScenarioOverride{
				{
					Name:          "af_ground",
					Keywords:      []string{"seed grounding context"},
					MatchLastOnly: true,
					ToolCall:      config.ToolCallOverride{Name: "kubernaut_investigate"},
				},
				{
					Name:           "af_decision",
					Keywords:       []string{"present structured rca decision"},
					MatchLastOnly:  true,
					RepeatToolCall: decisionRepeat,
					ToolCall:       config.ToolCallOverride{Name: "kubernaut_present_decision"},
				},
			},
		}
	}

	// groundedDecisionRequest builds a Gemini request whose history already
	// contains af_ground's FunctionResponse (from an earlier, unrelated
	// turn), with the current last user message matching af_decision for
	// the first time.
	groundedDecisionRequest := func() response.GeminiRequest {
		return response.GeminiRequest{
			Contents: []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{Text: "seed grounding context"}}},
				{Role: "model", Parts: []response.GeminiPart{{FunctionCall: &response.GeminiFunctionCall{Name: "kubernaut_investigate"}}}},
				{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{Name: "kubernaut_investigate", Response: map[string]interface{}{"session_id": "sess-ground"}}}}},
				{Role: "user", Parts: []response.GeminiPart{{Text: "present structured rca decision"}}},
			},
			Tools: []response.GeminiToolDecl{
				{FunctionDeclarations: []response.GeminiFunctionDecl{{Name: "kubernaut_present_decision"}}},
			},
		}
	}

	It("UT-ML-2023-101: without repeat_tool_call, a scenario's first-ever match is poisoned by an earlier unrelated scenario's FunctionResponse", func() {
		registry := scenarios.DefaultRegistryWithOverrides(buildTwoScenarioOverrides(false))
		router := handlers.NewRouter(registry, false, "")
		ts := httptest.NewServer(router)
		defer ts.Close()

		body, err := json.Marshal(groundedDecisionRequest())
		Expect(err).NotTo(HaveOccurred())

		resp, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var gemResp response.GeminiResponse
		Expect(json.NewDecoder(resp.Body).Decode(&gemResp)).To(Succeed())
		Expect(gemResp.Candidates).To(HaveLen(1))

		parts := gemResp.Candidates[0].Content.Parts
		Expect(parts).To(HaveLen(1))
		Expect(parts[0].FunctionCall).To(BeNil(),
			"documents the bug: af_decision's tool call is suppressed by af_ground's unrelated FunctionResponse")
		Expect(parts[0].Text).NotTo(BeEmpty())
	})

	It("UT-ML-2023-102: repeat_tool_call fixes the interaction, letting the new scenario fire its own tool call", func() {
		registry := scenarios.DefaultRegistryWithOverrides(buildTwoScenarioOverrides(true))
		router := handlers.NewRouter(registry, false, "")
		ts := httptest.NewServer(router)
		defer ts.Close()

		body, err := json.Marshal(groundedDecisionRequest())
		Expect(err).NotTo(HaveOccurred())

		resp, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var gemResp response.GeminiResponse
		Expect(json.NewDecoder(resp.Body).Decode(&gemResp)).To(Succeed())
		Expect(gemResp.Candidates).To(HaveLen(1))

		parts := gemResp.Candidates[0].Content.Parts
		Expect(parts).To(HaveLen(1))
		Expect(parts[0].FunctionCall).NotTo(BeNil(),
			"repeat_tool_call must let af_decision fire despite af_ground's earlier unrelated FunctionResponse")
		Expect(parts[0].FunctionCall.Name).To(Equal("kubernaut_present_decision"))

		// Second internal turn: once af_decision's OWN FunctionResponse is
		// the last content, it must still fall through to text (no infinite
		// loop) -- LastContentIsFunctionResponse scopes the guard to this
		// scenario's own last turn, not the whole conversation.
		followUp := groundedDecisionRequest()
		followUp.Contents = append(followUp.Contents,
			response.GeminiContent{Role: "model", Parts: []response.GeminiPart{{FunctionCall: &response.GeminiFunctionCall{Name: "kubernaut_present_decision"}}}},
			response.GeminiContent{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{Name: "kubernaut_present_decision", Response: map[string]interface{}{"acked": true}}}}},
		)
		body2, err := json.Marshal(followUp)
		Expect(err).NotTo(HaveOccurred())

		resp2, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body2))
		Expect(err).NotTo(HaveOccurred())
		defer resp2.Body.Close()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK))

		var gemResp2 response.GeminiResponse
		Expect(json.NewDecoder(resp2.Body).Decode(&gemResp2)).To(Succeed())
		Expect(gemResp2.Candidates).To(HaveLen(1))
		parts2 := gemResp2.Candidates[0].Content.Parts
		Expect(parts2).To(HaveLen(1))
		Expect(parts2[0].FunctionCall).To(BeNil(),
			"must not re-fire kubernaut_present_decision once its own result is already in history")
	})
})
