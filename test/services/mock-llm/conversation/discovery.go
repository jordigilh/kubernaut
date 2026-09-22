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
package conversation

import (
	"encoding/json"
	"strings"
)

// DiscoveryEventKind identifies a provider-neutral conversation event.
type DiscoveryEventKind string

const (
	DiscoverySystemContentEvent    DiscoveryEventKind = "system_content"
	DiscoveryUserContentEvent      DiscoveryEventKind = "user_content"
	DiscoveryAssistantContentEvent DiscoveryEventKind = "assistant_content"
	DiscoveryToolCallEvent         DiscoveryEventKind = "tool_call"
	DiscoveryToolResultEvent       DiscoveryEventKind = "tool_result"
)

// DiscoveryEvent is the provider-neutral representation of conversation content,
// tool calls, and tool results. Payload contains text or serialized tool data.
type DiscoveryEvent struct {
	Kind       DiscoveryEventKind
	ToolName   string
	ToolCallID string
	Payload    string
}

// DiscoveryTranscript contains only the request data needed by the discovery planner.
// It is reconstructed from each request and never stored by the server.
type DiscoveryTranscript struct {
	AdvertisedTools []string
	Events          []DiscoveryEvent
}

// DiscoveryPlannerInput configures one stateless discovery decision.
type DiscoveryPlannerInput struct {
	Transcript         DiscoveryTranscript
	ExpectedWorkflowID string
	ActionType         string
	HasResourceContext bool
}

// DiscoveryPlanKind identifies the semantic response requested from a provider adapter.
type DiscoveryPlanKind string

const (
	DiscoveryCallTool   DiscoveryPlanKind = "call_tool"
	DiscoveryComplete   DiscoveryPlanKind = "complete"
	DiscoveryUnresolved DiscoveryPlanKind = "unresolved"
)

// DiscoveryPlan is the provider-neutral result of one discovery decision.
type DiscoveryPlan struct {
	Kind      DiscoveryPlanKind
	ToolName  string
	Arguments map[string]interface{}
	Reason    string
}

// PlanDiscovery advances DD-KA-017 from the complete request transcript.
// Membership is derived only from list_workflows results. The get_workflow result
// is intentionally read-only and cannot expand the membership set.
func PlanDiscovery(input DiscoveryPlannerInput) DiscoveryPlan {
	if hasToolResult(input.Transcript, "get_workflow") &&
		!workflowWasListed(input.Transcript, input.ExpectedWorkflowID) {
		return unresolvedDiscovery("get_workflow cannot establish discovery membership")
	}
	if !hasToolResult(input.Transcript, "list_available_actions") {
		if input.HasResourceContext && !hasToolResult(input.Transcript, "get_resource_context") {
			return callDiscoveryTool("get_resource_context", nil)
		}
		return callDiscoveryTool("list_available_actions", nil)
	}

	listResult, hasListResult := latestToolResult(input.Transcript, "list_workflows")
	if !hasListResult {
		return callDiscoveryTool("list_workflows", map[string]interface{}{
			"action_type": normalizedActionType(input.ActionType),
		})
	}

	if input.ExpectedWorkflowID == "" {
		return unresolvedDiscovery("workflow target is not configured")
	}

	membershipFound := workflowWasListed(input.Transcript, input.ExpectedWorkflowID)
	if membershipFound {
		if hasToolResult(input.Transcript, "get_workflow") {
			return DiscoveryPlan{Kind: DiscoveryComplete}
		}
		return callDiscoveryTool("get_workflow", map[string]interface{}{
			"workflow_id": input.ExpectedWorkflowID,
		})
	}

	result, valid := decodePlannerWorkflowDiscoveryResult(listResult.Payload)
	if !valid {
		return unresolvedDiscovery("invalid list_workflows result")
	}
	if result.Pagination.HasNext && result.Pagination.NextCursor != "" {
		return callDiscoveryTool("list_workflows", map[string]interface{}{
			"action_type": normalizedActionType(input.ActionType),
			"page":        "next",
			"cursor":      result.Pagination.NextCursor,
		})
	}
	return unresolvedDiscovery("workflow target was not returned by list_workflows")
}

func callDiscoveryTool(name string, args map[string]interface{}) DiscoveryPlan {
	return DiscoveryPlan{Kind: DiscoveryCallTool, ToolName: name, Arguments: args}
}

func unresolvedDiscovery(reason string) DiscoveryPlan {
	return DiscoveryPlan{Kind: DiscoveryUnresolved, Reason: reason}
}

func normalizedActionType(actionType string) string {
	if actionType == "" {
		return "remediation"
	}
	return actionType
}

func hasToolResult(transcript DiscoveryTranscript, name string) bool {
	_, found := latestToolResult(transcript, name)
	return found
}

func latestToolResult(transcript DiscoveryTranscript, name string) (DiscoveryEvent, bool) {
	for i := len(transcript.Events) - 1; i >= 0; i-- {
		event := transcript.Events[i]
		if event.Kind == DiscoveryToolResultEvent && event.ToolName == name {
			return event, true
		}
	}
	return DiscoveryEvent{}, false
}

func workflowWasListed(transcript DiscoveryTranscript, expectedID string) bool {
	for _, event := range transcript.Events {
		if event.Kind != DiscoveryToolResultEvent || event.ToolName != "list_workflows" {
			continue
		}
		result, valid := decodePlannerWorkflowDiscoveryResult(event.Payload)
		if !valid {
			continue
		}
		for _, workflow := range result.Workflows {
			if workflow.WorkflowID == expectedID || workflow.WorkflowIDSnake == expectedID {
				return true
			}
		}
	}
	return false
}

type plannerWorkflowDiscoveryResult struct {
	Workflows  []plannerWorkflowDiscoveryEntry `json:"workflows"`
	Pagination plannerWorkflowDiscoveryPage    `json:"pagination"`
}

type plannerWorkflowDiscoveryEntry struct {
	WorkflowID      string `json:"workflowId"`
	WorkflowIDSnake string `json:"workflow_id"`
}

type plannerWorkflowDiscoveryPage struct {
	HasNext         bool   `json:"hasNext"`
	HasNextSnake    bool   `json:"has_next"`
	NextCursor      string `json:"nextCursor"`
	NextCursorSnake string `json:"next_cursor"`
}

func (p plannerWorkflowDiscoveryPage) hasNext() bool {
	return p.HasNext || p.HasNextSnake
}

func (p plannerWorkflowDiscoveryPage) cursor() string {
	if p.NextCursor != "" {
		return p.NextCursor
	}
	return p.NextCursorSnake
}

func decodePlannerWorkflowDiscoveryResult(payload string) (plannerWorkflowDiscoveryResult, bool) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return plannerWorkflowDiscoveryResult{}, false
	}
	if start := strings.Index(payload, "{"); start >= 0 {
		payload = payload[start:]
	}
	var result plannerWorkflowDiscoveryResult
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		return plannerWorkflowDiscoveryResult{}, false
	}
	if result.Pagination.hasNext() && result.Pagination.cursor() != "" {
		result.Pagination.HasNext = true
		result.Pagination.NextCursor = result.Pagination.cursor()
	}
	return result, true
}
