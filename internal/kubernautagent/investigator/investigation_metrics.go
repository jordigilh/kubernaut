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

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// InvestigationMetrics accumulates LLM interaction counts across all
// investigation phases (RCA + workflow selection). Unlike TokenAccumulator
// which tracks token usage for audit, this tracks call-level metrics
// for surfacing in the structured decision payload.
//
// Mutex-guarded: increments arrive from the sequential investigation loop,
// but interactive sessions may drive concurrent turns against one
// Investigator singleton, so shared scope entries must be race-safe.
type InvestigationMetrics struct {
	mu        sync.Mutex
	llmTurns  int
	toolCalls int
}

func NewInvestigationMetrics() *InvestigationMetrics {
	return &InvestigationMetrics{}
}

func (m *InvestigationMetrics) IncLLMTurns() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.llmTurns++
}

func (m *InvestigationMetrics) IncToolCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolCalls++
}

// AddToolCalls records a batch of dispatched tool calls at once. Used by
// processToolCalls, which fans dispatches out over an errgroup: counting the
// batch after g.Wait() avoids per-goroutine increments entirely.
func (m *InvestigationMetrics) AddToolCalls(n int) {
	if n <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolCalls += n
}

// Reset clears both counters so a fresh investigation starting under an
// already-seen correlationID never inherits a prior run's totals.
func (m *InvestigationMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.llmTurns = 0
	m.toolCalls = 0
}

func (m *InvestigationMetrics) LLMTurns() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.llmTurns
}

func (m *InvestigationMetrics) ToolCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.toolCalls
}

// metricsEntry tracks a per-investigation InvestigationMetrics alongside the
// last time it was accessed, so the TTL sweep can reclaim abandoned entries
// without depending on any investigation exit path firing (mirrors
// anomalyDetectorEntry, #1892).
type metricsEntry struct {
	metrics    *InvestigationMetrics
	lastAccess time.Time
}

// metricsScope holds one isolated InvestigationMetrics per investigation
// (keyed by correlationID). This mirrors anomalyScope: the Investigator is a
// singleton shared across concurrent investigations, so a single shared
// counter would corrupt every concurrent run. Call sites only need the
// correlationID they already carry — no signature threading required.
type metricsScope struct {
	mu       sync.Mutex
	entries  map[string]*metricsEntry
	fallback *InvestigationMetrics
}

func newMetricsScope() *metricsScope {
	return &metricsScope{
		entries:  make(map[string]*metricsEntry),
		fallback: NewInvestigationMetrics(),
	}
}

// metricsFor returns the InvestigationMetrics scoped to correlationID,
// creating one on first access. Safe for concurrent use. An empty
// correlationID falls back to a shared instance whose increments are
// effectively discarded (never applied to a result): failing open is safer
// than panicking, and production paths always carry signal.RemediationID.
func (inv *Investigator) metricsFor(correlationID string) *InvestigationMetrics {
	if inv.metricsScope == nil {
		inv.metricsScope = newMetricsScope()
	}
	if correlationID == "" {
		return inv.metricsScope.fallback
	}

	inv.metricsScope.mu.Lock()
	defer inv.metricsScope.mu.Unlock()

	if inv.metricsScope.entries == nil {
		inv.metricsScope.entries = make(map[string]*metricsEntry)
	}
	entry, ok := inv.metricsScope.entries[correlationID]
	if !ok {
		entry = &metricsEntry{metrics: NewInvestigationMetrics()}
		inv.metricsScope.entries[correlationID] = entry
	}
	entry.lastAccess = time.Now()
	return entry.metrics
}

// resetMetrics clears the scoped counters for correlationID. Called once per
// investigation entry point (Investigate, RunWorkflowDiscoveryFromRCA) —
// never between phases, so RCA + workflow-selection counts accumulate.
func (inv *Investigator) resetMetrics(correlationID string) {
	inv.metricsFor(correlationID).Reset()
}

// applyMetricsFromScope accumulates the scoped counts onto result.
// Additive (+=) as defense-in-depth; under the scope-source-of-truth
// invariant results never pre-carry counts (extraction included, which
// records into the scope), so this behaves as a set in practice.
// Nil-result safe.
func (inv *Investigator) applyMetricsFromScope(result *katypes.InvestigationResult, correlationID string) {
	if result == nil {
		return
	}
	m := inv.metricsFor(correlationID)
	result.TotalLLMTurns += m.LLMTurns()
	result.TotalToolCalls += m.ToolCalls()
}

// InvestigationTotals snapshots the cumulative per-RR accounting (turns,
// tools, tokens) for session assembly points outside this package
// (#2387 Gap 2: interactive select/complete flows). Cumulative across every
// leg of the remediation request; zeros when nothing was recorded.
func (inv *Investigator) InvestigationTotals(correlationID string) katypes.InvestigationTotals {
	m := inv.metricsFor(correlationID)
	prompt, completion, total := inv.tokenScopeTotals(correlationID)
	return katypes.InvestigationTotals{
		LLMTurns:         m.LLMTurns(),
		ToolCalls:        m.ToolCalls(),
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
	}
}

// ApplyTotals stamps a totals snapshot onto result with SET semantics: the
// per-RR scope is the single source of truth, so assembly points overwrite
// (never add). In particular sess.RCAResult may alias a stored autonomous
// result that already carries older totals — the scope is always a superset
// of those (same rrID, reset only at Investigate entry), so overwrite is
// both safe and required to avoid double-counting. Nil-result safe.
func ApplyTotals(result *katypes.InvestigationResult, totals katypes.InvestigationTotals) {
	if result == nil {
		return
	}
	result.TotalLLMTurns = totals.LLMTurns
	result.TotalToolCalls = totals.ToolCalls
	result.TokenUsage = &katypes.TokenUsageSummary{
		PromptTokens:     totals.PromptTokens,
		CompletionTokens: totals.CompletionTokens,
		TotalTokens:      totals.TotalTokens,
	}
}

// pruneMetrics removes entries idle longer than maxAge, returning the count
// removed. Called from the shared anomaly-cleanup tick so no new goroutine
// or cmd wiring is needed.
func (inv *Investigator) pruneMetrics(maxAge time.Duration) int {
	if inv.metricsScope == nil {
		return 0
	}
	cutoff := time.Now().Add(-maxAge)

	inv.metricsScope.mu.Lock()
	defer inv.metricsScope.mu.Unlock()

	removed := 0
	for id, entry := range inv.metricsScope.entries {
		if entry.lastAccess.Before(cutoff) {
			delete(inv.metricsScope.entries, id)
			removed++
		}
	}
	return removed
}
