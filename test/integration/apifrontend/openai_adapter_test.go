package apifrontend_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("OpenAI Adapter Wiring (BR-INTEGRATION-1254)", func() {

	var (
		mockServer   *httptest.Server
		requestCount atomic.Int32
	)

	chatCompletionHandler := func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-it",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "llama3.1",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Integration test response from mock LLM.",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     5,
				"completion_tokens": 10,
				"total_tokens":      15,
			},
		})
	}

	BeforeEach(func() {
		requestCount.Store(0)
		mockServer = httptest.NewServer(http.HandlerFunc(chatCompletionHandler))
	})

	AfterEach(func() {
		if mockServer != nil {
			mockServer.Close()
		}
	})

	simpleLLMRequest := func(text string) *model.LLMRequest {
		return &model.LLMRequest{
			Contents: []*genai.Content{
				{Role: "user", Parts: []*genai.Part{{Text: text}}},
			},
		}
	}

	// IT-AF-1254-001 [CM-6]: Factory dispatches to OpenAI adapter
	It("IT-AF-1254-001 NewModelFromConfig with openai_compatible returns working model.LLM", func() {
		cfg := config.LLMConfig{
			Provider: config.LLMProviderOpenAICompatible,
			Model:    "llama3.1",
			Endpoint: mockServer.URL,
			APIKey:   "test-api-key",
		}

		m, err := launcher.NewModelFromConfig(context.Background(), cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(m).NotTo(BeNil())
		Expect(m.Name()).To(Equal("llama3.1"))
	})

	// IT-AF-1254-002 [IA-5]: Factory wiring works without API key (keyless LlamaStack)
	It("IT-AF-1254-002 NewModelFromConfig with openai_compatible and no API key works", func() {
		cfg := config.LLMConfig{
			Provider: config.LLMProviderOpenAICompatible,
			Model:    "llama3.1",
			Endpoint: mockServer.URL,
			APIKey:   "",
		}

		m, err := launcher.NewModelFromConfig(context.Background(), cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(m).NotTo(BeNil())

		req := simpleLLMRequest("Hello")
		for resp, genErr := range m.GenerateContent(context.Background(), req, false) {
			Expect(genErr).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		}
		Expect(requestCount.Load()).To(BeNumerically(">", 0),
			"adapter must have sent at least one request to the mock server")
	})

	// IT-AF-1254-003 [SC-8]: Transport chain injected into adapter HTTP client
	It("IT-AF-1254-003 transport chain custom header is injected into adapter requests", func() {
		var receivedHeaders http.Header
		customServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeaders = r.Header.Clone()
			chatCompletionHandler(w, r)
		}))
		defer customServer.Close()

		cfg := config.LLMConfig{
			Provider: config.LLMProviderOpenAICompatible,
			Model:    "llama3.1",
			Endpoint: customServer.URL,
			CustomHeaders: []config.LLMHeader{
				{Name: "X-Kubernaut-Tenant", Value: "test-tenant"},
			},
		}

		m, err := launcher.NewModelFromConfig(context.Background(), cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(m).NotTo(BeNil())

		req := simpleLLMRequest("Hello with headers")
		for resp, genErr := range m.GenerateContent(context.Background(), req, false) {
			Expect(genErr).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		}

		Expect(receivedHeaders).NotTo(BeNil())
		Expect(receivedHeaders.Get("X-Kubernaut-Tenant")).To(Equal("test-tenant"),
			"custom header from transport chain must be injected into OpenAI adapter requests")
	})

	// IT-AF-1254-004 [AC-4]: Adapter round-trips chat completion via httptest
	It("IT-AF-1254-004 adapter round-trips chat completion with content and tool calls", func() {
		var receivedBody map[string]any
		roundTripServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-rt",
				"object":  "chat.completion",
				"created": 1700000000,
				"model":   "gpt-4o",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "Round-trip success!",
							"tool_calls": []map[string]any{
								{
									"id":   "call_rt",
									"type": "function",
									"function": map[string]any{
										"name":      "kubernaut_investigate",
										"arguments": `{"target":"my-deployment"}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     20,
					"completion_tokens": 30,
					"total_tokens":      50,
				},
			})
		}))
		defer roundTripServer.Close()

		cfg := config.LLMConfig{
			Provider: config.LLMProviderOpenAICompatible,
			Model:    "gpt-4o",
			Endpoint: roundTripServer.URL,
			APIKey:   "roundtrip-key",
		}

		m, err := launcher.NewModelFromConfig(context.Background(), cfg)
		Expect(err).NotTo(HaveOccurred())

		req := simpleLLMRequest("Investigate deployment issues")
		var lastResp *model.LLMResponse
		for resp, genErr := range m.GenerateContent(context.Background(), req, false) {
			Expect(genErr).NotTo(HaveOccurred())
			lastResp = resp
		}
		Expect(lastResp).NotTo(BeNil(), "must receive at least one response")

		// Verify request was correctly formatted (OpenAI API contract)
		Expect(receivedBody).To(HaveKey("model"))
		Expect(receivedBody["model"]).To(Equal("gpt-4o"))
		Expect(receivedBody).To(HaveKey("messages"))
		messages, ok := receivedBody["messages"].([]any)
		Expect(ok).To(BeTrue())
		Expect(messages).NotTo(BeEmpty())

		// Verify response contains tool call
		Expect(lastResp.Content).NotTo(BeNil())
		foundToolCall := false
		for _, part := range lastResp.Content.Parts {
			if part.FunctionCall != nil {
				Expect(part.FunctionCall.Name).To(Equal("kubernaut_investigate"))
				foundToolCall = true
			}
		}
		Expect(foundToolCall).To(BeTrue(),
			"response must contain FunctionCall part from the round-trip")
	})
})
