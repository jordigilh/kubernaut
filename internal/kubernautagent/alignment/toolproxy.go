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

	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
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

// Execute delegates to the inner registry, then submits the result
// for alignment evaluation via the context-scoped Observer.
func (p *ToolProxy) Execute(ctx context.Context, name string, args json.RawMessage) (string, error) {
	result, err := p.inner.Execute(ctx, name, args)
	if err != nil {
		return result, err
	}

	if obs := ObserverFromContext(ctx); obs != nil && result != "" {
		step := Step{
			Index:   obs.NextStepIndex(),
			Kind:    StepKindToolResult,
			Tool:    name,
			Content: result,
		}
		obs.SubmitAsync(ctx, step)
	}

	return result, nil
}

// ToolsForPhase delegates directly to the inner registry.
func (p *ToolProxy) ToolsForPhase(phase katypes.Phase, phaseTools katypes.PhaseToolMap) []tools.Tool {
	return p.inner.ToolsForPhase(phase, phaseTools)
}

// All delegates directly to the inner registry.
func (p *ToolProxy) All() []tools.Tool {
	return p.inner.All()
}
