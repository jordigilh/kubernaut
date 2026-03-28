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
package tracker

import (
	"sync"
	"time"
)

// ToolCallRecord represents a recorded tool call invocation.
type ToolCallRecord struct {
	Name      string    `json:"name"`
	Arguments string    `json:"arguments"`
	Timestamp time.Time `json:"timestamp"`
}

// Tracker records tool calls, detected scenarios, and DAG paths for
// the verification API to expose.
type Tracker struct {
	mu            sync.RWMutex
	toolCalls     []ToolCallRecord
	lastScenario  string
	dagPath       []string
	requestCount  int
}

// New creates an empty Tracker.
func New() *Tracker {
	return &Tracker{}
}

// RecordToolCall logs a tool call invocation.
func (t *Tracker) RecordToolCall(name, arguments string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.toolCalls = append(t.toolCalls, ToolCallRecord{
		Name:      name,
		Arguments: arguments,
		Timestamp: time.Now(),
	})
}

// RecordScenario stores the detected scenario name.
func (t *Tracker) RecordScenario(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastScenario = name
}

// RecordDAGPath stores the DAG traversal path.
func (t *Tracker) RecordDAGPath(path []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.dagPath = append([]string{}, path...)
}

// IncrementRequestCount increments the request counter.
func (t *Tracker) IncrementRequestCount() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requestCount++
}

// GetToolCalls returns a copy of all recorded tool calls.
func (t *Tracker) GetToolCalls() []ToolCallRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]ToolCallRecord, len(t.toolCalls))
	copy(result, t.toolCalls)
	return result
}

// GetLastScenario returns the last detected scenario name.
func (t *Tracker) GetLastScenario() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastScenario
}

// GetDAGPath returns the last DAG traversal path.
func (t *Tracker) GetDAGPath() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]string, len(t.dagPath))
	copy(result, t.dagPath)
	return result
}

// GetRequestCount returns the total request count.
func (t *Tracker) GetRequestCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.requestCount
}

// Reset clears all tracked state.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.toolCalls = nil
	t.lastScenario = ""
	t.dagPath = nil
	t.requestCount = 0
}
