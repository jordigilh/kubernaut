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

import "encoding/json"

// InvestigationResultSchema returns the JSON Schema that describes the expected
// LLM response format for incident investigation. Used by the structured output
// transport to enforce response shape via Anthropic's output_config.format.
//
// The schema mirrors the nested LLM response format (llmResponse) that
// parseLLMFormat expects, ensuring constrained decoding produces output the
// parser can handle without fallback extraction.
func InvestigationResultSchema() json.RawMessage {
	return json.RawMessage(investigationResultSchemaJSON)
}

// RCAResultSchema returns the JSON Schema for the RCA-only phase (Phase 1).
// It excludes workflow selection and escalation fields that belong in Phase 3.
// Aligned with HAPI v1.2.1 PHASE1_SECTIONS: root_cause_analysis, confidence,
// investigation_outcome, actionable, severity, detected_labels.
func RCAResultSchema() json.RawMessage {
	return json.RawMessage(rcaResultSchemaJSON)
}

// WithWorkflowResultSchema returns the JSON Schema for the submit_result_with_workflow
// tool (#760 v2). Used when the LLM selects a workflow during the workflow discovery phase.
// Reuses the investigationResultSchemaJSON which already includes selected_workflow.
func WithWorkflowResultSchema() json.RawMessage {
	return json.RawMessage(investigationResultSchemaJSON)
}

// NoWorkflowResultSchema returns the JSON Schema for the submit_result_no_workflow
// tool (#760 v2). Used when the LLM determines no matching workflow exists.
// Excludes selected_workflow and alternative_workflows; includes reasoning.
func NoWorkflowResultSchema() json.RawMessage {
	return json.RawMessage(noWorkflowResultSchemaJSON)
}

const noWorkflowResultSchemaJSON = `{
  "type": "object",
  "properties": {
    "root_cause_analysis": {
      "type": "object",
      "properties": {
        "summary": { "type": "string" },
        "severity": { "type": "string", "enum": ["critical", "high", "medium", "low", "info", "unknown"] },
        "signal_name": { "type": "string" },
        "contributing_factors": { "type": "array", "items": { "type": "string" } },
        "remediation_target": {
          "type": "object",
          "properties": {
            "kind": { "type": "string" },
            "name": { "type": "string" },
            "namespace": { "type": "string" }
          }
        }
      },
      "required": ["summary"]
    },
    "reasoning": { "type": "string", "description": "Explanation of why no workflow matches the incident" },
    "severity": { "type": "string", "enum": ["critical", "high", "medium", "low", "info", "unknown"] },
    "confidence": { "type": "number", "minimum": 0, "maximum": 1 },
    "investigation_outcome": { "type": "string", "enum": ["actionable", "not_actionable", "problem_resolved", "insufficient_data", "inconclusive"] }
  },
  "required": ["root_cause_analysis"]
}`

const rcaResultSchemaJSON = `{
  "type": "object",
  "properties": {
    "root_cause_analysis": {
      "type": "object",
      "properties": {
        "summary": { "type": "string" },
        "severity": { "type": "string", "enum": ["critical", "high", "medium", "low", "info", "unknown"] },
        "signal_name": { "type": "string" },
        "contributing_factors": { "type": "array", "items": { "type": "string" } },
        "remediation_target": {
          "type": "object",
          "properties": {
            "kind": { "type": "string" },
            "name": { "type": "string" },
            "namespace": { "type": "string" }
          }
        },
        "investigation_analysis": { "type": "string", "description": "Concise narrative summary of the investigation findings and reasoning (< 500 words). This field is consumed by the Phase 3 workflow selection LLM to provide investigation context." }
      },
      "required": ["summary"]
    },
    "severity": { "type": "string", "enum": ["critical", "high", "medium", "low", "info", "unknown"] },
    "confidence": { "type": "number", "minimum": 0, "maximum": 1 },
    "investigation_outcome": { "type": "string", "enum": ["actionable", "not_actionable", "problem_resolved", "insufficient_data", "inconclusive"] },
    "actionable": { "type": "boolean" },
    "detected_labels": { "type": "object" }
  },
  "required": ["root_cause_analysis", "confidence"]
}`

const investigationResultSchemaJSON = `{
  "type": "object",
  "properties": {
    "root_cause_analysis": {
      "type": "object",
      "properties": {
        "summary": { "type": "string" },
        "severity": { "type": "string", "enum": ["critical", "high", "medium", "low", "info", "unknown"] },
        "signal_name": { "type": "string" },
        "contributing_factors": { "type": "array", "items": { "type": "string" } },
        "remediation_target": {
          "type": "object",
          "properties": {
            "kind": { "type": "string" },
            "name": { "type": "string" },
            "namespace": { "type": "string" }
          }
        }
      },
      "required": ["summary"]
    },
    "selected_workflow": {
      "type": "object",
      "properties": {
        "workflow_id": { "type": "string" },
        "confidence": { "type": "number", "minimum": 0, "maximum": 1 },
        "rationale": { "type": "string" },
        "parameters": { "type": "object" },
        "execution_engine": { "type": "string" },
        "execution_bundle": { "type": "string" }
      },
      "required": ["workflow_id", "confidence"]
    },
    "alternative_workflows": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "workflow_id": { "type": "string" },
          "confidence": { "type": "number" },
          "rationale": { "type": "string" }
        },
        "required": ["workflow_id", "confidence"]
      }
    },
    "severity": { "type": "string", "enum": ["critical", "high", "medium", "low", "info", "unknown"] },
    "confidence": { "type": "number", "minimum": 0, "maximum": 1 },
    "investigation_outcome": { "type": "string", "enum": ["actionable", "not_actionable", "problem_resolved", "insufficient_data", "inconclusive"] },
    "actionable": { "type": "boolean" },
    "detected_labels": { "type": "object" }
  },
  "required": ["root_cause_analysis", "confidence"]
}`
