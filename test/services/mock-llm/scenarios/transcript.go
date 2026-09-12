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
	"strings"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

const transcriptConfidence = 1.05

type transcriptScenario struct {
	name  string
	steps []config.TranscriptStepOverride
}

func newTranscriptScenario(override config.TranscriptScenarioOverride) *transcriptScenario {
	return &transcriptScenario{name: override.Name, steps: override.Steps}
}

func (s *transcriptScenario) Name() string { return s.name }

func (s *transcriptScenario) Match(ctx *DetectionContext) (bool, float64) {
	if s.stepFor(ctx) == nil {
		return false, 0
	}
	return true, transcriptConfidence
}

func (s *transcriptScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{
		Name:        s.name,
		Description: "explicit A2A transcript scenario",
	}
}

func (s *transcriptScenario) DAG() *conversation.DAG { return nil }

// ConfigForContext returns only the tool call for the exact current user turn.
// The handler's existing repeat guard prevents the same tool from being
// emitted again while its result is being processed.
func (s *transcriptScenario) ConfigForContext(ctx *DetectionContext) MockScenarioConfig {
	step := s.stepFor(ctx)
	if step == nil {
		return MockScenarioConfig{}
	}
	return MockScenarioConfig{
		ScenarioName:      s.name,
		ToolCallName:      step.ToolCall.Name,
		ToolCallArgs:      cloneArguments(step.ToolCall.Arguments),
		FallbackArguments: cloneArguments(step.ToolCall.FallbackArguments),
		ForceText:         BoolPtr(false),
		RepeatToolCall:    true,
	}
}

func cloneArguments(arguments map[string]interface{}) map[string]interface{} {
	if arguments == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(arguments))
	for key, value := range arguments {
		cloned[key] = value
	}
	return cloned
}

func (s *transcriptScenario) stepFor(ctx *DetectionContext) *config.TranscriptStepOverride {
	if ctx == nil {
		return nil
	}
	target := strings.TrimSpace(ctx.LastUserContent)
	if target == "" {
		return nil
	}
	for i := range s.steps {
		if strings.EqualFold(strings.TrimSpace(s.steps[i].User), target) {
			return &s.steps[i]
		}
	}
	return nil
}
