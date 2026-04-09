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

package alignment

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// EvaluatorConfig holds configuration for the per-step evaluator.
type EvaluatorConfig struct {
	Timeout       time.Duration
	MaxStepTokens int
	MaxRetries    int
}

// Evaluator sends individual steps to the shadow LLM for alignment checking.
type Evaluator struct {
	client llm.Client
	config EvaluatorConfig
	prompt string
}

type evalResponse struct {
	Suspicious  bool   `json:"suspicious"`
	Explanation string `json:"explanation"`
}

// NewEvaluator creates an Evaluator with the given shadow LLM client and config.
func NewEvaluator(client llm.Client, cfg EvaluatorConfig) *Evaluator {
	return &Evaluator{client: client, config: cfg}
}

// WithSystemPrompt sets the system prompt used for each evaluation call.
func (e *Evaluator) WithSystemPrompt(prompt string) *Evaluator {
	e.prompt = prompt
	return e
}

// EvaluateStep sends a step to the shadow LLM and returns an observation.
// Respects Timeout and MaxRetries. Content is truncated to MaxStepTokens runes.
func (e *Evaluator) EvaluateStep(ctx context.Context, step Step) Observation {
	content := truncateRunes(step.Content, e.config.MaxStepTokens)

	userMsg := fmt.Sprintf("Step %d [%s]", step.Index, step.Kind)
	if step.Tool != "" {
		userMsg += fmt.Sprintf(" tool=%s", step.Tool)
	}
	userMsg += fmt.Sprintf("\n\n%s", content)

	messages := []llm.Message{
		{Role: "user", Content: userMsg},
	}
	if e.prompt != "" {
		messages = append([]llm.Message{{Role: "system", Content: e.prompt}}, messages...)
	}

	req := llm.ChatRequest{
		Messages: messages,
		Options:  llm.ChatOptions{JSONMode: true},
	}

	var lastErr error
	attempts := e.config.MaxRetries
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 0; attempt < attempts; attempt++ {
		evalCtx, cancel := context.WithTimeout(ctx, e.config.Timeout)
		resp, err := e.client.Chat(evalCtx, req)
		cancel()

		if err != nil {
			lastErr = err
			continue
		}

		var parsed evalResponse
		if jsonErr := json.Unmarshal([]byte(resp.Message.Content), &parsed); jsonErr != nil {
			lastErr = fmt.Errorf("parse evaluator response: %w", jsonErr)
			continue
		}

		return Observation{
			Step:        step,
			Suspicious:  parsed.Suspicious,
			Explanation: parsed.Explanation,
		}
	}

	return Observation{
		Step:        step,
		Suspicious:  false,
		Explanation: fmt.Sprintf("evaluator_unavailable: %v", lastErr),
	}
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
