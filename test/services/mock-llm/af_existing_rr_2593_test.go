package mockllm_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Existing RR contention fixture [BR-INTERACTIVE-004, BR-INTEGRATION-065]", func() {
	It("UT-MOCK-AF-2593-003: actual Gemini HTTP output contains rr_id alone from the last user message", func() {
		server := httptest.NewServer(handlers.NewRouter(scenarios.DefaultRegistryWithOverrides(nil), false, config.ModeFull))
		DeferCleanup(server.Close)
		request := response.GeminiRequest{
			Contents: []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{Text: "fleet lease contention investigate rr_id=rr-retained"}}},
				{Role: "model", Parts: []response.GeminiPart{{Text: "Earlier conversation."}}},
				{Role: "user", Parts: []response.GeminiPart{{Text: "fleet lease contention investigate rr_id=rr-actual-current"}}},
			},
			Tools: []response.GeminiToolDecl{{FunctionDeclarations: []response.GeminiFunctionDecl{{Name: "kubernaut_investigate"}}}},
		}
		payload, err := json.Marshal(request)
		Expect(err).NotTo(HaveOccurred())
		resp, err := http.Post(server.URL+"/v1beta/models/mock:generateContent", "application/json", bytes.NewReader(payload))
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(resp.Body.Close()).To(Succeed()) }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		var output response.GeminiResponse
		Expect(json.NewDecoder(resp.Body).Decode(&output)).To(Succeed())
		Expect(output.Candidates).To(HaveLen(1))
		Expect(output.Candidates[0].Content.Parts).To(HaveLen(1))
		call := output.Candidates[0].Content.Parts[0].FunctionCall
		Expect(call).NotTo(BeNil())
		Expect(call.Name).To(Equal("kubernaut_investigate"))
		Expect(call.Args).To(Equal(map[string]interface{}{"rr_id": "rr-actual-current"}))
	})

	It("UT-MOCK-AF-2593-001: emits only the current request's authoritative rr_id through the provider handler", func() {
		server := httptest.NewServer(handlers.NewRouter(scenarios.DefaultRegistryWithOverrides(nil), false, config.ModeFull))
		DeferCleanup(server.Close)
		var workers sync.WaitGroup
		for i := range 24 {
			workers.Add(1)
			go func(i int) {
				defer GinkgoRecover()
				defer workers.Done()
				rrID := fmt.Sprintf("rr-current-%d", i)
				prompt := "fleet lease contention investigate rr_id=" + rrID
				response := postOpenAI(server.URL, openai.ChatCompletionRequest{
					Model:    "mock-model",
					Messages: []openai.Message{{Role: "user", Content: &prompt}},
					Tools:    []openai.Tool{{Type: "function", Function: openai.ToolDefinition{Name: "kubernaut_investigate"}}},
				})
				Expect(response.Choices).To(HaveLen(1))
				Expect(response.Choices[0].Message.ToolCalls).To(HaveLen(1))
				call := response.Choices[0].Message.ToolCalls[0]
				Expect(call.Function.Name).To(Equal("kubernaut_investigate"))
				var args map[string]string
				Expect(json.Unmarshal([]byte(call.Function.Arguments), &args)).To(Succeed())
				Expect(args).To(Equal(map[string]string{"rr_id": rrID}), "no creation args, shared mutation or cross-conversation lookup")
			}(i)
		}
		workers.Wait()
	})

	DescribeTable("UT-MOCK-AF-2593-002: no marker injection, stale history, or non-AF selection", func(last, history string, caller scenarios.Caller) {
		result := scenarios.DefaultRegistryWithOverrides(nil).Detect(&scenarios.DetectionContext{
			LastUserContent: last, Content: history, AllText: history,
			Caller: caller, AvailableTools: []string{"kubernaut_investigate"},
		})
		if result != nil {
			Expect(result.Scenario.Name()).NotTo(Equal("af_fleet_existing_rr_2593"))
		}
	},
		Entry("history is not an instruction", "continue", "fleet lease contention investigate rr_id=rr-old", scenarios.CallerAF),
		Entry("URL is not an RR", "fleet lease contention investigate rr_id=https://example.invalid/x", "", scenarios.CallerAF),
		Entry("extra args are not accepted", "fleet lease contention investigate rr_id=rr-ok namespace=other", "", scenarios.CallerAF),
		Entry("empty ID", "fleet lease contention investigate rr_id=", "", scenarios.CallerAF),
		Entry("KA must not select AF fixture", "fleet lease contention investigate rr_id=rr-ok", "", scenarios.CallerKA),
	)
})
