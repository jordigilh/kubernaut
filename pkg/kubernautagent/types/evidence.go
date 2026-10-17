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

package types

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
)

// EvidenceStatus is the bounded, server-owned evidence outcome carried beside
// an investigation result. It deliberately contains classifications and
// validated identities only; raw transport errors, URLs, credentials, and MCP
// payloads never enter this type.
type EvidenceStatus struct {
	Version          string                 `json:"version"`
	Class            EvidenceFailureClass   `json:"class"`
	Source           EvidenceSource         `json:"source"`
	Outcome          EvidenceOutcome        `json:"outcome"`
	TargetRef        string                 `json:"target_ref,omitempty"`
	ClusterID        string                 `json:"cluster_id,omitempty"`
	LogicalTool      string                 `json:"logical_tool,omitempty"`
	Attempts         int                    `json:"attempts,omitempty"`
	LastFailureCode  string                 `json:"last_failure_code,omitempty"`
	CorrelationID    string                 `json:"correlation_id,omitempty"`
	Sources          []EvidenceSourceStatus `json:"sources,omitempty"`
	SourcesTruncated bool                   `json:"sources_truncated,omitempty"`
}

// EvidenceSourceStatus is the bounded status for one (target, source) pair.
// The scope uses that pair as its merge key so a failed optional source cannot
// replace valid evidence from another target or source.
type EvidenceSourceStatus struct {
	Version         string               `json:"version"`
	Class           EvidenceFailureClass `json:"class"`
	Source          EvidenceSource       `json:"source"`
	Outcome         EvidenceOutcome      `json:"outcome"`
	TargetRef       string               `json:"target_ref,omitempty"`
	ClusterID       string               `json:"cluster_id,omitempty"`
	LogicalTool     string               `json:"logical_tool,omitempty"`
	Attempts        int                  `json:"attempts,omitempty"`
	LastFailureCode string               `json:"last_failure_code,omitempty"`
	CorrelationID   string               `json:"correlation_id,omitempty"`
}

// EvidenceFailureClass is a closed set of terminal evidence classifications.
type EvidenceFailureClass string

const (
	EvidenceFailureNone                       EvidenceFailureClass = "none"
	EvidenceFailureHistoryBasedIneffective    EvidenceFailureClass = "history_based_ineffective_remediation"
	EvidenceFailureRCAIncomplete              EvidenceFailureClass = "rca_incomplete"
	EvidenceFailureTransportUnavailable       EvidenceFailureClass = "evidence_transport_unavailable"
	EvidenceFailureSchemaClientRequestFailure EvidenceFailureClass = "schema_client_request_failure"
)

// EvidenceSource identifies a bounded evidence producer.
type EvidenceSource string

const (
	EvidenceSourceRemediationHistory EvidenceSource = "remediation_history"
	EvidenceSourceOwnerChain         EvidenceSource = "owner_chain"
	EvidenceSourceDetectedLabels     EvidenceSource = "detected_labels"
	EvidenceSourceMCP                EvidenceSource = "mcp"
	EvidenceSourceContextTool        EvidenceSource = "context_tool"
)

// EvidenceOutcome is a closed set of source/terminal outcomes.
type EvidenceOutcome string

const (
	EvidenceOutcomeSuccess              EvidenceOutcome = "success"
	EvidenceOutcomePartial              EvidenceOutcome = "partial"
	EvidenceOutcomeSkipped              EvidenceOutcome = "skipped"
	EvidenceOutcomeValidEmpty           EvidenceOutcome = "valid_empty"
	EvidenceOutcomeTransportUnavailable EvidenceOutcome = "transport_unavailable"
	EvidenceOutcomeSchemaClientFailure  EvidenceOutcome = "schema_client_failure"
	EvidenceOutcomeCanceled             EvidenceOutcome = "canceled"
	EvidenceOutcomeSourcesTruncated     EvidenceOutcome = "sources_truncated"
)

const (
	// EvidenceStatusVersion is the version of the bounded evidence contract.
	EvidenceStatusVersion = "v1"
	maxEvidenceString     = 253
	maxEvidenceAttempts   = 100
	maxEvidenceSources    = 8
)

// Server-owned failure codes. These values are safe for audit fields and
// metrics; raw dependency errors remain wrapped causes/log context only.
const (
	EvidenceFailureCodeHashUnavailable  = "hash_unavailable"
	EvidenceFailureCodeTransport        = "transport_unavailable"
	EvidenceFailureCodeSchema           = "schema_client_failure"
	EvidenceFailureCodeForbidden        = "rbac_forbidden"
	EvidenceFailureCodeCanceled         = "canceled"
	EvidenceFailureCodeSourcesTruncated = "sources_truncated"
)

// NewEvidenceStatus creates a normalized aggregate status.
func NewEvidenceStatus(class EvidenceFailureClass, source EvidenceSource, outcome EvidenceOutcome) *EvidenceStatus {
	return &EvidenceStatus{
		Version: EvidenceStatusVersion,
		Class:   class,
		Source:  source,
		Outcome: outcome,
	}
}

// NewEvidenceSourceStatus creates a normalized source status.
func NewEvidenceSourceStatus(source EvidenceSource, outcome EvidenceOutcome) EvidenceSourceStatus {
	return EvidenceSourceStatus{Version: EvidenceStatusVersion, Source: source, Outcome: outcome}
}

func (s EvidenceStatus) normalized() EvidenceStatus {
	s.Version = boundedEvidenceString(s.Version)
	if s.Version == "" {
		s.Version = EvidenceStatusVersion
	}
	s.Class = normalizeFailureClass(s.Class)
	s.Source = normalizeEvidenceSource(s.Source)
	s.Outcome = normalizeEvidenceOutcome(s.Outcome)
	s.TargetRef = boundedEvidenceString(s.TargetRef)
	s.ClusterID = boundedEvidenceString(s.ClusterID)
	s.LogicalTool = boundedEvidenceString(s.LogicalTool)
	s.LastFailureCode = boundedEvidenceString(s.LastFailureCode)
	s.CorrelationID = boundedEvidenceString(s.CorrelationID)
	s.Attempts = boundedAttempts(s.Attempts)
	return s
}

func (s EvidenceSourceStatus) normalized() EvidenceSourceStatus {
	s.Version = boundedEvidenceString(s.Version)
	if s.Version == "" {
		s.Version = EvidenceStatusVersion
	}
	s.Class = normalizeFailureClass(s.Class)
	s.Source = normalizeEvidenceSource(s.Source)
	s.Outcome = normalizeEvidenceOutcome(s.Outcome)
	s.TargetRef = boundedEvidenceString(s.TargetRef)
	s.ClusterID = boundedEvidenceString(s.ClusterID)
	s.LogicalTool = boundedEvidenceString(s.LogicalTool)
	s.LastFailureCode = boundedEvidenceString(s.LastFailureCode)
	s.CorrelationID = boundedEvidenceString(s.CorrelationID)
	s.Attempts = boundedAttempts(s.Attempts)
	return s
}

func boundedEvidenceString(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > maxEvidenceString {
		return value[:maxEvidenceString]
	}
	return value
}

func boundedAttempts(attempts int) int {
	if attempts < 0 {
		return 0
	}
	if attempts > maxEvidenceAttempts {
		return maxEvidenceAttempts
	}
	return attempts
}

func normalizeFailureClass(value EvidenceFailureClass) EvidenceFailureClass {
	switch value {
	case EvidenceFailureNone, EvidenceFailureHistoryBasedIneffective,
		EvidenceFailureRCAIncomplete, EvidenceFailureTransportUnavailable,
		EvidenceFailureSchemaClientRequestFailure:
		return value
	default:
		return EvidenceFailureSchemaClientRequestFailure
	}
}

func normalizeEvidenceSource(value EvidenceSource) EvidenceSource {
	switch value {
	case EvidenceSourceRemediationHistory, EvidenceSourceOwnerChain,
		EvidenceSourceDetectedLabels, EvidenceSourceMCP, EvidenceSourceContextTool:
		return value
	default:
		return EvidenceSourceContextTool
	}
}

func normalizeEvidenceOutcome(value EvidenceOutcome) EvidenceOutcome {
	switch value {
	case EvidenceOutcomeSuccess, EvidenceOutcomePartial, EvidenceOutcomeSkipped,
		EvidenceOutcomeValidEmpty, EvidenceOutcomeTransportUnavailable,
		EvidenceOutcomeSchemaClientFailure, EvidenceOutcomeCanceled,
		EvidenceOutcomeSourcesTruncated:
		return value
	default:
		return EvidenceOutcomeSchemaClientFailure
	}
}

// EvidenceStatusScope is a per-investigation, concurrency-safe source-status
// accumulator. It is intentionally independent of the investigator package so
// context tools and automatic enrichment use the same carrier.
type EvidenceStatusScope struct {
	mu      sync.Mutex
	status  EvidenceStatus
	sources map[string]EvidenceSourceStatus
}

// NewEvidenceStatusScope creates an empty scope.
func NewEvidenceStatusScope() *EvidenceStatusScope {
	return &EvidenceStatusScope{sources: make(map[string]EvidenceSourceStatus)}
}

// Merge adds an aggregate status and its source entries. It keeps at most eight
// deterministic source entries and preserves the most consequential outcome.
func (s *EvidenceStatusScope) Merge(status *EvidenceStatus) {
	if s == nil || status == nil {
		return
	}
	normalized := status.normalized()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mergeAggregateLocked(normalized)
	if normalized.Source != "" && normalized.Outcome != "" {
		s.mergeSourceLocked(EvidenceSourceStatus{
			Version:         normalized.Version,
			Class:           normalized.Class,
			Source:          normalized.Source,
			Outcome:         normalized.Outcome,
			TargetRef:       normalized.TargetRef,
			ClusterID:       normalized.ClusterID,
			LogicalTool:     normalized.LogicalTool,
			Attempts:        normalized.Attempts,
			LastFailureCode: normalized.LastFailureCode,
			CorrelationID:   normalized.CorrelationID,
		})
	}
	for _, source := range normalized.Sources {
		s.mergeSourceLocked(source)
	}
}

// MergeSource adds a single source status without requiring callers to create
// an aggregate first.
func (s *EvidenceStatusScope) MergeSource(source EvidenceSourceStatus) {
	if s == nil {
		return
	}
	source = source.normalized()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mergeSourceLocked(source)
	s.mergeAggregateLocked(EvidenceStatus{
		Version:         source.Version,
		Class:           source.Class,
		Source:          source.Source,
		Outcome:         source.Outcome,
		TargetRef:       source.TargetRef,
		ClusterID:       source.ClusterID,
		LogicalTool:     source.LogicalTool,
		Attempts:        source.Attempts,
		LastFailureCode: source.LastFailureCode,
		CorrelationID:   source.CorrelationID,
	})
}

// Snapshot returns an immutable copy suitable for an InvestigationResult or
// terminal hook. Nil means no evidence producer reported a status.
func (s *EvidenceStatusScope) Snapshot() *EvidenceStatus {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Source == "" && len(s.sources) == 0 {
		return nil
	}
	copyStatus := s.status
	copyStatus.Sources = make([]EvidenceSourceStatus, 0, len(s.sources))
	for _, source := range s.sources {
		copyStatus.Sources = append(copyStatus.Sources, source)
	}
	sort.Slice(copyStatus.Sources, func(i, j int) bool {
		left, right := copyStatus.Sources[i], copyStatus.Sources[j]
		return sourceKey(left) < sourceKey(right)
	})
	return &copyStatus
}

func (s *EvidenceStatusScope) mergeAggregateLocked(incoming EvidenceStatus) {
	if s.status.Version == "" {
		s.status = incoming
		return
	}
	if evidenceOutcomeRank(incoming.Outcome) >= evidenceOutcomeRank(s.status.Outcome) {
		s.status.Class = incoming.Class
		s.status.Source = incoming.Source
		s.status.Outcome = incoming.Outcome
	}
	if incoming.TargetRef != "" {
		s.status.TargetRef = incoming.TargetRef
	}
	if incoming.ClusterID != "" {
		s.status.ClusterID = incoming.ClusterID
	}
	if incoming.LogicalTool != "" {
		s.status.LogicalTool = incoming.LogicalTool
	}
	if incoming.Attempts > s.status.Attempts {
		s.status.Attempts = incoming.Attempts
	}
	if incoming.LastFailureCode != "" {
		s.status.LastFailureCode = incoming.LastFailureCode
	}
	if incoming.CorrelationID != "" {
		s.status.CorrelationID = incoming.CorrelationID
	}
	s.status.SourcesTruncated = s.status.SourcesTruncated || incoming.SourcesTruncated
}

func (s *EvidenceStatusScope) mergeSourceLocked(incoming EvidenceSourceStatus) {
	if incoming.Source == "" {
		return
	}
	key := sourceKey(incoming)
	if existing, ok := s.sources[key]; ok {
		if evidenceOutcomeRank(incoming.Outcome) >= evidenceOutcomeRank(existing.Outcome) {
			existing.Class = incoming.Class
			existing.Outcome = incoming.Outcome
		}
		if incoming.Attempts > existing.Attempts {
			existing.Attempts = incoming.Attempts
		}
		if incoming.LastFailureCode != "" {
			existing.LastFailureCode = incoming.LastFailureCode
		}
		s.sources[key] = existing
		return
	}
	if len(s.sources) >= maxEvidenceSources {
		s.status.SourcesTruncated = true
		s.status.Outcome = EvidenceOutcomeSourcesTruncated
		return
	}
	s.sources[key] = incoming
}

func sourceKey(source EvidenceSourceStatus) string {
	return source.TargetRef + "\x00" + string(source.Source)
}

func evidenceOutcomeRank(outcome EvidenceOutcome) int {
	switch outcome {
	case EvidenceOutcomeSchemaClientFailure:
		return 7
	case EvidenceOutcomeTransportUnavailable:
		return 6
	case EvidenceOutcomeCanceled:
		return 5
	case EvidenceOutcomeSourcesTruncated:
		return 4
	case EvidenceOutcomePartial:
		return 3
	case EvidenceOutcomeSkipped:
		return 2
	case EvidenceOutcomeValidEmpty:
		return 1
	default:
		return 0
	}
}

type evidenceScopeKey struct{}

// WithEvidenceStatusScope attaches a scope to a context.
func WithEvidenceStatusScope(ctx context.Context, scope *EvidenceStatusScope) context.Context {
	return context.WithValue(ctx, evidenceScopeKey{}, scope)
}

// EvidenceStatusScopeFromContext returns the scope attached to ctx.
func EvidenceStatusScopeFromContext(ctx context.Context) (*EvidenceStatusScope, bool) {
	scope, ok := ctx.Value(evidenceScopeKey{}).(*EvidenceStatusScope)
	return scope, ok && scope != nil
}

// InvestigationError carries terminal evidence when no result object exists.
// It preserves the original error for errors.Is/errors.As without serializing
// its raw text into EvidenceStatus.
type InvestigationError struct {
	Err            error
	EvidenceStatus *EvidenceStatus
}

func (e *InvestigationError) Error() string {
	if e == nil || e.Err == nil {
		return "investigation failed"
	}
	return e.Err.Error()
}

func (e *InvestigationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NewInvestigationError wraps err with a bounded evidence snapshot.
func NewInvestigationError(err error, status *EvidenceStatus) error {
	if err == nil && status == nil {
		return nil
	}
	return &InvestigationError{Err: err, EvidenceStatus: status}
}

// EvidenceStatusFromError extracts the typed terminal status, if present.
func EvidenceStatusFromError(err error) *EvidenceStatus {
	var carrier *InvestigationError
	if errors.As(err, &carrier) && carrier != nil {
		return carrier.EvidenceStatus
	}
	return nil
}
