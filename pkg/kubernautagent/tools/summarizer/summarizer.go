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

package summarizer

import (
	"context"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// Summarizer uses a secondary LLM call to shorten tool output that exceeds
// a configurable threshold. Per DD-HAPI-019-002: llm_summarize transformer.
type Summarizer struct {
	llmClient llm.Client
	threshold int
}

// New creates a summarizer with the given LLM client and threshold.
func New(client llm.Client, threshold int) *Summarizer {
	return &Summarizer{
		llmClient: client,
		threshold: threshold,
	}
}

// MaybeSummarize returns the input unchanged if it is at or below the threshold.
// If the input exceeds the threshold, it makes a secondary LLM call to produce
// a shorter summary preserving key details for incident investigation.
func (s *Summarizer) MaybeSummarize(ctx context.Context, toolName string, result string) (string, error) {
	if len(result) <= s.threshold {
		return result, nil
	}

	prompt := fmt.Sprintf(
		"Summarize the following %s output, preserving key details for incident investigation:\n\n%s",
		toolName, result,
	)

	resp, err := s.llmClient.Chat(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Options: llm.ChatOptions{
			Temperature: 0.0,
		},
	})
	if err != nil {
		return "", fmt.Errorf("summarization LLM call: %w", err)
	}

	return resp.Message.Content, nil
}
