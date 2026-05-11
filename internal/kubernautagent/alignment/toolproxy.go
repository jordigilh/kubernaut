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

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

// ToolProxy wraps a registry.ToolRegistry, intercepting Execute calls
// to feed tool results to the shadow agent while delegating read-only
// methods (ToolsForPhase, All) directly. The proxy is safe for concurrent
// use because the Observer is retrieved per-call from the context.
type ToolProxy struct {
	inner registry.ToolRegistry
}

var _ registry.ToolRegistry = (*ToolProxy)(nil)

// NewToolProxy creates a ToolProxy that decorates the given registry.
func NewToolProxy(inner registry.ToolRegistry) *ToolProxy {
	return &ToolProxy{inner: inner}
}

// Execute delegates to the inner registry. Shadow observation is handled
// post-pipeline by the investigator (SubmitToolStep) so the shadow agent
// evaluates the same sanitized, truncated output that the primary LLM sees.
func (p *ToolProxy) Execute(ctx context.Context, name string, args json.RawMessage) (string, error) {
	return p.inner.Execute(ctx, name, args)
}

// SubmitToolStep sends a post-pipeline tool result to the shadow agent for
// alignment evaluation. Called by the investigator after sanitize/summarize/
// truncate so the shadow evaluates the same content the primary LLM sees.
func SubmitToolStep(ctx context.Context, name, content string) {
	obs := ObserverFromContext(ctx)
	if obs == nil || content == "" {
		return
	}
	step := Step{
		Index:   obs.NextStepIndex(),
		Kind:    StepKindToolResult,
		Tool:    name,
		Content: content,
	}
	obs.SubmitAsync(ctx, step)
}

// NotifyRCAComplete triggers the full-context grounding review (#1096).
// Called by the investigator after the RCA loop completes, passing the
// full conversation for grounding analysis. Safe to call when alignment
// is disabled or the observer is absent (no-op in both cases).
func NotifyRCAComplete(ctx context.Context, messages []llm.Message) {
	obs := ObserverFromContext(ctx)
	if obs == nil {
		return
	}
	obs.StartGroundingReview(messages)
}

// ToolsForPhase delegates directly to the inner registry.
func (p *ToolProxy) ToolsForPhase(phase katypes.Phase, phaseTools katypes.PhaseToolMap) []tools.Tool {
	return p.inner.ToolsForPhase(phase, phaseTools)
}

// All delegates directly to the inner registry.
func (p *ToolProxy) All() []tools.Tool {
	return p.inner.All()
}
