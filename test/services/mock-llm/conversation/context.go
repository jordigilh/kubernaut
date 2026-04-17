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
		if m.Role == "tool" {
			count++
		}
	}
	return count
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
	reSignalName   = regexp.MustCompile(`(?i)-\s*Signal Name:\s*(\S+)`)
	reNamespace    = regexp.MustCompile(`(?i)-\s*Namespace:\s*(\S+)`)
	rePod          = regexp.MustCompile(`(?i)-\s*Pod:\s*(\S+)`)
	reNode         = regexp.MustCompile(`(?i)-\s*Node:\s*(\S+)`)
	reResourceLine = regexp.MustCompile(`(?i)-\s*Resource:\s*(\S+)/(\S+)/(\S+)`)
)

// ExtractResource pulls resource name, namespace, and signal from
// structured "- Key: Value" lines in message content.
//
// Primary: parses "- Resource: ns/kind/name" (KA prompt template format).
// Fallback: individual "- Pod:" / "- Node:" / "- Namespace:" lines.
func (c *Context) ExtractResource() ResourceInfo {
	combined := c.combinedContent()
	info := ResourceInfo{}

	if m := reSignalName.FindStringSubmatch(combined); len(m) > 1 {
		info.SignalName = m[1]
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
