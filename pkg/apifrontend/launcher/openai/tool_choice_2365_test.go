package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	openaimodel "github.com/jordigilh/kubernaut/pkg/apifrontend/launcher/openai"
)

var _ = Describe("OpenAI adapter presentation recovery (#2365)", func() {
	It("IT-AF-2365-004 (SI-10, AU-3): maps ADK tool choice to OpenAI wire format", func() {
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"kubernaut_present_decision","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`))
		}))
		DeferCleanup(server.Close)

		modelClient := openaimodel.NewModel("gpt-4o", server.URL, "")
		var err error
		for _, runErr := range modelClient.GenerateContent(context.Background(), &model.LLMRequest{
			Config: &genai.GenerateContentConfig{
				Tools: []*genai.Tool{{FunctionDeclarations: []*genai.FunctionDeclaration{{Name: "kubernaut_present_decision"}}}},
				ToolConfig: &genai.ToolConfig{FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode:                 genai.FunctionCallingConfigModeAny,
					AllowedFunctionNames: []string{"kubernaut_present_decision"},
				}},
			},
		}, false) {
			err = runErr
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(body["tool_choice"]).To(Equal(map[string]any{
			"type":     "function",
			"function": map[string]any{"name": "kubernaut_present_decision"},
		}))
	})
})
