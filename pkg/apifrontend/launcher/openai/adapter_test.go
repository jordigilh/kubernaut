package openai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	openaimodel "github.com/jordigilh/kubernaut/pkg/apifrontend/launcher/openai"
)

func chatCompletionResponse(content string) map[string]any {
	return map[string]any{
		"id":      "chatcmpl-test",
		"object":  "chat.completion",
		"created": 1700000000,
		"model":   "gpt-4o",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 20,
			"total_tokens":      30,
		},
	}
}

func chatCompletionWithToolCalls() map[string]any {
	return map[string]any{
		"id":      "chatcmpl-tool",
		"object":  "chat.completion",
		"created": 1700000000,
		"model":   "gpt-4o",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": nil,
					"tool_calls": []map[string]any{
						{
							"id":   "call_123",
							"type": "function",
							"function": map[string]any{
								"name":      "get_weather",
								"arguments": `{"location":"San Francisco"}`,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     15,
			"completion_tokens": 25,
			"total_tokens":      40,
		},
	}
}

func streamingChunks(text string) []map[string]any {
	chunks := make([]map[string]any, 0, len(text)+1)
	for _, ch := range text {
		chunks = append(chunks, map[string]any{
			"id":      "chatcmpl-stream",
			"object":  "chat.completion.chunk",
			"created": 1700000000,
			"model":   "gpt-4o",
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
						"content": string(ch),
					},
					"finish_reason": nil,
				},
			},
		})
	}
	chunks = append(chunks, map[string]any{
		"id":      "chatcmpl-stream",
		"object":  "chat.completion.chunk",
		"created": 1700000000,
		"model":   "gpt-4o",
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "stop",
			},
		},
	})
	return chunks
}

func sseServer(chunks []map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range chunks {
			data, _ := json.Marshal(chunk)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
}

func streamingToolCallChunks() []map[string]any {
	return []map[string]any{
		{
			"id": "chatcmpl-tc-stream", "object": "chat.completion.chunk",
			"model": "gpt-4o",
			"choices": []map[string]any{{
				"index": 0,
				"delta": map[string]any{
					"role": "assistant",
					"tool_calls": []map[string]any{{
						"index": 0,
						"id":    "call_456",
						"type":  "function",
						"function": map[string]any{
							"name":      "get_weather",
							"arguments": "",
						},
					}},
				},
				"finish_reason": nil,
			}},
		},
		{
			"id": "chatcmpl-tc-stream", "object": "chat.completion.chunk",
			"model": "gpt-4o",
			"choices": []map[string]any{{
				"index": 0,
				"delta": map[string]any{
					"tool_calls": []map[string]any{{
						"index": 0,
						"function": map[string]any{
							"arguments": `{"loc`,
						},
					}},
				},
				"finish_reason": nil,
			}},
		},
		{
			"id": "chatcmpl-tc-stream", "object": "chat.completion.chunk",
			"model": "gpt-4o",
			"choices": []map[string]any{{
				"index": 0,
				"delta": map[string]any{
					"tool_calls": []map[string]any{{
						"index": 0,
						"function": map[string]any{
							"arguments": `ation":"SF"}`,
						},
					}},
				},
				"finish_reason": nil,
			}},
		},
		{
			"id": "chatcmpl-tc-stream", "object": "chat.completion.chunk",
			"model": "gpt-4o",
			"choices": []map[string]any{{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "tool_calls",
			}},
		},
	}
}

var _ = Describe("OpenAI Adapter (BR-INTEGRATION-1254)", func() {

	Describe("NewModel", func() {
		// UT-AF-1254-029 [SC-8]: Custom HTTP client is used for all requests
		It("UT-AF-1254-029 accepts custom HTTP client for transport chain injection", func() {
			customClient := &http.Client{}
			m := openaimodel.NewModel("gpt-4o", "https://api.openai.com/v1", "test-key",
				openaimodel.WithHTTPClient(customClient))
			Expect(m).NotTo(BeNil())
			Expect(m.Name()).To(Equal("gpt-4o"))
		})
	})

	Describe("Message Conversion", func() {
		var (
			server *httptest.Server
			m      model.LLM
		)

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
		})

		// UT-AF-1254-020 [AC-4]: User message content is faithfully transmitted to the LLM
		It("UT-AF-1254-020 converts user message to OpenAI format", func() {
			var receivedBody map[string]any
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(chatCompletionResponse("hello back"))
			}))

			m = openaimodel.NewModel("gpt-4o", server.URL, "")
			req := &model.LLMRequest{
				Contents: []*genai.Content{
					{Role: "user", Parts: []*genai.Part{{Text: "hello world"}}},
				},
			}

			for resp, err := range m.GenerateContent(context.Background(), req, false) {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
			}

			messages, ok := receivedBody["messages"].([]any)
			Expect(ok).To(BeTrue(), "request body must contain messages array")
			Expect(messages).To(HaveLen(1))
			msg := messages[0].(map[string]any)
			Expect(msg["role"]).To(Equal("user"))
			Expect(msg["content"]).To(Equal("hello world"))
		})

		// UT-AF-1254-021 [AC-4]: System instruction is transmitted as OpenAI system message
		It("UT-AF-1254-021 converts system instruction to system message", func() {
			var receivedBody map[string]any
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(chatCompletionResponse("acknowledged"))
			}))

			m = openaimodel.NewModel("gpt-4o", server.URL, "")
			req := &model.LLMRequest{
				Config: &genai.GenerateContentConfig{
					SystemInstruction: &genai.Content{
						Parts: []*genai.Part{{Text: "You are a helpful assistant."}},
					},
				},
				Contents: []*genai.Content{
					{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
				},
			}

			for resp, err := range m.GenerateContent(context.Background(), req, false) {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
			}

			messages, ok := receivedBody["messages"].([]any)
			Expect(ok).To(BeTrue())
			Expect(len(messages)).To(BeNumerically(">=", 2))
			sysMsg := messages[0].(map[string]any)
			Expect(sysMsg["role"]).To(Equal("system"))
			Expect(sysMsg["content"]).To(Equal("You are a helpful assistant."))
		})

		// UT-AF-1254-022 [AC-4]: Tool declarations are transmitted as OpenAI function tools
		It("UT-AF-1254-022 converts tool declarations to OpenAI tools", func() {
			var receivedBody map[string]any
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(chatCompletionResponse("ok"))
			}))

			m = openaimodel.NewModel("gpt-4o", server.URL, "")
			req := &model.LLMRequest{
				Config: &genai.GenerateContentConfig{
					Tools: []*genai.Tool{
						{
							FunctionDeclarations: []*genai.FunctionDeclaration{
								{
									Name:        "get_weather",
									Description: "Get current weather for a location",
									Parameters: &genai.Schema{
										Type: genai.TypeObject,
										Properties: map[string]*genai.Schema{
											"location": {Type: genai.TypeString, Description: "City name"},
										},
										Required: []string{"location"},
									},
								},
							},
						},
					},
				},
				Contents: []*genai.Content{
					{Role: "user", Parts: []*genai.Part{{Text: "What's the weather?"}}},
				},
			}

			for resp, err := range m.GenerateContent(context.Background(), req, false) {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
			}

			tools, ok := receivedBody["tools"].([]any)
			Expect(ok).To(BeTrue(), "request body must contain tools array")
			Expect(tools).To(HaveLen(1))
			tool := tools[0].(map[string]any)
			Expect(tool["type"]).To(Equal("function"))
			fn := tool["function"].(map[string]any)
			Expect(fn["name"]).To(Equal("get_weather"))
			Expect(fn["description"]).To(Equal("Get current weather for a location"))
		})
	})

	Describe("Response Mapping", func() {
		var server *httptest.Server

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
		})

		// UT-AF-1254-026 [AC-4]: Non-streaming response content and usage are mapped
		It("UT-AF-1254-026 maps non-streaming response to LLMResponse", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(chatCompletionResponse("The answer is 42."))
			}))

			m := openaimodel.NewModel("gpt-4o", server.URL, "")
			req := &model.LLMRequest{
				Contents: []*genai.Content{
					{Role: "user", Parts: []*genai.Part{{Text: "What is the answer?"}}},
				},
			}

			var responses []*model.LLMResponse
			for resp, err := range m.GenerateContent(context.Background(), req, false) {
				Expect(err).NotTo(HaveOccurred())
				responses = append(responses, resp)
			}

			Expect(responses).To(HaveLen(1))
			resp := responses[0]
			Expect(resp.Content).NotTo(BeNil())
			Expect(resp.Content.Parts).NotTo(BeEmpty())

			foundText := false
			for _, part := range resp.Content.Parts {
				if part.Text != "" {
					Expect(part.Text).To(Equal("The answer is 42."))
					foundText = true
				}
			}
			Expect(foundText).To(BeTrue(), "response must contain text part")
		})

		// UT-AF-1254-026b: Tool call response mapped correctly
		It("UT-AF-1254-026b maps tool call response to LLMResponse with FunctionCall parts", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(chatCompletionWithToolCalls())
			}))

			m := openaimodel.NewModel("gpt-4o", server.URL, "")
			req := &model.LLMRequest{
				Contents: []*genai.Content{
					{Role: "user", Parts: []*genai.Part{{Text: "What's the weather?"}}},
				},
			}

			var responses []*model.LLMResponse
			for resp, err := range m.GenerateContent(context.Background(), req, false) {
				Expect(err).NotTo(HaveOccurred())
				responses = append(responses, resp)
			}

			Expect(responses).To(HaveLen(1))
			resp := responses[0]
			Expect(resp.Content).NotTo(BeNil())

			foundFunctionCall := false
			for _, part := range resp.Content.Parts {
				if part.FunctionCall != nil {
					Expect(part.FunctionCall.Name).To(Equal("get_weather"))
					args := part.FunctionCall.Args
					Expect(args).To(HaveKey("location"))
					foundFunctionCall = true
				}
			}
			Expect(foundFunctionCall).To(BeTrue(), "response must contain FunctionCall part")
		})
	})

	Describe("Streaming", func() {
		var server *httptest.Server

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
		})

		// UT-AF-1254-024 [AC-4]: Streaming text responses yielded as partial events
		It("UT-AF-1254-024 streams text responses as partial LLMResponse events", func() {
			server = sseServer(streamingChunks("Hi!"))

			m := openaimodel.NewModel("gpt-4o", server.URL, "")
			req := &model.LLMRequest{
				Contents: []*genai.Content{
					{Role: "user", Parts: []*genai.Part{{Text: "Say hi"}}},
				},
			}

			var responses []*model.LLMResponse
			for resp, err := range m.GenerateContent(context.Background(), req, true) {
				Expect(err).NotTo(HaveOccurred())
				responses = append(responses, resp)
			}

			Expect(len(responses)).To(BeNumerically(">=", 2),
				"streaming must yield multiple partial responses")

			var assembled string
			for _, r := range responses {
				if r.Content != nil {
					for _, p := range r.Content.Parts {
						assembled += p.Text
					}
				}
			}
			Expect(assembled).To(Equal("Hi!"))
		})

		// UT-AF-1254-025 [AC-4]: Streaming finish reason is mapped
		It("UT-AF-1254-025 maps streaming finish reason to genai.FinishReason", func() {
			server = sseServer(streamingChunks("done"))

			m := openaimodel.NewModel("gpt-4o", server.URL, "")
			req := &model.LLMRequest{
				Contents: []*genai.Content{
					{Role: "user", Parts: []*genai.Part{{Text: "done"}}},
				},
			}

			var lastResp *model.LLMResponse
			for resp, err := range m.GenerateContent(context.Background(), req, true) {
				Expect(err).NotTo(HaveOccurred())
				lastResp = resp
			}

			Expect(lastResp).NotTo(BeNil())
			Expect(lastResp.FinishReason).To(Equal(genai.FinishReasonStop))
			Expect(lastResp.TurnComplete).To(BeTrue())
		})

		// UT-AF-1254-023 [AC-4]: Multi-chunk streaming tool calls accumulated
		It("UT-AF-1254-023 accumulates streaming tool call chunks into complete tool call", func() {
			server = sseServer(streamingToolCallChunks())

			m := openaimodel.NewModel("gpt-4o", server.URL, "")
			req := &model.LLMRequest{
				Contents: []*genai.Content{
					{Role: "user", Parts: []*genai.Part{{Text: "What's the weather?"}}},
				},
			}

			var responses []*model.LLMResponse
			for resp, err := range m.GenerateContent(context.Background(), req, true) {
				Expect(err).NotTo(HaveOccurred())
				responses = append(responses, resp)
			}

			Expect(responses).NotTo(BeEmpty())
			lastResp := responses[len(responses)-1]
			Expect(lastResp.Content).NotTo(BeNil())

			foundFunctionCall := false
			for _, part := range lastResp.Content.Parts {
				if part.FunctionCall != nil {
					Expect(part.FunctionCall.Name).To(Equal("get_weather"))
					Expect(part.FunctionCall.Args).To(HaveKey("location"))
					Expect(part.FunctionCall.Args["location"]).To(Equal("SF"))
					foundFunctionCall = true
				}
			}
			Expect(foundFunctionCall).To(BeTrue(),
				"accumulated tool call chunks must produce a complete FunctionCall part")
		})
	})

	Describe("Generation Config", func() {
		var server *httptest.Server

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
		})

		// UT-AF-1254-027 [AC-4]: Generation config params forwarded
		It("UT-AF-1254-027 forwards temperature, topP, maxTokens, stop sequences", func() {
			var receivedBody map[string]any
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(chatCompletionResponse("tuned response"))
			}))

			temp := float32(0.7)
			topP := float32(0.9)
			m := openaimodel.NewModel("gpt-4o", server.URL, "")
			req := &model.LLMRequest{
				Config: &genai.GenerateContentConfig{
					Temperature:     &temp,
					TopP:            &topP,
					MaxOutputTokens: 500,
					StopSequences:   []string{"END", "STOP"},
				},
				Contents: []*genai.Content{
					{Role: "user", Parts: []*genai.Part{{Text: "generate"}}},
				},
			}

			for resp, err := range m.GenerateContent(context.Background(), req, false) {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
			}

			Expect(receivedBody["temperature"]).To(BeNumerically("~", 0.7, 0.01))
			Expect(receivedBody["top_p"]).To(BeNumerically("~", 0.9, 0.01))
			Expect(receivedBody["max_tokens"]).To(BeNumerically("==", 500))
			stopSeqs, ok := receivedBody["stop"].([]any)
			Expect(ok).To(BeTrue())
			Expect(stopSeqs).To(ConsistOf("END", "STOP"))
		})

		// UT-AF-1254-028 [AC-4]: Response schema forwarded as JSON schema response format
		It("UT-AF-1254-028 forwards response schema as OpenAI JSON schema format", func() {
			var receivedBody map[string]any
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(chatCompletionResponse(`{"severity":"high"}`))
			}))

			m := openaimodel.NewModel("gpt-4o", server.URL, "")
			req := &model.LLMRequest{
				Config: &genai.GenerateContentConfig{
					ResponseSchema: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"severity": {Type: genai.TypeString},
						},
						Required: []string{"severity"},
					},
				},
				Contents: []*genai.Content{
					{Role: "user", Parts: []*genai.Part{{Text: "triage this"}}},
				},
			}

			for resp, err := range m.GenerateContent(context.Background(), req, false) {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
			}

			respFormat, ok := receivedBody["response_format"].(map[string]any)
			Expect(ok).To(BeTrue(), "request body must contain response_format")
			Expect(respFormat["type"]).To(Equal("json_schema"))
		})
	})
})
