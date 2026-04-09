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

package investigation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

var todoWriteParams = json.RawMessage(`{"type":"object","properties":{"todos":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"content":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","completed","cancelled"]}},"required":["id","content","status"]}}},"required":["todos"]}`)

// ToolName is the canonical name registered with the LLM.
const ToolName = "todo_write"

type todoItem struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

type todoWriteTool struct {
	mu    sync.Mutex
	items map[string]todoItem
}

// NewTodoWriteTool creates a TodoWrite tool with an empty in-memory task list.
// The tool persists state across calls within a single investigation.
func NewTodoWriteTool() tools.Tool {
	return &todoWriteTool{
		items: make(map[string]todoItem),
	}
}

func (t *todoWriteTool) Name() string               { return ToolName }
func (t *todoWriteTool) Description() string         { return "Manage an investigation task list. Create, update, and track investigation todos." }
func (t *todoWriteTool) Parameters() json.RawMessage { return todoWriteParams }

func (t *todoWriteTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Todos []todoItem `json:"todos"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("parsing todo_write args: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for _, item := range input.Todos {
		t.items[item.ID] = item
	}

	byStatus := make(map[string]int)
	var inProgress []todoItem
	for _, item := range t.items {
		byStatus[item.Status]++
		if item.Status == "in_progress" {
			inProgress = append(inProgress, item)
		}
	}

	summary := map[string]interface{}{
		"total":       len(t.items),
		"by_status":   byStatus,
		"in_progress": inProgress,
	}

	data, err := json.Marshal(summary)
	if err != nil {
		return "", fmt.Errorf("marshaling todo summary: %w", err)
	}
	return string(data), nil
}
