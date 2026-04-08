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
// Supports two dialog formats:
//   - kaDialog: KA v1.3+ native format with rawResponses (full LLM JSON)
//   - hapiDialog: HAPI v1.2 format with aiMessages (backward compatibility)
//
// When kaDialog.rawResponses is populated, the last entry becomes ExactAnalysisText.
// When only hapiDialog is present, ExactAnalysisText is synthesized from the
// analysis fields, converting camelCase legacy (v1.2) format to the snake_case JSON
// that KA's ResultParser expects.
type GoldenTranscript struct {
	Scenario         string       `json:"scenario"`
	SignalName       string       `json:"signalName"`
	KubernautVersion string       `json:"kubernautVersion"`
	CapturedAt       string       `json:"capturedAt"`
	Analysis         HAPIAnalysis `json:"analysis"`
	KADialog         struct {
		RawResponses []string `json:"rawResponses"`
		ToolCalls    []struct {
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
			Result    json.RawMessage `json:"result"`
		} `json:"toolCalls"`
		LLMModel     string `json:"llmModel"`
		LLMCallCount int    `json:"llmCallCount"`
	} `json:"kaDialog"`
	HAPIDialog *HAPIDialog `json:"hapiDialog,omitempty"`
}

// HAPI v1.2 backward compatibility types.
// Golden transcripts were captured from the Python HAPI service (v1.2).
// These types and the "hapiDialog" JSON key must be preserved to
// deserialize existing golden transcript files.

// HAPIDialog holds the HAPI v1.2 conversation format captured from pod logs.
type HAPIDialog struct {
	ToolCalls []struct {
		Index       int     `json:"index"`
		Tool        string  `json:"tool"`
		Description string  `json:"description"`
		DurationSec float64 `json:"durationSec"`
		OutputChars int     `json:"outputChars"`
	} `json:"toolCalls"`
	AIMessages   []string `json:"aiMessages"`
	LLMModel     string   `json:"llmModel"`
	LLMCallCount int      `json:"llmCallCount"`
}

// HAPIAnalysis is a superset that captures both KA-native fields (json.RawMessage)
// and legacy v1.2 typed analysis fields for backward-compatible synthesis.
type HAPIAnalysis struct {
	Signal            json.RawMessage `json:"signal,omitempty"`
	RootCauseAnalysis json.RawMessage `json:"rootCauseAnalysis"`
	SelectedWorkflow  json.RawMessage `json:"selectedWorkflow,omitempty"`
	AlternativeWFs    json.RawMessage `json:"alternativeWorkflows,omitempty"`
	NeedsHumanReview  *bool           `json:"needsHumanReview,omitempty"`
	Actionability     string          `json:"actionability,omitempty"`
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
		} else if t.HAPIDialog != nil {
			synthesized, synthErr := synthesizeFromHAPI(t.Analysis)
			if synthErr != nil {
				errs = append(errs, fmt.Errorf("synthesizing %s: %w", path, synthErr))
				continue
			}
			exactText = synthesized
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

// synthesizeFromHAPI converts legacy camelCase analysis fields to the snake_case
// JSON format that KA's ResultParser expects. This bridges the gap between
// v1.2 golden transcripts and KA's structured output contract.
func synthesizeFromHAPI(analysis HAPIAnalysis) (string, error) {
	result := make(map[string]interface{})

	if len(analysis.RootCauseAnalysis) > 0 {
		var hapiRCA map[string]interface{}
		if err := json.Unmarshal(analysis.RootCauseAnalysis, &hapiRCA); err == nil {
			snakeRCA := camelToSnakeMap(hapiRCA)
			result["root_cause_analysis"] = snakeRCA
		}
	}

	if len(analysis.SelectedWorkflow) > 0 {
		var hapiWF map[string]interface{}
		if err := json.Unmarshal(analysis.SelectedWorkflow, &hapiWF); err == nil {
			snakeWF := camelToSnakeMap(hapiWF)
			result["selected_workflow"] = snakeWF
			if conf, ok := snakeWF["confidence"]; ok {
				result["confidence"] = conf
			}
		}
	}

	if len(analysis.AlternativeWFs) > 0 {
		var hapiAlts []map[string]interface{}
		if err := json.Unmarshal(analysis.AlternativeWFs, &hapiAlts); err == nil {
			snakeAlts := make([]map[string]interface{}, len(hapiAlts))
			for i, alt := range hapiAlts {
				snakeAlts[i] = camelToSnakeMap(alt)
			}
			result["alternative_workflows"] = snakeAlts
		}
	}

	if analysis.NeedsHumanReview != nil {
		result["needs_human_review"] = *analysis.NeedsHumanReview
	}

	if analysis.Actionability != "" {
		result["investigation_outcome"] = strings.ToLower(analysis.Actionability)
	}

	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshaling synthesized response: %w", err)
	}
	return string(data), nil
}

// camelToSnakeMap converts known camelCase keys to snake_case for KA parser
// compatibility. Only converts keys that appear in the LLM response schema;
// unknown keys are passed through unchanged.
func camelToSnakeMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		snakeKey := camelToSnake(k)
		switch val := v.(type) {
		case map[string]interface{}:
			out[snakeKey] = camelToSnakeMap(val)
		default:
			out[snakeKey] = v
			_ = val
		}
	}
	return out
}

var camelSnakeOverrides = map[string]string{
	"workflowId":          "workflow_id",
	"executionBundle":     "execution_bundle",
	"executionEngine":     "execution_engine",
	"serviceAccountName":  "service_account_name",
	"remediationTarget":   "remediation_target",
	"contributingFactors": "contributing_factors",
	"signalName":          "signal_name",
	"needsHumanReview":    "needs_human_review",
	"humanReviewReason":   "human_review_reason",
	"rootCauseAnalysis":   "root_cause_analysis",
	"selectedWorkflow":    "selected_workflow",
}

func camelToSnake(s string) string {
	if override, ok := camelSnakeOverrides[s]; ok {
		return override
	}
	return s
}
