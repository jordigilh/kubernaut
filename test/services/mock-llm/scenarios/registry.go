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
package scenarios

import (
	"sync"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

// Scenario defines the interface that all scenario implementations must satisfy.
type Scenario interface {
	Name() string
	Match(ctx *DetectionContext) (bool, float64)
	Metadata() ScenarioMetadata
	DAG() *conversation.DAG
}

// ScenarioMetadata describes a scenario for documentation and listing.
type ScenarioMetadata struct {
	Name          string
	Description   string
	WorkflowName  string
	Environment   string
	ActionType    string
	IsProactive   bool
	SignalPatterns []string
	Keywords      []string
}

// DetectionResult captures which scenario matched and how.
type DetectionResult struct {
	Scenario   Scenario
	Confidence float64
	Method     string
}

// Registry holds registered scenarios and provides detection.
type Registry struct {
	mu        sync.RWMutex
	scenarios []Scenario
	byName    map[string]Scenario
}

// NewRegistry creates an empty scenario registry.
func NewRegistry() *Registry {
	return &Registry{
		byName: make(map[string]Scenario),
	}
}

// Register adds a scenario to the registry.
func (r *Registry) Register(s Scenario) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scenarios = append(r.scenarios, s)
	r.byName[s.Name()] = s
}

// Get looks up a scenario by name.
func (r *Registry) Get(name string) (Scenario, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byName[name]
	return s, ok
}

// Detect evaluates all registered scenarios against the given context
// and returns the highest-confidence match. Returns nil if no scenario matches.
func (r *Registry) Detect(ctx *DetectionContext) *DetectionResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var best *DetectionResult

	for _, s := range r.scenarios {
		matched, confidence := s.Match(ctx)
		if !matched {
			continue
		}
		if best == nil || confidence > best.Confidence {
			best = &DetectionResult{
				Scenario:   s,
				Confidence: confidence,
			}
		}
	}
	return best
}

// List returns metadata for all registered scenarios.
func (r *Registry) List() []ScenarioMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ScenarioMetadata, len(r.scenarios))
	for i, s := range r.scenarios {
		result[i] = s.Metadata()
	}
	return result
}
