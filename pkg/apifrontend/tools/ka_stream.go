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
