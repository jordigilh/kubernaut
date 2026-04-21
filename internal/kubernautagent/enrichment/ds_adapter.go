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

package enrichment

import (
	"context"
	"fmt"
	"slices"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// DSAdapter implements DataStorageClient by wrapping the ogen-generated DS client.
type DSAdapter struct {
	client HistoryContextClient
}

// HistoryContextClient is a narrow interface for the ogen DS client method we use.
// Satisfied by *ogenclient.Client.
type HistoryContextClient interface {
	GetRemediationHistoryContext(
		ctx context.Context,
		params ogenclient.GetRemediationHistoryContextParams,
	) (ogenclient.GetRemediationHistoryContextRes, error)
}

var _ DataStorageClient = (*DSAdapter)(nil)

// NewDSAdapter creates a DSAdapter wrapping the given DS client.
func NewDSAdapter(client HistoryContextClient) *DSAdapter {
	return &DSAdapter{client: client}
}

// GetRemediationHistory queries DataStorage for remediation history and maps
// ogen-generated types to enrichment domain types.
func (a *DSAdapter) GetRemediationHistory(ctx context.Context, kind, name, namespace, specHash string) (*RemediationHistoryResult, error) {
	params := ogenclient.GetRemediationHistoryContextParams{
		TargetKind:      kind,
		TargetName:      name,
		TargetNamespace: namespace,
		CurrentSpecHash: specHash,
	}

	res, err := a.client.GetRemediationHistoryContext(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("ds adapter: history query for %s/%s/%s: %w", namespace, kind, name, err)
	}

	if _, isBadRequest := res.(*ogenclient.GetRemediationHistoryContextBadRequest); isBadRequest {
		return nil, fmt.Errorf("ds adapter: bad request for %s/%s/%s (spec_hash=%s)", namespace, kind, name, specHash)
	}

	historyCtx, ok := res.(*ogenclient.RemediationHistoryContext)
	if !ok {
		return nil, fmt.Errorf("ds adapter: unexpected response type %T", res)
	}

	result := &RemediationHistoryResult{
		TargetResource:     historyCtx.TargetResource,
		RegressionDetected: historyCtx.RegressionDetected,
		Tier1Window:        historyCtx.Tier1.Window,
		Tier2Window:        historyCtx.Tier2.Window,
	}

	result.Tier1 = make([]Tier1Entry, 0, len(historyCtx.Tier1.Chain))
	for _, e := range historyCtx.Tier1.Chain {
		result.Tier1 = append(result.Tier1, mapTier1Entry(e))
	}
	slices.SortFunc(result.Tier1, func(a, b Tier1Entry) int {
		return b.CompletedAt.Compare(a.CompletedAt)
	})

	result.Tier2 = make([]Tier2Summary, 0, len(historyCtx.Tier2.Chain))
	for _, e := range historyCtx.Tier2.Chain {
		result.Tier2 = append(result.Tier2, mapTier2Summary(e))
	}
	slices.SortFunc(result.Tier2, func(a, b Tier2Summary) int {
		return b.CompletedAt.Compare(a.CompletedAt)
	})

	return result, nil
}

func mapTier1Entry(e ogenclient.RemediationHistoryEntry) Tier1Entry {
	entry := Tier1Entry{
		RemediationUID: e.RemediationUID,
		CompletedAt:    e.CompletedAt,
	}
	if e.SignalType.Set {
		entry.SignalType = e.SignalType.Value
	}
	if e.ActionType.Set && !e.ActionType.Null {
		entry.ActionType = e.ActionType.Value
	}
	if e.Outcome.Set {
		entry.Outcome = e.Outcome.Value
	}
	if e.EffectivenessScore.Set && !e.EffectivenessScore.Null {
		v := e.EffectivenessScore.Value
		entry.EffectivenessScore = &v
	}
	if e.SignalResolved.Set && !e.SignalResolved.Null {
		v := e.SignalResolved.Value
		entry.SignalResolved = &v
	}
	if e.HashMatch.Set {
		entry.HashMatch = string(e.HashMatch.Value)
	}
	if e.PreRemediationSpecHash.Set {
		entry.PreRemediationSpecHash = e.PreRemediationSpecHash.Value
	}
	if e.PostRemediationSpecHash.Set {
		entry.PostRemediationSpecHash = e.PostRemediationSpecHash.Value
	}
	if e.AssessmentReason.Set && !e.AssessmentReason.Null {
		entry.AssessmentReason = string(e.AssessmentReason.Value)
	}
	if e.HealthChecks.Set {
		entry.HealthChecks = mapHealthChecks(e.HealthChecks.Value)
	}
	if e.MetricDeltas.Set {
		entry.MetricDeltas = mapMetricDeltas(e.MetricDeltas.Value)
	}
	return entry
}

func mapTier2Summary(e ogenclient.RemediationHistorySummary) Tier2Summary {
	s := Tier2Summary{
		RemediationUID: e.RemediationUID,
		CompletedAt:    e.CompletedAt,
	}
	if e.SignalType.Set {
		s.SignalType = e.SignalType.Value
	}
	if e.ActionType.Set && !e.ActionType.Null {
		s.ActionType = e.ActionType.Value
	}
	if e.Outcome.Set {
		s.Outcome = e.Outcome.Value
	}
	if e.EffectivenessScore.Set && !e.EffectivenessScore.Null {
		v := e.EffectivenessScore.Value
		s.EffectivenessScore = &v
	}
	if e.SignalResolved.Set && !e.SignalResolved.Null {
		v := e.SignalResolved.Value
		s.SignalResolved = &v
	}
	if e.HashMatch.Set {
		s.HashMatch = string(e.HashMatch.Value)
	}
	if e.AssessmentReason.Set && !e.AssessmentReason.Null {
		s.AssessmentReason = string(e.AssessmentReason.Value)
	}
	return s
}

func mapHealthChecks(hc ogenclient.RemediationHealthChecks) *HealthChecks {
	result := &HealthChecks{}
	if hc.PodRunning.Set {
		v := hc.PodRunning.Value
		result.PodRunning = &v
	}
	if hc.ReadinessPass.Set {
		v := hc.ReadinessPass.Value
		result.ReadinessPass = &v
	}
	if hc.RestartDelta.Set {
		v := hc.RestartDelta.Value
		result.RestartDelta = &v
	}
	if hc.CrashLoops.Set {
		v := hc.CrashLoops.Value
		result.CrashLoops = &v
	}
	if hc.OomKilled.Set {
		v := hc.OomKilled.Value
		result.OomKilled = &v
	}
	if hc.PendingCount.Set {
		v := hc.PendingCount.Value
		result.PendingCount = &v
	}
	return result
}

func mapMetricDeltas(md ogenclient.RemediationMetricDeltas) *MetricDeltas {
	result := &MetricDeltas{}
	if md.CpuBefore.Set {
		v := md.CpuBefore.Value
		result.CpuBefore = &v
	}
	if md.CpuAfter.Set {
		v := md.CpuAfter.Value
		result.CpuAfter = &v
	}
	if md.MemoryBefore.Set {
		v := md.MemoryBefore.Value
		result.MemoryBefore = &v
	}
	if md.MemoryAfter.Set {
		v := md.MemoryAfter.Value
		result.MemoryAfter = &v
	}
	if md.LatencyP95BeforeMs.Set {
		v := md.LatencyP95BeforeMs.Value
		result.LatencyP95BeforeMs = &v
	}
	if md.LatencyP95AfterMs.Set {
		v := md.LatencyP95AfterMs.Value
		result.LatencyP95AfterMs = &v
	}
	if md.ErrorRateBefore.Set {
		v := md.ErrorRateBefore.Value
		result.ErrorRateBefore = &v
	}
	if md.ErrorRateAfter.Set {
		v := md.ErrorRateAfter.Value
		result.ErrorRateAfter = &v
	}
	return result
}
