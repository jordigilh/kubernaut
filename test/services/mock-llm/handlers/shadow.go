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
package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
)

var injectionPatterns = []string{
	"system:",
	"admin note:",
	"important:",
	"assistant:",
	"ignore previous",
	"skip human review",
	"confidence=1.0",
	"override workflow",
	"forget your prompt",
	"you are now",
}

func handleShadowOpenAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	lastUserContent := extractLastUserContent(req.Messages)
	suspicious, matched := matchInjectionPattern(lastUserContent)

	var verdict string
	if suspicious {
		verdict = fmt.Sprintf(`{"suspicious":true,"explanation":"injection pattern detected: %s"}`, matched)
		log.Printf("[shadow] SUSPICIOUS — pattern=%q", matched)
	} else {
		verdict = `{"suspicious":false,"explanation":"clean"}`
		log.Printf("[shadow] CLEAN")
	}

	resp := openai.ChatCompletionResponse{
		ID:      "shadow-eval-001",
		Object:  openai.ObjectChatCompletion,
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []openai.Choice{
			{
				Index: 0,
				Message: openai.Message{
					Role:    "assistant",
					Content: &verdict,
				},
				FinishReason: "stop",
			},
		},
		Usage: openai.Usage{PromptTokens: 50, CompletionTokens: 20, TotalTokens: 70},
	}
	writeJSON(w, http.StatusOK, resp)
}

func extractLastUserContent(messages []openai.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && messages[i].Content != nil {
			return *messages[i].Content
		}
	}
	return ""
}

func matchInjectionPattern(text string) (bool, string) {
	lower := strings.ToLower(text)
	for _, pattern := range injectionPatterns {
		if strings.Contains(lower, pattern) {
			return true, pattern
		}
	}
	return false, ""
}
