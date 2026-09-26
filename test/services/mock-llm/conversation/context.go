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
	"regexp"
	"strings"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
)

// Context holds the state for a single conversation execution.
type Context struct {
	Messages    []openai.Message
	CurrentNode string
	Metadata    map[string]interface{}
}

const (
	workflowIDMetadataKey = "workflow_id"
	toolRole              = "tool"
)

// NewContext creates a new conversation context from the provided messages.
func NewContext(messages []openai.Message) *Context {
	return &Context{
		Messages: messages,
		Metadata: make(map[string]interface{}),
	}
}

// CountToolResults counts messages with role=="tool" in the conversation.
func (c *Context) CountToolResults() int {
	count := 0
	for _, m := range c.Messages {
		if m.Role == toolRole {
			count++
		}
	}
	return count
}

// SetWorkflowID records the scenario's expected workflow for discovery
// pagination decisions.
func (c *Context) SetWorkflowID(workflowID string) {
	if c.Metadata == nil {
		c.Metadata = make(map[string]interface{})
	}
	c.Metadata[workflowIDMetadataKey] = workflowID
}

func (c *Context) workflowID() string {
	workflowID, _ := c.Metadata[workflowIDMetadataKey].(string)
	return workflowID
}

// WorkflowDiscoveryTargetFound reports whether a list_workflows result in the
// conversation contains the workflow configured for the current scenario.
func (c *Context) WorkflowDiscoveryTargetFound() bool {
	workflowID := c.workflowID()
	if workflowID == "" {
		return false
	}
	for i, message := range c.Messages {
		if message.Role != toolRole || c.toolCallNameBefore(i) != openai.ToolListWorkflows || message.Content == nil {
			continue
		}
		result := decodeWorkflowDiscoveryResult(*message.Content)
		for _, workflow := range result.Workflows {
			if workflow.WorkflowID == workflowID {
				return true
			}
		}
	}
	return false
}

// WorkflowDiscoveryNextCursor returns the next cursor from the most recent
// list_workflows result when another page is available.
func (c *Context) WorkflowDiscoveryNextCursor() string {
	for i := len(c.Messages) - 1; i >= 0; i-- {
		message := c.Messages[i]
		if message.Role != toolRole || c.toolCallNameBefore(i) != openai.ToolListWorkflows || message.Content == nil {
			continue
		}
		result := decodeWorkflowDiscoveryResult(*message.Content)
		if result.Pagination.HasNext {
			return result.Pagination.NextCursor
		}
		return ""
	}
	return ""
}

// LastToolCallName returns the tool call associated with the latest tool
// result. It is used to distinguish a paginated discovery result from the
// get_workflow result that completes the discovery sequence.
func (c *Context) LastToolCallName() string {
	for i := len(c.Messages) - 1; i >= 0; i-- {
		if c.Messages[i].Role != toolRole {
			continue
		}
		return c.toolCallNameBefore(i)
	}
	return ""
}

func (c *Context) toolCallNameBefore(index int) string {
	toolCallID := c.Messages[index].ToolCallID
	if toolCallID != "" {
		for i := index - 1; i >= 0; i-- {
			message := c.Messages[i]
			if message.Role != "assistant" {
				continue
			}
			for _, toolCall := range message.ToolCalls {
				if toolCall.ID == toolCallID {
					return toolCall.Function.Name
				}
			}
		}
	}
	for i := index - 1; i >= 0; i-- {
		message := c.Messages[i]
		if message.Role != "assistant" || len(message.ToolCalls) == 0 {
			continue
		}
		return message.ToolCalls[len(message.ToolCalls)-1].Function.Name
	}
	return ""
}

type workflowDiscoveryResult struct {
	Workflows  []workflowDiscoveryEntry `json:"workflows"`
	Pagination workflowDiscoveryPage    `json:"pagination"`
}

type workflowDiscoveryEntry struct {
	WorkflowID string `json:"workflowId"`
}

type workflowDiscoveryPage struct {
	HasNext    bool   `json:"hasNext"`
	NextCursor string `json:"nextCursor"`
}

func decodeWorkflowDiscoveryResult(content string) workflowDiscoveryResult {
	var result workflowDiscoveryResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return workflowDiscoveryResult{}
	}
	return result
}

// Phase 3 markers that KA injects into the enriched prompt.
const (
	MarkerEnrichmentContext = "## Enrichment Context (Phase 2"
	MarkerPhase1RCA         = "## Phase 1 Root Cause Analysis"
	MarkerRootOwner         = "**Root Owner**:"
)

// HasPhase3Markers returns true only when ALL three Phase 3 markers are
// present in the combined message content.
func (c *Context) HasPhase3Markers() bool {
	combined := c.combinedContent()
	return strings.Contains(combined, MarkerEnrichmentContext) &&
		strings.Contains(combined, MarkerPhase1RCA) &&
		strings.Contains(combined, MarkerRootOwner)
}

// ResourceInfo holds resource details extracted from structured prompt content.
type ResourceInfo struct {
	SignalName string
	Namespace  string
	Name       string
	Kind       string
}

var (
	reSignalName      = regexp.MustCompile(`(?i)-\s*Signal Name:\s*(\S+)`)
	reNamespace       = regexp.MustCompile(`(?i)-\s*Namespace:\s*(\S+)`)
	rePod             = regexp.MustCompile(`(?i)-\s*Pod:\s*(\S+)`)
	reNode            = regexp.MustCompile(`(?i)-\s*Node:\s*(\S+)`)
	reResourceLine    = regexp.MustCompile(`(?i)-\s*Resource:\s*(\S+)/(\S+)/(\S+)`)
	reOwnerChain      = regexp.MustCompile(`\*\*Owner Chain\*\*:\s*(.+)`)
	reOwnerChainEntry = regexp.MustCompile(`^(\w+)/([^(]+?)(?:\(([^)]+)\))?$`)
)

// ExtractResource pulls resource name, namespace, and signal from
// structured prompt content, modelling what a real LLM would do:
// identify the root owner from the enrichment context.
//
// Priority:
//  1. **Owner Chain** — last entry is the root owner (e.g. Deployment/name).
//  2. - Resource: ns/kind/name — raw signal resource (fallback).
//  3. Individual "- Pod:" / "- Node:" / "- Namespace:" lines (legacy fallback).
func (c *Context) ExtractResource() ResourceInfo {
	combined := c.combinedContent()
	info := ResourceInfo{}

	if m := reSignalName.FindStringSubmatch(combined); len(m) > 1 {
		info.SignalName = m[1]
	}

	if root := extractRootOwnerFromChain(combined); root.Kind != "" {
		info.Kind = root.Kind
		info.Name = root.Name
		if root.Namespace != "" {
			info.Namespace = root.Namespace
		} else if m := reResourceLine.FindStringSubmatch(combined); len(m) > 3 {
			info.Namespace = m[1]
		}
		return info
	}

	if m := reResourceLine.FindStringSubmatch(combined); len(m) > 3 {
		info.Namespace = m[1]
		info.Kind = m[2]
		info.Name = m[3]
		return info
	}

	if m := reNamespace.FindStringSubmatch(combined); len(m) > 1 {
		info.Namespace = m[1]
	}
	if m := rePod.FindStringSubmatch(combined); len(m) > 1 {
		info.Name = m[1]
		info.Kind = "Pod"
	} else if m := reNode.FindStringSubmatch(combined); len(m) > 1 {
		info.Name = m[1]
		info.Kind = "Node"
	}
	return info
}

// extractRootOwnerFromChain parses "**Owner Chain**: Kind/Name(NS) → Kind/Name(NS)"
// and returns the last entry (root owner). Returns empty ResourceInfo on parse failure.
func extractRootOwnerFromChain(content string) ResourceInfo {
	m := reOwnerChain.FindStringSubmatch(content)
	if len(m) < 2 {
		return ResourceInfo{}
	}
	entries := strings.Split(m[1], " → ")
	if len(entries) == 0 {
		return ResourceInfo{}
	}
	last := strings.TrimSpace(entries[len(entries)-1])
	em := reOwnerChainEntry.FindStringSubmatch(last)
	if len(em) < 3 {
		return ResourceInfo{}
	}
	info := ResourceInfo{Kind: em[1], Name: em[2]}
	if len(em) > 3 {
		info.Namespace = em[3]
	}
	return info
}

// RootOwner holds the root owner information from get_resource_context results.
type RootOwner struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// ExtractRootOwner scans tool result messages for a JSON object containing
// a root_owner field. Tolerates prefix text before the JSON (e.g., HolmesGPT
// analysis preamble).
func (c *Context) ExtractRootOwner() *RootOwner {
	for _, m := range c.Messages {
		if m.Role != "tool" || m.Content == nil {
			continue
		}
		content := *m.Content
		idx := strings.Index(content, "{")
		if idx < 0 {
			continue
		}
		jsonStr := content[idx:]

		var wrapper struct {
			RootOwner *RootOwner `json:"root_owner"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
			continue
		}
		if wrapper.RootOwner != nil && wrapper.RootOwner.Kind != "" {
			return wrapper.RootOwner
		}
	}
	return nil
}

func (c *Context) combinedContent() string {
	var parts []string
	for _, m := range c.Messages {
		if m.Content != nil {
			parts = append(parts, *m.Content)
		}
	}
	return strings.Join(parts, " ")
}
