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
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// DD-KA-2496 / BR-KA-016: this packet exists only for the current model handoff.
// Status/audit/live projections receive its digest, not its observation contents.
// Pointer fields make unknown distinct from known empty/core group, zero or false.
type evidenceHandoff struct {
	SchemaVersion       string                  `json:"schema_version"`
	Target              handoffTarget           `json:"target"`
	TargetResolution    handoffTargetResolution `json:"target_resolution"`
	RCA                 handoffRCA              `json:"rca"`
	CurrentObservations []handoffObservation    `json:"current_observations"`
	PriorAttempts       []handoffPriorAttempt   `json:"prior_attempts"`
	Context             handoffContext          `json:"context"`
	Comparisons         []string                `json:"comparisons"`
	Hypotheses          []string                `json:"hypotheses"`
	ContraryEvidence    []string                `json:"contrary_evidence"`
	Unknowns            []string                `json:"unknowns"`
	HandoffDigest       string                  `json:"handoff_digest,omitempty"`
	FrozenAt            *time.Time              `json:"frozen_at,omitempty"`
}

type handoffTarget struct {
	ClusterID  *string `json:"cluster_id"`
	APIGroup   *string `json:"api_group"`
	APIVersion *string `json:"api_version"`
	Kind       *string `json:"kind"`
	Namespace  *string `json:"namespace"`
	Name       *string `json:"name"`
}

type handoffTargetResolution struct {
	Basis      string     `json:"basis"`
	ObservedAt *time.Time `json:"observed_at"`
}

type handoffRCA struct {
	Summary               string                      `json:"summary"`
	Confidence            *float64                    `json:"confidence"`
	InvestigationAnalysis string                      `json:"investigation_analysis"`
	CausalChain           []string                    `json:"causal_chain"`
	DueDiligence          *katypes.DueDiligenceReview `json:"due_diligence"`
}

type handoffPriorAttempt struct {
	Attempt                     int                    `json:"attempt"`
	RemediationUID              string                 `json:"remediation_uid"`
	WorkflowID                  *string                `json:"workflow_id"`
	WorkflowName                *string                `json:"workflow_name"`
	WorkflowSelectionConfidence *float64               `json:"workflow_selection_confidence"`
	ActionType                  *string                `json:"action_type"`
	SignalType                  *string                `json:"signal_type"`
	RecordedAt                  *time.Time             `json:"recorded_at"`
	Assessment                  handoffPriorAssessment `json:"assessment"`
	RecordedState               handoffPriorState      `json:"recorded_state"`
	sortKey                     string
}

type handoffPriorAssessment struct {
	Reason             *string  `json:"reason"`
	EffectivenessScore *float64 `json:"effectiveness_score"`
	SignalResolved     *bool    `json:"signal_resolved"`
	Outcome            *string  `json:"outcome"`
	CompletedValid     bool     `json:"completed_valid"`
	FailureCause       *string  `json:"failure_cause"`
}

type handoffPriorState struct {
	PreSpecHash  *string                  `json:"pre_spec_hash"`
	PostSpecHash *string                  `json:"post_spec_hash"`
	HashMatch    *string                  `json:"hash_match"`
	HealthChecks *enrichment.HealthChecks `json:"health_checks"`
	MetricDeltas *enrichment.MetricDeltas `json:"metric_deltas"`
}

type handoffContext struct {
	OwnerChain         []string                                 `json:"owner_chain"`
	DetectedLabels     map[string]string                        `json:"detected_labels"`
	QuotaDetails       map[string]enrichment.QuotaResourceUsage `json:"quota_details"`
	Tier1Window        *string                                  `json:"tier1_window"`
	Tier2Window        *string                                  `json:"tier2_window"`
	RegressionDetected *bool                                    `json:"regression_detected"`
	SignalFiringTime   *string                                  `json:"signal_firing_time"`
	SignalReceivedTime *string                                  `json:"signal_received_time"`
}

// source is deep-copied and sanitized before any projection is rendered. It is
// never changed by Phase 3 or by callbacks mutating the original enrichment/RCA.
type handoffInput struct {
	Signal     katypes.SignalContext
	RCASummary string
	EnrichData *prompt.EnrichmentData
	Phase1     *prompt.Phase1Data
}

type frozenEvidenceHandoff struct {
	serialized string
	digest     string
	source     handoffInput
	repeat     *parser.RepeatSelectionContext
}

func optionalHandoffString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func handoffRecordedTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func scopedHandoffInput(in handoffInput) handoffInput {
	if in.EnrichData == nil {
		return in
	}
	data := *in.EnrichData
	in.EnrichData = &data
	if data.HistoryResult != nil {
		if data.HistoryResult.TargetResource != in.Signal.Namespace+"/"+in.Signal.ResourceKind+"/"+in.Signal.ResourceName {
			data.HistoryResult = nil // Never disclose or match another target's history.
		}
	}
	return in
}

func (inv *Investigator) freezeEvidenceHandoff(ctx context.Context, in handoffInput, id string) (*frozenEvidenceHandoff, error) {
	if in.EnrichData != nil && in.EnrichData.HistoryUnavailable {
		return nil, fmt.Errorf("remediation history acquisition failed; complete repeat context is unavailable")
	}
	observations, err := inv.evidenceSnapshot(id)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(scopedHandoffInput(in))
	if err != nil {
		return nil, fmt.Errorf("serialize handoff source: %w", err)
	}
	// Sanitize string leaves, not the whole JSON as text: the tool-output I1
	// mimicry rule must not strip legitimate server-owned workflow_id fields.
	sanitized, err := inv.sanitizeHandoffJSON(ctx, raw)
	if err != nil {
		return nil, err
	}
	var source handoffInput
	if err := json.Unmarshal(sanitized, &source); err != nil {
		return nil, fmt.Errorf("freeze handoff source: %w", err)
	}
	packet, err := buildEvidenceHandoff(source, observations)
	if err != nil {
		return nil, err
	}
	if source.Signal.ResourceAPIVersion != "" && packet.Target.APIGroup == nil {
		inv.logger.Info("Target API group/version unavailable in evidence handoff", "correlation_id", id, "reason", "unparseable_api_version")
	}
	wire, digest, err := packet.serializeFrozen(time.Now().UTC(), inv.evidence.budget)
	if err != nil {
		return nil, err
	}
	packet.HandoffDigest = digest
	return &frozenEvidenceHandoff{serialized: wire, digest: digest,
		source: source, repeat: repeatContextFromHandoff(packet)}, nil
}

// serializeFrozen is the single representation/budget boundary for DD-KA-2496.
// Copy the packet so a failed freeze never mutates its caller, and always compute
// runtime metadata here instead of including caller-supplied metadata in the hash.
func (packet evidenceHandoff) serializeFrozen(now time.Time, budget int) (string, string, error) {
	packet.HandoffDigest, packet.FrozenAt = "", nil
	body, err := json.Marshal(packet)
	if err != nil {
		return "", "", fmt.Errorf("serialize evidence handoff: %w", err)
	}
	var canonical map[string]json.RawMessage
	if err := json.Unmarshal(body, &canonical); err != nil {
		return "", "", fmt.Errorf("freeze evidence handoff: %w", err)
	}
	// Canonical JSON has deterministic map ordering. Neither runtime field is
	// present yet; both are excluded from the digest, but included in the budget.
	canonicalBody, err := json.Marshal(canonical)
	if err != nil {
		return "", "", fmt.Errorf("canonicalize evidence handoff: %w", err)
	}
	packet.HandoffDigest = fmt.Sprintf("sha256:%x", sha256.Sum256(canonicalBody))
	now = now.UTC()
	packet.FrozenAt = &now
	wire, err := json.Marshal(packet)
	if err != nil {
		return "", "", fmt.Errorf("serialize frozen evidence handoff: %w", err)
	}
	if len(wire) > budget {
		return "", "", fmt.Errorf("evidence handoff exceeds configured byte budget")
	}
	return string(wire), packet.HandoffDigest, nil
}

func buildEvidenceHandoff(source handoffInput, observations []handoffObservation) (*evidenceHandoff, error) {
	signal := source.Signal
	packet := &evidenceHandoff{
		SchemaVersion: "1",
		Target: handoffTarget{ClusterID: optionalHandoffString(signal.ClusterID),
			Kind: optionalHandoffString(signal.ResourceKind), Namespace: optionalHandoffString(signal.Namespace), Name: optionalHandoffString(signal.ResourceName)},
		TargetResolution: handoffTargetResolution{Basis: "trusted_signal"},
		RCA:              handoffRCA{Summary: source.RCASummary}, CurrentObservations: observations,
		PriorAttempts: []handoffPriorAttempt{}, Comparisons: []string{}, Hypotheses: []string{}, ContraryEvidence: []string{},
		Unknowns: []string{"target-resolution observation timestamp is unknown", "prior resource manifests and failure mechanisms are unknown", "current/prior state comparison and separately recorded contrary evidence are unavailable"},
		Context:  handoffContext{SignalFiringTime: optionalHandoffString(signal.FiringTime), SignalReceivedTime: optionalHandoffString(signal.ReceivedTime)},
	}
	if signal.ResourceAPIVersion != "" {
		gv, err := schema.ParseGroupVersion(signal.ResourceAPIVersion)
		if err != nil {
			// DD-KA-2496: unavailable target values remain unknown. The
			// existing API-version gate accepts any non-empty value and leaves
			// validity to downstream consumers; do not add a handoff veto or
			// rewrite their input while projecting a separate group/version.
			packet.Unknowns = append(packet.Unknowns, "target API group/version could not be parsed")
		} else {
			packet.Target.APIGroup, packet.Target.APIVersion = &gv.Group, &gv.Version
		}
	} else {
		packet.Unknowns = append(packet.Unknowns, "target API group/version is unknown")
	}
	if signal.ClusterID == "" {
		packet.Unknowns = append(packet.Unknowns, "target cluster ID is unknown")
	}
	if len(observations) == 0 {
		packet.Unknowns = append(packet.Unknowns, "current observations were not recorded for this request")
	}
	if source.RCASummary != "" {
		packet.Hypotheses = append(packet.Hypotheses, source.RCASummary)
	}
	packet.projectPhase1(source.Phase1, signal)
	packet.projectEnrichment(source.EnrichData)
	for i := range packet.PriorAttempts {
		attempt := &packet.PriorAttempts[i]
		key, err := json.Marshal(attempt)
		if err != nil {
			return nil, fmt.Errorf("serialize prior attempt: %w", err)
		}
		attempt.sortKey = string(key)
	}
	slices.SortStableFunc(packet.PriorAttempts, compareHandoffAttempts)
	for i := range packet.PriorAttempts {
		packet.PriorAttempts[i].Attempt = i + 1
	}
	return packet, nil
}

func (packet *evidenceHandoff) projectPhase1(p1 *prompt.Phase1Data, signal katypes.SignalContext) {
	if p1 == nil {
		return
	}
	packet.RCA.Confidence, packet.RCA.InvestigationAnalysis = p1.RCAConfidence, p1.InvestigationAnalysis
	packet.RCA.CausalChain, packet.RCA.DueDiligence = p1.CausalChain, p1.DueDiligence
	packet.Hypotheses = append(packet.Hypotheses, p1.ContributingFactors...)
	if p1.RemediationTarget.Kind != "" && p1.RemediationTarget.Kind == signal.ResourceKind && p1.RemediationTarget.Name == signal.ResourceName {
		packet.TargetResolution.Basis = "phase_1_rca"
	}
	if p1.DueDiligence != nil && p1.DueDiligence.AlternativeHypotheses != "" {
		packet.Hypotheses = append(packet.Hypotheses, p1.DueDiligence.AlternativeHypotheses)
	}
}

func (packet *evidenceHandoff) projectEnrichment(data *prompt.EnrichmentData) {
	if data == nil {
		packet.Unknowns = append(packet.Unknowns, "target-scoped enrichment and prior history are unavailable")
		return
	}
	packet.Context.OwnerChain, packet.Context.DetectedLabels, packet.Context.QuotaDetails = data.OwnerChain, data.DetectedLabels, data.QuotaDetails
	h := data.HistoryResult
	if h == nil {
		packet.Unknowns = append(packet.Unknowns, "target-scoped prior history is unavailable")
		return
	}
	packet.Context.Tier1Window, packet.Context.Tier2Window = optionalHandoffString(h.Tier1Window), optionalHandoffString(h.Tier2Window)
	packet.Context.RegressionDetected = &h.RegressionDetected
	packet.PriorAttempts = projectHandoffHistory(h)
}

func projectHandoffHistory(history *enrichment.RemediationHistoryResult) []handoffPriorAttempt {
	attempts := make([]handoffPriorAttempt, 0, len(history.Tier1))
	for _, prior := range history.Tier1 {
		attempts = append(attempts, handoffPriorAttempt{
			RemediationUID: prior.RemediationUID, WorkflowID: optionalHandoffString(prior.WorkflowID), WorkflowName: optionalHandoffString(prior.WorkflowName),
			WorkflowSelectionConfidence: prior.WorkflowSelectionConfidence, ActionType: optionalHandoffString(prior.ActionType), SignalType: optionalHandoffString(prior.SignalType),
			RecordedAt: handoffRecordedTime(prior.CompletedAt),
			Assessment: handoffPriorAssessment{Reason: optionalHandoffString(prior.AssessmentReason), EffectivenessScore: prior.EffectivenessScore,
				SignalResolved: prior.SignalResolved, Outcome: optionalHandoffString(prior.Outcome), CompletedValid: enrichment.IsCompletedValidAssessment(prior.AssessmentReason, prior.EffectivenessScore)},
			RecordedState: handoffPriorState{PreSpecHash: optionalHandoffString(prior.PreRemediationSpecHash), PostSpecHash: optionalHandoffString(prior.PostRemediationSpecHash),
				HashMatch: optionalHandoffString(prior.HashMatch), HealthChecks: prior.HealthChecks, MetricDeltas: prior.MetricDeltas},
		})
	}
	// Grow reserves the second tier without unchecked allocation-size addition.
	attempts = slices.Grow(attempts, len(history.Tier2))
	for _, prior := range history.Tier2 {
		attempts = append(attempts, handoffPriorAttempt{
			RemediationUID: prior.RemediationUID, WorkflowID: optionalHandoffString(prior.WorkflowID), WorkflowName: optionalHandoffString(prior.WorkflowName),
			WorkflowSelectionConfidence: prior.WorkflowSelectionConfidence, ActionType: optionalHandoffString(prior.ActionType), SignalType: optionalHandoffString(prior.SignalType),
			RecordedAt: handoffRecordedTime(prior.CompletedAt),
			Assessment: handoffPriorAssessment{Reason: optionalHandoffString(prior.AssessmentReason), EffectivenessScore: prior.EffectivenessScore,
				SignalResolved: prior.SignalResolved, Outcome: optionalHandoffString(prior.Outcome), CompletedValid: enrichment.IsCompletedValidAssessment(prior.AssessmentReason, prior.EffectivenessScore)},
			RecordedState: handoffPriorState{HashMatch: optionalHandoffString(prior.HashMatch)},
		})
	}
	return attempts
}

func compareHandoffAttempts(a, b handoffPriorAttempt) int {
	var at, bt time.Time
	if a.RecordedAt != nil {
		at = *a.RecordedAt
	}
	if b.RecordedAt != nil {
		bt = *b.RecordedAt
	}
	if cmp := at.Compare(bt); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(a.RemediationUID, b.RemediationUID); cmp != 0 {
		return cmp
	}
	return strings.Compare(a.sortKey, b.sortKey)
}

func repeatContextFromHandoff(packet *evidenceHandoff) *parser.RepeatSelectionContext {
	scope := &parser.RepeatSelectionContext{CompletedPriorWorkflows: make(map[string][]string), HandoffDigest: packet.HandoffDigest}
	for _, prior := range packet.PriorAttempts {
		if prior.WorkflowID == nil || prior.WorkflowName == nil || strings.TrimSpace(*prior.WorkflowID) == "" ||
			strings.TrimSpace(*prior.WorkflowName) == "" || !prior.Assessment.CompletedValid {
			continue
		}
		id := *prior.WorkflowID
		if !slices.Contains(scope.CompletedPriorWorkflows[id], prior.RemediationUID) {
			scope.CompletedPriorWorkflows[id] = append(scope.CompletedPriorWorkflows[id], prior.RemediationUID)
		}
	}
	return scope
}

func (inv *Investigator) sanitizeHandoffJSON(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	switch strings.TrimSpace(string(raw))[0] {
	case '"':
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("decode evidence text: %w", err)
		}
		clean, err := inv.sanitizeEvidenceText(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("sanitize evidence text: %w", err)
		}
		return json.Marshal(clean)
	case '{':
		return inv.sanitizeHandoffObject(ctx, raw)
	case '[':
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, fmt.Errorf("decode evidence array: %w", err)
		}
		for i, value := range values {
			clean, err := inv.sanitizeHandoffJSON(ctx, value)
			if err != nil {
				return nil, err
			}
			values[i] = clean
		}
		return json.Marshal(values)
	default:
		return raw, nil
	}
}

func (inv *Investigator) sanitizeHandoffObject(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("decode evidence object: %w", err)
	}
	cleaned := make(map[string]json.RawMessage, len(object))
	for key, value := range object {
		cleanKey, err := inv.sanitizeEvidenceText(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("sanitize evidence key: %w", err)
		}
		if cleanKey == "" || cleaned[cleanKey] != nil {
			return nil, fmt.Errorf("evidence key cannot be safely represented")
		}
		clean, err := inv.sanitizeHandoffJSON(ctx, value)
		if err != nil {
			return nil, err
		}
		cleaned[cleanKey] = clean
	}
	return json.Marshal(cleaned)
}
