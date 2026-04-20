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
	"encoding/json"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// Summarizer uses a secondary LLM call to shorten tool output that exceeds
// a configurable threshold. Per DD-HAPI-019-002: llm_summarize transformer.
type Summarizer struct {
	llmClient    llm.Client
	threshold    int
	maxInputSize int
}

// New creates a summarizer with the given LLM client and threshold.
// maxInputSize defaults to 0 (no pre-truncation).
func New(client llm.Client, threshold int) *Summarizer {
	return &Summarizer{
		llmClient: client,
		threshold: threshold,
	}
}

// NewWithMaxInput creates a summarizer that pre-truncates inputs exceeding
// maxInputSize before sending them to the secondary LLM call. This prevents
// the summarizer's own LLM from hitting context window limits (#752).
func NewWithMaxInput(client llm.Client, threshold, maxInputSize int) *Summarizer {
	return &Summarizer{
		llmClient:    client,
		threshold:    threshold,
		maxInputSize: maxInputSize,
	}
}

// MaybeSummarize returns the input unchanged if it is at or below the threshold.
// If the input exceeds the threshold, it makes a secondary LLM call to produce
// a shorter summary preserving key details for incident investigation.
// When maxInputSize > 0 and the result exceeds it, the input is pre-truncated
// before being sent to the LLM to prevent the summarizer itself from overflowing.
func (s *Summarizer) MaybeSummarize(ctx context.Context, toolName string, result string) (string, error) {
	if len(result) <= s.threshold {
		return result, nil
	}

	input := result
	preTruncated := false
	if s.maxInputSize > 0 && len(input) > s.maxInputSize {
		input = input[:s.maxInputSize]
		preTruncated = true
	}

	var prompt string
	if preTruncated {
		prompt = fmt.Sprintf(
			"Summarize the following %s output [PRE-TRUNCATED from %d to %d chars], preserving key details for incident investigation:\n\n%s",
			toolName, len(result), s.maxInputSize, input,
		)
	} else {
		prompt = fmt.Sprintf(
			"Summarize the following %s output, preserving key details for incident investigation:\n\n%s",
			toolName, input,
		)
	}

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

// TruncateToolOutput applies a hard character limit to tool output, appending
// a guidance hint when truncation occurs. This is the final safety net
// preventing oversized output from entering the LLM context window (#752).
func TruncateToolOutput(output, toolName string, limit int) string {
	if len(output) <= limit {
		return output
	}
	hint := fmt.Sprintf(
		"\n... [TRUNCATED] %s output was %d chars (limit: %d). Use kubectl_get_by_name or kubectl_get_by_name_in_cluster to fetch specific resources instead of listing all.",
		toolName, len(output), limit,
	)
	return output[:limit] + hint
}

// Wrap returns a tool that delegates to inner but applies MaybeSummarize
// to the output before returning it to the LLM.
func (s *Summarizer) Wrap(inner tools.Tool) tools.Tool {
	return &wrappedTool{inner: inner, summarizer: s}
}

type wrappedTool struct {
	inner      tools.Tool
	summarizer *Summarizer
}

func (w *wrappedTool) Name() string               { return w.inner.Name() }
func (w *wrappedTool) Description() string         { return w.inner.Description() }
func (w *wrappedTool) Parameters() json.RawMessage { return w.inner.Parameters() }

func (w *wrappedTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	result, err := w.inner.Execute(ctx, args)
	if err != nil {
		return "", err
	}
	summarized, sErr := w.summarizer.MaybeSummarize(ctx, w.inner.Name(), result)
	if sErr != nil {
		return result, nil
	}
	return summarized, nil
}
