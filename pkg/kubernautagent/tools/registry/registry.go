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

package registry

import (
	"context"
	"encoding/json"

	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// ErrToolNotFound is returned when a tool name is not registered.
type ErrToolNotFound struct {
	Name string
}

func (e *ErrToolNotFound) Error() string {
	return "tool not found: " + e.Name
}

// ToolRegistry abstracts tool execution and lookup so that decorators
// (e.g. alignment.ToolProxy) can intercept calls transparently.
type ToolRegistry interface {
	Execute(ctx context.Context, name string, args json.RawMessage) (string, error)
	ToolsForPhase(phase katypes.Phase, phaseTools katypes.PhaseToolMap) []tools.Tool
	All() []tools.Tool
}

// Registry holds registered tools and resolves them by name and phase.
type Registry struct {
	byName map[string]tools.Tool
	order  []tools.Tool
}

// New creates a new empty tool registry.
func New() *Registry {
	return &Registry{
		byName: make(map[string]tools.Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool tools.Tool) {
	name := tool.Name()
	if _, exists := r.byName[name]; !exists {
		r.order = append(r.order, tool)
	}
	r.byName[name] = tool
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (tools.Tool, error) {
	t, ok := r.byName[name]
	if !ok {
		return nil, &ErrToolNotFound{Name: name}
	}
	return t, nil
}

// All returns all registered tools in registration order.
func (r *Registry) All() []tools.Tool {
	result := make([]tools.Tool, len(r.order))
	copy(result, r.order)
	return result
}

// ToolsForPhase returns tools available for the given phase.
func (r *Registry) ToolsForPhase(phase katypes.Phase, phaseTools katypes.PhaseToolMap) []tools.Tool {
	names, ok := phaseTools[phase]
	if !ok {
		return nil
	}
	var result []tools.Tool
	for _, name := range names {
		if t, exists := r.byName[name]; exists {
			result = append(result, t)
		}
	}
	return result
}

// Execute looks up a tool by name and executes it.
func (r *Registry) Execute(ctx context.Context, name string, args json.RawMessage) (string, error) {
	t, ok := r.byName[name]
	if !ok {
		return "", &ErrToolNotFound{Name: name}
	}
	return t.Execute(ctx, args)
}
