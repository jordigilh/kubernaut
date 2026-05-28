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
package tools

import (
	"encoding/json"
)

// StreamEventDigest is a compact representation of an SSE event for the LLM.
type StreamEventDigest struct {
	Type  string `json:"type"`
	Phase string `json:"phase,omitempty"`
	Text  string `json:"text,omitempty"`
}

func extractTextFromData(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		return str
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err == nil {
		if text, ok := obj["text"].(string); ok {
			return text
		}
		if content, ok := obj["content"].(string); ok {
			return content
		}
		if preview, ok := obj["content_preview"].(string); ok {
			return preview
		}
		if delta, ok := obj["delta"].(string); ok {
			return delta
		}
		if summary, ok := obj["summary"].(string); ok {
			return summary
		}
	}
	return string(data)
}

// ExtractToolResult parses a tool_result SSE event data payload and returns:
//   - toolName: the tool that produced the result (empty for flat test format)
//   - preview: the result preview text
//   - isErr: true if the result contains a toolErrorJSON payload
//
// Handles two wire formats:
//  1. Production KA envelope: {"type":"tool_result","turn":N,"data":{"tool_name":...,"result_preview":...}}
//  2. Test flat format: JSON string (e.g. "pod-1 Running")
func ExtractToolResult(data json.RawMessage) (toolName, preview string, isErr bool) {
	if len(data) == 0 {
		return "", "", false
	}

	// Try flat JSON string first (test format)
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		return "", str, isToolErrorContent(str)
	}

	// Try production KA envelope
	var envelope struct {
		Data struct {
			ToolName      string `json:"tool_name"`
			ResultPreview string `json:"result_preview"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Data.ToolName != "" {
		return envelope.Data.ToolName, envelope.Data.ResultPreview, isToolErrorContent(envelope.Data.ResultPreview)
	}

	// Fallback: try as a simple object with tool_name/result_preview at top level
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err == nil {
		if tn, ok := obj["tool_name"].(string); ok {
			rp, _ := obj["result_preview"].(string)
			return tn, rp, isToolErrorContent(rp)
		}
	}

	return "", string(data), false
}

// isToolErrorContent checks if the content matches KA's toolErrorJSON format:
// {"status":"error","error":"..."}
func isToolErrorContent(s string) bool {
	var errPayload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(s), &errPayload); err == nil {
		return errPayload.Status == "error"
	}
	return false
}

// FormatToolError produces a human-readable error message from a tool name
// and its error content. The message is truncated to 200 characters.
func FormatToolError(toolName, rawError string) string {
	errText := rawError

	// Try to extract the "error" field from toolErrorJSON
	var errPayload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(rawError), &errPayload) == nil && errPayload.Error != "" {
		errText = errPayload.Error
	}

	msg := toolName + ": " + errText
	if len(msg) > 200 {
		msg = msg[:197] + "..."
	}
	return msg
}

func extractSummaryFromComplete(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err == nil {
		if summary, ok := obj["summary"].(string); ok {
			return summary
		}
		if rootCause, ok := obj["root_cause"].(string); ok {
			return rootCause
		}
	}
	text := extractTextFromData(data)
	return truncateForLLM(text, 2000)
}

func truncateForLLM(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "... (truncated)"
}
