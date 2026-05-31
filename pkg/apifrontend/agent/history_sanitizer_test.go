package agent

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

type stubCallbackContext struct {
	context.Context
}

func (s *stubCallbackContext) UserContent() *genai.Content              { return nil }
func (s *stubCallbackContext) InvocationID() string                     { return "" }
func (s *stubCallbackContext) AgentName() string                        { return "test-agent" }
func (s *stubCallbackContext) ReadonlyState() session.ReadonlyState     { return nil }
func (s *stubCallbackContext) UserID() string                           { return "" }
func (s *stubCallbackContext) AppName() string                          { return "" }
func (s *stubCallbackContext) SessionID() string                        { return "" }
func (s *stubCallbackContext) Branch() string                           { return "" }
func (s *stubCallbackContext) Artifacts() agent.Artifacts               { return nil }
func (s *stubCallbackContext) State() session.State                     { return nil }

func testCallbackCtx() agent.CallbackContext {
	return &stubCallbackContext{Context: context.Background()}
}

var _ = Describe("historySanitizer BeforeModelCallback (#1299)", func() {

	makeContents := func(items ...*genai.Content) []*genai.Content {
		return items
	}

	userText := func(text string) *genai.Content {
		return &genai.Content{Role: "user", Parts: []*genai.Part{{Text: text}}}
	}

	modelText := func(text string) *genai.Content {
		return &genai.Content{Role: "model", Parts: []*genai.Part{{Text: text}}}
	}

	modelWithCall := func(text, callID, toolName string) *genai.Content {
		parts := []*genai.Part{
			{Text: text},
			{FunctionCall: &genai.FunctionCall{ID: callID, Name: toolName, Args: map[string]any{"ns": "demo"}}},
		}
		return &genai.Content{Role: "model", Parts: parts}
	}

	toolResult := func(callID, toolName string, resp map[string]any) *genai.Content {
		return &genai.Content{
			Role: "user",
			Parts: []*genai.Part{
				{FunctionResponse: &genai.FunctionResponse{ID: callID, Name: toolName, Response: resp}},
			},
		}
	}

	callReq := func(contents []*genai.Content) *model.LLMRequest {
		return &model.LLMRequest{Contents: contents}
	}

	It("UT-AF-1299-001: no FunctionCalls in history — contents unchanged", func() {
		req := callReq(makeContents(
			userText("investigate the incident"),
			modelText("I found the issue."),
		))
		original := len(req.Contents)

		resp, err := historySanitizer(testCallbackCtx(), req)

		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeNil(), "must return nil to let model call proceed")
		Expect(req.Contents).To(HaveLen(original), "contents must not be modified")
	})

	It("UT-AF-1299-002: properly paired FunctionCall/FunctionResponse — contents unchanged", func() {
		req := callReq(makeContents(
			userText("investigate"),
			modelWithCall("Starting", "toolu_vrtx_001", "kubernaut_investigate"),
			toolResult("toolu_vrtx_001", "kubernaut_investigate", map[string]any{"status": "ok"}),
			modelText("Investigation complete."),
		))
		original := len(req.Contents)

		resp, err := historySanitizer(testCallbackCtx(), req)

		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeNil())
		Expect(req.Contents).To(HaveLen(original), "properly paired contents must not be modified")
	})

	It("UT-AF-1299-003: single orphaned FunctionCall — synthetic FunctionResponse injected", func() {
		req := callReq(makeContents(
			userText("investigate"),
			modelWithCall("Let me check", "toolu_vrtx_orphan", "kubernaut_investigate"),
			// Missing FunctionResponse for toolu_vrtx_orphan
			userText("start the investigation again"),
		))

		resp, err := historySanitizer(testCallbackCtx(), req)

		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeNil())

		// A synthetic FunctionResponse content should have been injected
		var foundSyntheticResponse bool
		for _, c := range req.Contents {
			for _, p := range c.Parts {
				if p.FunctionResponse != nil && p.FunctionResponse.ID == "toolu_vrtx_orphan" {
					foundSyntheticResponse = true
					Expect(p.FunctionResponse.Name).To(Equal("kubernaut_investigate"))
					Expect(p.FunctionResponse.Response).To(HaveKey("error"))
				}
			}
		}
		Expect(foundSyntheticResponse).To(BeTrue(),
			"a synthetic FunctionResponse must be injected for the orphaned FunctionCall")
	})

	It("UT-AF-1299-004: multiple orphaned FunctionCalls — all get synthetic responses", func() {
		req := callReq(makeContents(
			userText("investigate"),
			&genai.Content{Role: "model", Parts: []*genai.Part{
				{FunctionCall: &genai.FunctionCall{ID: "call-A", Name: "tool_a"}},
				{FunctionCall: &genai.FunctionCall{ID: "call-B", Name: "tool_b"}},
			}},
			// No FunctionResponses for either call
			userText("try again"),
		))

		resp, err := historySanitizer(testCallbackCtx(), req)

		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeNil())

		responseIDs := map[string]bool{}
		for _, c := range req.Contents {
			for _, p := range c.Parts {
				if p.FunctionResponse != nil {
					responseIDs[p.FunctionResponse.ID] = true
				}
			}
		}
		Expect(responseIDs).To(HaveKey("call-A"), "synthetic response for call-A")
		Expect(responseIDs).To(HaveKey("call-B"), "synthetic response for call-B")
	})

	It("UT-AF-1299-005: mixed paired and orphaned — only orphans patched", func() {
		req := callReq(makeContents(
			userText("investigate"),
			modelWithCall("Step 1", "paired-001", "tool_ok"),
			toolResult("paired-001", "tool_ok", map[string]any{"status": "done"}),
			modelWithCall("Step 2", "orphan-002", "tool_broken"),
			// Missing response for orphan-002
			userText("continue"),
		))

		resp, err := historySanitizer(testCallbackCtx(), req)

		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeNil())

		responseIDs := map[string]bool{}
		for _, c := range req.Contents {
			for _, p := range c.Parts {
				if p.FunctionResponse != nil {
					responseIDs[p.FunctionResponse.ID] = true
				}
			}
		}
		Expect(responseIDs).To(HaveKey("paired-001"), "original paired response preserved")
		Expect(responseIDs).To(HaveKey("orphan-002"), "synthetic response for orphan")
	})

	It("UT-AF-1299-006: synthetic response is placed immediately after orphaned FunctionCall content", func() {
		req := callReq(makeContents(
			userText("investigate"),
			modelWithCall("Calling tool", "orphan-pos", "kubernaut_investigate"),
			userText("continue please"),
		))

		_, _ = historySanitizer(testCallbackCtx(), req)

		// Find the index of the model content with the FunctionCall
		callIdx := -1
		for i, c := range req.Contents {
			for _, p := range c.Parts {
				if p.FunctionCall != nil && p.FunctionCall.ID == "orphan-pos" {
					callIdx = i
				}
			}
		}
		Expect(callIdx).To(BeNumerically(">=", 0))

		// The next content must be the synthetic FunctionResponse
		Expect(len(req.Contents)).To(BeNumerically(">", callIdx+1))
		nextContent := req.Contents[callIdx+1]
		Expect(nextContent.Role).To(Equal("user"),
			"synthetic tool_result must be in a user-role content")
		Expect(nextContent.Parts).To(HaveLen(1))
		Expect(nextContent.Parts[0].FunctionResponse).NotTo(BeNil())
		Expect(nextContent.Parts[0].FunctionResponse.ID).To(Equal("orphan-pos"))
	})

	It("UT-AF-1299-007: nil and empty contents handled gracefully", func() {
		reqNil := &model.LLMRequest{Contents: nil}
		resp, err := historySanitizer(testCallbackCtx(), reqNil)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeNil())

		reqEmpty := &model.LLMRequest{Contents: []*genai.Content{}}
		resp, err = historySanitizer(testCallbackCtx(), reqEmpty)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeNil())
	})
})
