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

package parser

import (
	"encoding/json"
	"fmt"
	"strings"

	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

// ResultParser extracts and validates InvestigationResult from LLM JSON output.
type ResultParser struct{}

// NewResultParser creates a new result parser.
func NewResultParser() *ResultParser {
	return &ResultParser{}
}

// Parse extracts InvestigationResult from raw LLM content.
// Handles both raw JSON and JSON embedded in markdown code blocks.
func (p *ResultParser) Parse(content string) (*katypes.InvestigationResult, error) {
	if content == "" {
		return nil, fmt.Errorf("empty JSON content")
	}

	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}

	var result katypes.InvestigationResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err == nil && (result.RCASummary != "" || result.WorkflowID != "") {
		return &result, nil
	}

	return parseLLMFormat(jsonStr)
}

// extractJSON finds JSON content, first trying to extract from a markdown
// code block, then falling back to the raw string.
func extractJSON(content string) string {
	if idx := strings.Index(content, "```json"); idx != -1 {
		start := idx + len("```json")
		end := strings.Index(content[start:], "```")
		if end != -1 {
			return strings.TrimSpace(content[start : start+end])
		}
	}
	if idx := strings.Index(content, "```"); idx != -1 {
		start := idx + len("```")
		end := strings.Index(content[start:], "```")
		if end != -1 {
			candidate := strings.TrimSpace(content[start : start+end])
			if len(candidate) > 0 && candidate[0] == '{' {
				return candidate
			}
		}
	}
	trimmed := strings.TrimSpace(content)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return trimmed
	}
	return ""
}

// llmResponse is the nested JSON structure that LLMs typically produce.
type llmResponse struct {
	RCA      *llmRCA      `json:"root_cause_analysis"`
	Workflow *llmWorkflow `json:"selected_workflow"`
}

type llmRCA struct {
	Summary string `json:"summary"`
}

type llmWorkflow struct {
	WorkflowID string  `json:"workflow_id"`
	Confidence float64 `json:"confidence"`
}

// parseLLMFormat parses the nested LLM response format and converts
// it to a flat InvestigationResult.
func parseLLMFormat(jsonStr string) (*katypes.InvestigationResult, error) {
	var resp llmResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing LLM JSON response: %w", err)
	}

	result := &katypes.InvestigationResult{}
	if resp.RCA != nil {
		result.RCASummary = resp.RCA.Summary
	}
	if resp.Workflow != nil {
		result.WorkflowID = resp.Workflow.WorkflowID
		result.Confidence = resp.Workflow.Confidence
	}

	if result.RCASummary == "" && result.WorkflowID == "" {
		return nil, fmt.Errorf("no recognized fields in LLM JSON response")
	}

	return result, nil
}
