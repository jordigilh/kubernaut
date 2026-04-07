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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

// replayConfidence is set higher than keyword scenarios (1.0) so that
// golden transcript replay always wins when a matching transcript exists.
const replayConfidence = 1.1

// GoldenTranscript is the JSON schema for files in MOCK_LLM_GOLDEN_DIR.
// See Phase 5b in the implementation plan for the full specification.
type GoldenTranscript struct {
	Scenario         string `json:"scenario"`
	SignalName       string `json:"signalName"`
	KubernautVersion string `json:"kubernautVersion"`
	CapturedAt       string `json:"capturedAt"`
	Analysis         struct {
		Signal            json.RawMessage `json:"signal"`
		RootCauseAnalysis json.RawMessage `json:"rootCauseAnalysis"`
		SelectedWorkflow  json.RawMessage `json:"selectedWorkflow"`
	} `json:"analysis"`
	KADialog struct {
		RawResponses []string `json:"rawResponses"`
		ToolCalls    []struct {
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
			Result    json.RawMessage `json:"result"`
		} `json:"toolCalls"`
		LLMModel     string `json:"llmModel"`
		LLMCallCount int    `json:"llmCallCount"`
	} `json:"kaDialog"`
}

// replayScenario matches on signalName from a golden transcript and returns
// the verbatim LLM response for full-fidelity replay.
type replayScenario struct {
	transcript GoldenTranscript
	config     MockScenarioConfig
}

func (s *replayScenario) Name() string {
	return "replay:" + s.transcript.Scenario
}

func (s *replayScenario) Match(ctx *DetectionContext) (bool, float64) {
	signal := extractSignal(ctx)
	if signal == "" {
		return false, 0
	}
	if strings.EqualFold(signal, s.transcript.SignalName) {
		return true, replayConfidence
	}
	return false, 0
}

func (s *replayScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{
		Name:        "replay:" + s.transcript.Scenario,
		Description: fmt.Sprintf("Golden transcript replay for %s (captured %s)", s.transcript.SignalName, s.transcript.CapturedAt),
	}
}

func (s *replayScenario) DAG() *conversation.DAG { return nil }

func (s *replayScenario) Config() MockScenarioConfig { return s.config }

// LoadReplayScenarios reads all .json files from goldenDir and returns
// a replay scenario for each valid golden transcript. Files that fail to
// parse are skipped with an error returned in the second slice.
// Returns nil slices when goldenDir is empty.
func LoadReplayScenarios(goldenDir string) ([]*replayScenario, []error) {
	if goldenDir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		return nil, []error{fmt.Errorf("reading golden dir %s: %w", goldenDir, err)}
	}

	var scenarios []*replayScenario
	var errs []error

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(goldenDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("reading %s: %w", path, err))
			continue
		}

		var t GoldenTranscript
		if err := json.Unmarshal(data, &t); err != nil {
			errs = append(errs, fmt.Errorf("parsing %s: %w", path, err))
			continue
		}

		if t.SignalName == "" {
			errs = append(errs, fmt.Errorf("skipping %s: missing signalName", path))
			continue
		}

		exactText := ""
		if len(t.KADialog.RawResponses) > 0 {
			exactText = t.KADialog.RawResponses[len(t.KADialog.RawResponses)-1]
		}

		cfg := MockScenarioConfig{
			ScenarioName:      "replay:" + t.Scenario,
			SignalName:        t.SignalName,
			ExactAnalysisText: exactText,
		}

		scenarios = append(scenarios, &replayScenario{
			transcript: t,
			config:     cfg,
		})
	}

	return scenarios, errs
}
