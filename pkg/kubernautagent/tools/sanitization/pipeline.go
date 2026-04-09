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

package sanitization

import (
	"context"
	"fmt"
)

// Stage represents a single sanitization stage (e.g., G4, I1).
type Stage interface {
	Name() string
	Sanitize(ctx context.Context, input string) (string, error)
}

// Pipeline chains sanitization stages in order (G4 → I1 per DD-HAPI-019-003).
type Pipeline struct {
	stages []Stage
}

// NewPipeline creates a sanitization pipeline with the given stages.
func NewPipeline(stages ...Stage) *Pipeline {
	return &Pipeline{stages: stages}
}

// Run executes all stages in order, returning the sanitized output.
func (p *Pipeline) Run(ctx context.Context, input string) (string, error) {
	current := input
	for _, stage := range p.stages {
		result, err := stage.Sanitize(ctx, current)
		if err != nil {
			return "", fmt.Errorf("sanitization stage %s: %w", stage.Name(), err)
		}
		current = result
	}
	return current, nil
}

// StageNames returns the ordered list of stage names in this pipeline.
func (p *Pipeline) StageNames() []string {
	names := make([]string, len(p.stages))
	for i, s := range p.stages {
		names[i] = s.Name()
	}
	return names
}
