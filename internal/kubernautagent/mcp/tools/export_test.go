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

package tools

// BuildFinalResult exposes buildFinalResult for external test packages.
var BuildFinalResult = buildFinalResult

// IsWorkflowInDiscoveryResult exposes isWorkflowInDiscoveryResult for external test packages.
var IsWorkflowInDiscoveryResult = isWorkflowInDiscoveryResult

// ExtractDiscoveryResult exposes extractDiscoveryResult for external test packages.
var ExtractDiscoveryResult = extractDiscoveryResult

// GetReconstructedHistory exposes the reconHistory sync.Map for test assertions.
// Returns nil if no history is stored for the given rrID.
func (t *InvestigateTool) GetReconstructedHistory(rrID string) []LLMMessage {
	val, ok := t.reconHistory.Load(rrID)
	if !ok {
		return nil
	}
	msgs, ok := val.([]LLMMessage)
	if !ok {
		return nil
	}
	return msgs
}
