package agent

import (
	"log"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// historySanitizer is a BeforeModelCallback that patches orphaned FunctionCall
// parts in the conversation history. An orphaned FunctionCall is one that has
// no matching FunctionResponse anywhere in the Contents. This can occur when
// the ADK runner stores a model response event (containing FunctionCalls) in
// the session but the tool execution is interrupted before the FunctionResponse
// event is stored (e.g., SSE disconnect, queue write failure).
//
// On multi-turn replay the orphaned tool_use block triggers a 400 from the
// Anthropic API ("tool_use ids were found without tool_result blocks").
//
// The sanitizer injects a synthetic FunctionResponse immediately after the
// Content containing the orphaned FunctionCall, allowing the model to see
// that the prior tool call was interrupted and proceed normally.
//
// Issue: #1299
func historySanitizer(_ agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
	if len(req.Contents) == 0 {
		return nil, nil
	}

	responseIDs := collectFunctionResponseIDs(req.Contents)

	var orphans []orphanCall

	for i, c := range req.Contents {
		if c == nil {
			continue
		}
		for _, p := range c.Parts {
			if p == nil || p.FunctionCall == nil || p.FunctionCall.ID == "" {
				continue
			}
			if _, hasResponse := responseIDs[p.FunctionCall.ID]; !hasResponse {
				orphans = append(orphans, orphanCall{contentIdx: i, call: p.FunctionCall})
			}
		}
	}

	if len(orphans) == 0 {
		return nil, nil
	}

	log.Printf("[history-sanitizer] patching %d orphaned FunctionCall(s) in session history (#1299)", len(orphans))

	// Group orphans by content index so we insert one synthetic Content per
	// source Content, preserving Anthropic's requirement that all tool_results
	// for a given tool_use group appear in a single user message.
	grouped := groupOrphansByIndex(orphans)

	// Build the patched contents slice. We walk the original contents and
	// insert a synthetic FunctionResponse content after each content that
	// contained orphaned FunctionCalls.
	patched := make([]*genai.Content, 0, len(req.Contents)+len(grouped))
	for i, c := range req.Contents {
		patched = append(patched, c)
		if calls, ok := grouped[i]; ok {
			patched = append(patched, syntheticResponseContent(calls))
		}
	}

	req.Contents = patched
	return nil, nil
}

func collectFunctionResponseIDs(contents []*genai.Content) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, c := range contents {
		if c == nil {
			continue
		}
		for _, p := range c.Parts {
			if p != nil && p.FunctionResponse != nil && p.FunctionResponse.ID != "" {
				ids[p.FunctionResponse.ID] = struct{}{}
			}
		}
	}
	return ids
}

type orphanCall struct {
	contentIdx int
	call       *genai.FunctionCall
}

func groupOrphansByIndex(orphans []orphanCall) map[int][]*genai.FunctionCall {
	m := make(map[int][]*genai.FunctionCall)
	for _, o := range orphans {
		m[o.contentIdx] = append(m[o.contentIdx], o.call)
	}
	return m
}

func syntheticResponseContent(calls []*genai.FunctionCall) *genai.Content {
	parts := make([]*genai.Part, 0, len(calls))
	for _, call := range calls {
		parts = append(parts, &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				ID:   call.ID,
				Name: call.Name,
				Response: map[string]any{
					"error": "tool call interrupted — session was resumed",
				},
			},
		})
	}
	return &genai.Content{Role: "user", Parts: parts}
}
