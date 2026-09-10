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

package investigator

import (
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// tokenScopeEntry tracks per-investigation token usage alongside the last
// access time for TTL reclamation. Mirrors metricsEntry (#2387).
type tokenScopeEntry struct {
	promptTokens     int
	completionTokens int
	totalTokens      int
	lastAccess       time.Time
}

// tokenScope holds one cumulative token-usage accumulator per investigation
// (keyed by correlationID, i.e. the RemediationRequest name). Unlike
// TokenAccumulator — which is threaded through a single Investigate call for
// audit emission — this scope spans every leg of a remediation request:
// autonomous investigation, interactive turns, extraction, and discovery.
// Interactive and discovery legs have no TokenAccumulator threaded at all
// (LLMInvocationContext.Tokens is nil there), so without this scope their
// tokens would stay invisible to reporting. Raw provider counts only; never
// financial costs.
type tokenScope struct {
	mu      sync.Mutex
	entries map[string]*tokenScopeEntry
}

func newTokenScope() *tokenScope {
	return &tokenScope{entries: make(map[string]*tokenScopeEntry)}
}

// recordTokenUsage adds one LLM response's usage to the investigation's
// cumulative totals. Safe for concurrent use. An empty correlationID is
// discarded (never attributed): production paths always carry
// signal.RemediationID, and misattribution is worse than a gap.
func (inv *Investigator) recordTokenUsage(correlationID string, usage llm.TokenUsage) {
	if inv.tokenScope == nil {
		inv.tokenScope = newTokenScope()
	}
	if correlationID == "" {
		return
	}

	inv.tokenScope.mu.Lock()
	defer inv.tokenScope.mu.Unlock()

	if inv.tokenScope.entries == nil {
		inv.tokenScope.entries = make(map[string]*tokenScopeEntry)
	}
	entry, ok := inv.tokenScope.entries[correlationID]
	if !ok {
		entry = &tokenScopeEntry{}
		inv.tokenScope.entries[correlationID] = entry
	}
	entry.promptTokens += usage.PromptTokens
	entry.completionTokens += usage.CompletionTokens
	entry.totalTokens += usage.TotalTokens
	entry.lastAccess = time.Now()
}

// resetTokenScope clears the cumulative totals for correlationID. Called once
// per autonomous Investigate entry — never between legs — so every leg of
// one remediation request accumulates into a single cumulative total.
func (inv *Investigator) resetTokenScope(correlationID string) {
	if inv.tokenScope == nil || correlationID == "" {
		return
	}

	inv.tokenScope.mu.Lock()
	defer inv.tokenScope.mu.Unlock()

	if e, ok := inv.tokenScope.entries[correlationID]; ok {
		e.promptTokens, e.completionTokens, e.totalTokens = 0, 0, 0
		e.lastAccess = time.Now()
	}
}

// tokenScopeTotals snapshots the cumulative totals for correlationID.
// Zero values when nothing was recorded.
func (inv *Investigator) tokenScopeTotals(correlationID string) (prompt, completion, total int) {
	if inv.tokenScope == nil || correlationID == "" {
		return 0, 0, 0
	}

	inv.tokenScope.mu.Lock()
	defer inv.tokenScope.mu.Unlock()

	if e, ok := inv.tokenScope.entries[correlationID]; ok {
		return e.promptTokens, e.completionTokens, e.totalTokens
	}
	return 0, 0, 0
}

// setTokenUsageFromScope stamps the cumulative token totals onto result.
// Always sets (even zero) for a uniform reporting schema.
func (inv *Investigator) setTokenUsageFromScope(result *katypes.InvestigationResult, correlationID string) {
	if result == nil {
		return
	}
	prompt, completion, total := inv.tokenScopeTotals(correlationID)
	result.TokenUsage = &katypes.TokenUsageSummary{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
	}
}

// pruneTokenScope removes entries idle longer than maxAge, returning the
// count removed. Called from the shared anomaly-cleanup tick.
func (inv *Investigator) pruneTokenScope(maxAge time.Duration) int {
	if inv.tokenScope == nil {
		return 0
	}
	cutoff := time.Now().Add(-maxAge)

	inv.tokenScope.mu.Lock()
	defer inv.tokenScope.mu.Unlock()

	removed := 0
	for id, entry := range inv.tokenScope.entries {
		if entry.lastAccess.Before(cutoff) {
			delete(inv.tokenScope.entries, id)
			removed++
		}
	}
	return removed
}
