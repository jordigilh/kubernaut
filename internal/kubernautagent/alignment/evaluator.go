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
	"log/slog"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/security/boundary"
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
	logger *slog.Logger
}

type evalResponse struct {
	Suspicious  *bool  `json:"suspicious"`
	Explanation string `json:"explanation"`
}

// NewEvaluator creates an Evaluator with the given shadow LLM client, config,
// and system prompt. The prompt is immutable after construction.
// The optional logger enables per-step DEBUG-level logging for operator
// troubleshooting. Pass nil to disable step-level logging.
func NewEvaluator(client llm.Client, cfg EvaluatorConfig, prompt string, opts ...EvaluatorOption) *Evaluator {
	e := &Evaluator{client: client, config: cfg, prompt: prompt}
	for _, o := range opts {
		o(e)
	}
	return e
}

// EvaluatorOption configures optional Evaluator behavior.
type EvaluatorOption func(*Evaluator)

// WithLogger attaches a structured logger for per-step DEBUG output.
func WithLogger(l *slog.Logger) EvaluatorOption {
	return func(e *Evaluator) { e.logger = l }
}

// EvaluateStep sends a step to the shadow LLM and returns an observation.
// Fail-closed: all error paths return Suspicious=true. Content is truncated
// to MaxStepTokens runes using head+tail strategy.
func (e *Evaluator) EvaluateStep(ctx context.Context, step Step) Observation {
	if ctx.Err() != nil {
		return Observation{
			Step:        step,
			Suspicious:  true,
			Explanation: fmt.Sprintf("evaluator_unavailable (fail-closed): context cancelled: %v", ctx.Err()),
		}
	}

	rawContent := step.Content

	// Step 1: Generate boundary token
	token := boundary.Generate()

	// Step 2: Pre-scan raw content for escape attempt (fail-closed)
	if boundary.ContainsEscape(rawContent, token) {
		return Observation{
			Step:        step,
			Suspicious:  true,
			Explanation: "boundary escape detected in raw content (fail-closed): content contains closing boundary marker",
		}
	}

	// Step 3: Truncate
	content := truncateHeadTail(rawContent, e.config.MaxStepTokens)

	// Step 4: Wrap in boundary
	wrapped := boundary.Wrap(content, token)

	userMsg := fmt.Sprintf("Step %d [%s]", step.Index, step.Kind)
	if step.Tool != "" {
		userMsg += fmt.Sprintf(" tool=%s", step.Tool)
	}
	userMsg += fmt.Sprintf("\n\n%s", wrapped)

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

		respContent := resp.Message.Content
		if hasDuplicateSuspiciousKey(respContent) {
			return Observation{
				Step:        step,
				Suspicious:  true,
				Explanation: "duplicate_key_attack (fail-closed): shadow LLM response contains duplicate 'suspicious' key",
			}
		}

		var parsed evalResponse
		if jsonErr := json.Unmarshal([]byte(respContent), &parsed); jsonErr != nil {
			lastErr = fmt.Errorf("parse evaluator response: %w", jsonErr)
			continue
		}

		if parsed.Suspicious == nil {
			obs := Observation{
				Step:        step,
				Suspicious:  true,
				Explanation: "evaluator_unavailable (fail-closed): shadow LLM response missing 'suspicious' field",
			}
			e.debugLog("step evaluated: missing suspicious field",
				slog.Int("step_index", step.Index), slog.String("kind", string(step.Kind)),
				slog.Bool("suspicious", true))
			return obs
		}

		e.debugLog("step evaluated",
			slog.Int("step_index", step.Index), slog.String("kind", string(step.Kind)),
			slog.String("tool", step.Tool), slog.Bool("suspicious", *parsed.Suspicious),
			slog.Int("attempt", attempt+1))
		return Observation{
			Step:        step,
			Suspicious:  *parsed.Suspicious,
			Explanation: parsed.Explanation,
		}
	}

	return Observation{
		Step:        step,
		Suspicious:  true,
		Explanation: fmt.Sprintf("evaluator_unavailable (fail-closed): %v", lastErr),
	}
}

const truncationMarker = "…[truncated]…"

func truncateHeadTail(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	head := max / 2
	tail := max - head
	return string(runes[:head]) + truncationMarker + string(runes[len(runes)-tail:])
}

// TruncateHeadTail is the exported version for testing.
func TruncateHeadTail(s string, max int) string {
	return truncateHeadTail(s, max)
}

func (e *Evaluator) debugLog(msg string, attrs ...any) {
	if e.logger != nil {
		e.logger.Debug(msg, attrs...)
	}
}

// hasDuplicateSuspiciousKey performs a raw-byte pre-scan for duplicate
// "suspicious" keys in JSON. Go's json.Unmarshal uses last-key-wins
// semantics, so an attacker could send {"suspicious":true,"suspicious":false}
// to flip the verdict. This check returns true if more than one occurrence
// of the key is found.
func hasDuplicateSuspiciousKey(raw string) bool {
	return strings.Count(raw, `"suspicious"`) > 1
}
