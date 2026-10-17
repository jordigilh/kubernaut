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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
)

// OwnerChainEntry represents a single entry in a Kubernetes owner chain.
type OwnerChainEntry struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	// APIVersion captures the owner's API group/version from OwnerReference.
	// Format: "group/version" (e.g. "apps/v1") or "version" for core (e.g. "v1").
	// Issue #1040.
	APIVersion string `json:"api_version,omitempty"`
}

// DetectedLabels is the canonical label detection result type.
// Alias for sharedtypes.DetectedLabels to avoid import cycles and duplication.
type DetectedLabels = sharedtypes.DetectedLabels

// QuotaResourceUsage holds the hard limit and current usage for a single
// resource key inside a ResourceQuota. Matches HAPI v1.2.1's
// _summarize_quotas output (DD-KA-018 Detection 8).
type QuotaResourceUsage struct {
	Hard string `json:"hard"`
	Used string `json:"used"`
}

// K8sClient abstracts Kubernetes API access for enrichment.
// Issue #1040: apiVersion parameter disambiguates multi-group kinds (e.g. Route).
// Pass "" when unknown — preserves existing heuristic-based resolution.
type K8sClient interface {
	GetOwnerChain(ctx context.Context, kind, name, namespace, apiVersion string) ([]OwnerChainEntry, error)
	GetSpecHash(ctx context.Context, kind, name, namespace, apiVersion string) (string, error)
}

// DataStorageClient abstracts DataStorage API access for enrichment.
// clusterID optionally scopes the query to a single fleet cluster (Issue
// #1802, main only); pass "" for unscoped (release/v1.5 semantics).
type DataStorageClient interface {
	GetRemediationHistory(ctx context.Context, kind, name, namespace, clusterID, specHash string) (*RemediationHistoryResult, error)
}

// RemediationHistoryResult holds the full DS response mapped to domain types.
type RemediationHistoryResult struct {
	TargetResource     string         `json:"target_resource"`
	CurrentSpecHash    string         `json:"current_spec_hash"`
	RegressionDetected bool           `json:"regression_detected"`
	Tier1              []Tier1Entry   `json:"tier1"`
	Tier1Window        string         `json:"tier1_window"`
	Tier2              []Tier2Summary `json:"tier2"`
	Tier2Window        string         `json:"tier2_window"`
	// EvidenceStatus travels beside the LLM-facing history payload. It is not
	// serialized into tool output; the investigator's status scope carries it
	// through terminal mappings.
	EvidenceStatus *katypes.EvidenceStatus `json:"-"`
}

// HistoryFetchOutcome is the typed boundary result for a history lookup.
// Existing DataStorageClient callers continue to receive the mapped history;
// the status is carried on RemediationHistoryResult or HistoryFetchError so
// older fakes remain source-compatible.
type HistoryFetchOutcome struct {
	History *RemediationHistoryResult
	Status  katypes.EvidenceSourceStatus
}

// HistoryFetchError preserves a bounded source classification while retaining
// the wrapped dependency error for caller diagnostics and errors.Is/As.
type HistoryFetchError struct {
	Err    error
	Status katypes.EvidenceSourceStatus
}

func (e *HistoryFetchError) Error() string {
	if e == nil || e.Err == nil {
		return "remediation history fetch failed"
	}
	return e.Err.Error()
}

func (e *HistoryFetchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func historyTargetRef(kind, name, namespace string) string {
	return strings.Trim(strings.Join([]string{namespace, kind, name}, "/"), "/")
}

func historySourceStatus(kind, name, namespace, clusterID, correlationID string, outcome katypes.EvidenceOutcome, code string) katypes.EvidenceSourceStatus {
	class := katypes.EvidenceFailureNone
	switch outcome {
	case katypes.EvidenceOutcomeSuccess,
		katypes.EvidenceOutcomePartial,
		katypes.EvidenceOutcomeSkipped,
		katypes.EvidenceOutcomeValidEmpty,
		katypes.EvidenceOutcomeSourcesTruncated:
		// These outcomes describe a completed or intentionally bounded source;
		// they do not imply a terminal evidence failure class.
	case katypes.EvidenceOutcomeTransportUnavailable, katypes.EvidenceOutcomeCanceled:
		class = katypes.EvidenceFailureTransportUnavailable
	case katypes.EvidenceOutcomeSchemaClientFailure:
		class = katypes.EvidenceFailureSchemaClientRequestFailure
	}
	return katypes.EvidenceSourceStatus{
		Version:         katypes.EvidenceStatusVersion,
		Class:           class,
		Source:          katypes.EvidenceSourceRemediationHistory,
		Outcome:         outcome,
		TargetRef:       historyTargetRef(kind, name, namespace),
		ClusterID:       clusterID,
		LastFailureCode: code,
		CorrelationID:   correlationID,
	}
}

func historyAggregateStatus(status katypes.EvidenceSourceStatus) *katypes.EvidenceStatus {
	return &katypes.EvidenceStatus{
		Version:         status.Version,
		Class:           status.Class,
		Source:          status.Source,
		Outcome:         status.Outcome,
		TargetRef:       status.TargetRef,
		ClusterID:       status.ClusterID,
		LogicalTool:     status.LogicalTool,
		Attempts:        status.Attempts,
		LastFailureCode: status.LastFailureCode,
		CorrelationID:   status.CorrelationID,
		Sources:         []katypes.EvidenceSourceStatus{status},
	}
}

func mergeEvidenceSourceStatus(ctx context.Context, status katypes.EvidenceSourceStatus) {
	if scope, ok := katypes.EvidenceStatusScopeFromContext(ctx); ok {
		scope.MergeSource(status)
	}
}

func historyStatusFromError(err error, kind, name, namespace, clusterID, correlationID string) katypes.EvidenceSourceStatus {
	var typed *HistoryFetchError
	if errors.As(err, &typed) && typed != nil {
		return typed.Status
	}
	if errors.Is(err, ErrRBACForbidden) {
		return historySourceStatus(kind, name, namespace, clusterID, correlationID,
			katypes.EvidenceOutcomeSchemaClientFailure, katypes.EvidenceFailureCodeForbidden)
	}
	return historySourceStatus(kind, name, namespace, clusterID, correlationID,
		katypes.EvidenceOutcomeTransportUnavailable, katypes.EvidenceFailureCodeTransport)
}

// HistoryEvidenceStatusForError maps an enrichment failure that prevented a
// history result from being returned into the bounded source contract. It is
// used by the investigator when Enrich must return early (for example, before
// the normal history fetch path can attach its status).
func HistoryEvidenceStatusForError(err error, kind, name, namespace, clusterID, correlationID string) katypes.EvidenceSourceStatus {
	return historyStatusFromError(err, kind, name, namespace, clusterID, correlationID)
}

// Tier1Entry is a detailed remediation history record (recent window).
type Tier1Entry struct {
	RemediationUID string `json:"remediation_uid"`
	// DD-KA-2496: current-version RO selection metadata, never inferred.
	WorkflowID                  string   `json:"workflow_id,omitempty"`
	WorkflowName                string   `json:"workflow_name,omitempty"`
	WorkflowSelectionConfidence *float64 `json:"workflow_selection_confidence,omitempty"`
	SignalType                  string   `json:"signal_type,omitempty"`
	ActionType                  string   `json:"action_type,omitempty"`
	Outcome                     string   `json:"outcome,omitempty"`
	EffectivenessScore          *float64 `json:"effectiveness_score,omitempty"`
	SignalResolved              *bool    `json:"signal_resolved,omitempty"`
	HashMatch                   string   `json:"hash_match,omitempty"`
	// DD-KA-2511: server-derived application evidence, never model supplied.
	AppliedState            string        `json:"applied_state,omitempty"`
	SpecChangeStatus        string        `json:"spec_change_status,omitempty"`
	PreRemediationSpecHash  string        `json:"pre_remediation_spec_hash,omitempty"`
	PostRemediationSpecHash string        `json:"post_remediation_spec_hash,omitempty"`
	HealthChecks            *HealthChecks `json:"health_checks,omitempty"`
	MetricDeltas            *MetricDeltas `json:"metric_deltas,omitempty"`
	AssessmentReason        string        `json:"assessment_reason,omitempty"`
	CompletedAt             time.Time     `json:"completed_at"`
}

// Tier2Summary is a compact historical remediation record (wider window).
type Tier2Summary struct {
	RemediationUID              string    `json:"remediation_uid"`
	WorkflowID                  string    `json:"workflow_id,omitempty"`
	WorkflowName                string    `json:"workflow_name,omitempty"`
	WorkflowSelectionConfidence *float64  `json:"workflow_selection_confidence,omitempty"`
	SignalType                  string    `json:"signal_type,omitempty"`
	ActionType                  string    `json:"action_type,omitempty"`
	Outcome                     string    `json:"outcome,omitempty"`
	EffectivenessScore          *float64  `json:"effectiveness_score,omitempty"`
	SignalResolved              *bool     `json:"signal_resolved,omitempty"`
	HashMatch                   string    `json:"hash_match,omitempty"`
	AppliedState                string    `json:"applied_state,omitempty"`
	SpecChangeStatus            string    `json:"spec_change_status,omitempty"`
	PreRemediationSpecHash      string    `json:"pre_remediation_spec_hash,omitempty"`
	PostRemediationSpecHash     string    `json:"post_remediation_spec_hash,omitempty"`
	AssessmentReason            string    `json:"assessment_reason,omitempty"`
	CompletedAt                 time.Time `json:"completed_at"`
}

// IsCompletedValidAssessment implements the shared DD-KA-2496 evidence predicate.
// Full/Partial are completion reasons from EM, not synthetic RO outcome fields.
// Missing, non-finite, out-of-range and invalidated assessment data stay unknown.
func IsCompletedValidAssessment(reason string, score *float64) bool {
	return (reason == "Full" || reason == "Partial") && score != nil &&
		*score >= 0 && *score <= 1
}

// HealthChecks holds post-remediation health check results.
type HealthChecks struct {
	PodRunning    *bool `json:"pod_running,omitempty"`
	ReadinessPass *bool `json:"readiness_pass,omitempty"`
	RestartDelta  *int  `json:"restart_delta,omitempty"`
	CrashLoops    *bool `json:"crash_loops,omitempty"`
	OomKilled     *bool `json:"oom_killed,omitempty"`
	PendingCount  *int  `json:"pending_count,omitempty"`
}

// MetricDeltas holds before/after metric measurements.
type MetricDeltas struct {
	CpuBefore          *float64 `json:"cpu_before,omitempty"`
	CpuAfter           *float64 `json:"cpu_after,omitempty"`
	MemoryBefore       *float64 `json:"memory_before,omitempty"`
	MemoryAfter        *float64 `json:"memory_after,omitempty"`
	LatencyP95BeforeMs *float64 `json:"latency_p95_before_ms,omitempty"`
	LatencyP95AfterMs  *float64 `json:"latency_p95_after_ms,omitempty"`
	ErrorRateBefore    *float64 `json:"error_rate_before,omitempty"`
	ErrorRateAfter     *float64 `json:"error_rate_after,omitempty"`
	// ThroughputBeforeRps/After backfill a pre-existing gap: present in the raw
	// EM audit event and DS's RemediationMetricDeltas since 21e592475, but never
	// propagated to this domain type until now.
	ThroughputBeforeRps *float64 `json:"throughput_before_rps,omitempty"`
	ThroughputAfterRps  *float64 `json:"throughput_after_rps,omitempty"`
	// Cluster-scoped fields (Issue #193, DD-EM-005 v1.1): populated only when
	// the source remediation targeted a Node or PersistentVolume.
	NodeNotReadyBefore       *float64 `json:"node_not_ready_before,omitempty"`
	NodeNotReadyAfter        *float64 `json:"node_not_ready_after,omitempty"`
	NodeMemoryPressureBefore *float64 `json:"node_memory_pressure_before,omitempty"`
	NodeMemoryPressureAfter  *float64 `json:"node_memory_pressure_after,omitempty"`
	NodeDiskPressureBefore   *float64 `json:"node_disk_pressure_before,omitempty"`
	NodeDiskPressureAfter    *float64 `json:"node_disk_pressure_after,omitempty"`
	PvPhaseFailedBefore      *float64 `json:"pv_phase_failed_before,omitempty"`
	PvPhaseFailedAfter       *float64 `json:"pv_phase_failed_after,omitempty"`
	PvPhasePendingBefore     *float64 `json:"pv_phase_pending_before,omitempty"`
	PvPhasePendingAfter      *float64 `json:"pv_phase_pending_after,omitempty"`
	PvUsageRatioBefore       *float64 `json:"pv_usage_ratio_before,omitempty"`
	PvUsageRatioAfter        *float64 `json:"pv_usage_ratio_after,omitempty"`
}

// RetryConfig controls retry behavior for infrastructure calls in Enrich().
// With MaxRetries=0 (default), enrichment is best-effort (current behavior).
// With MaxRetries>0, all errors are retried with exponential backoff and
// failures after exhaustion trigger HardFail on EnrichmentResult.
// Matches HAPI v1.2.1 EnrichmentService behavior (BR-KA-261 AC#7).
type RetryConfig struct {
	MaxRetries  int
	BaseBackoff time.Duration
}

// EnrichmentResult is the combined enrichment data.
type EnrichmentResult struct {
	ResourceKind      string            `json:"resource_kind,omitempty"`
	ResourceName      string            `json:"resource_name,omitempty"`
	ResourceNamespace string            `json:"resource_namespace,omitempty"`
	OwnerChain        []OwnerChainEntry `json:"owner_chain"`
	// OwnerChainError is non-nil when GetOwnerChain fails (resource not found, API error).
	// Set for observability regardless of retry mode.
	OwnerChainError error `json:"-"`
	// HardFail is true when owner chain resolution fails after retry
	// exhaustion (all errors retried, matching HAPI v1.2.1). The
	// investigator uses this to trigger rca_incomplete (BR-KA-261
	// AC#7, #704). Only set when RetryConfig has MaxRetries > 0.
	// Issue #1039: NotFound errors are exempt (deleted resources).
	HardFail bool `json:"-"`
	// TargetResourceDeleted is true when the remediation target no longer
	// exists in the cluster (K8s NotFound). Issue #1039.
	TargetResourceDeleted bool                          `json:"-"`
	DetectedLabels        *DetectedLabels               `json:"detected_labels,omitempty"`
	QuotaDetails          map[string]QuotaResourceUsage `json:"quota_details,omitempty"`
	RemediationHistory    *RemediationHistoryResult     `json:"remediation_history,omitempty"`
	// HistoryUnavailable distinguishes acquisition failure from a successful
	// empty history. Phase 2 must require review when repeat context is unavailable.
	HistoryUnavailable bool `json:"history_unavailable,omitempty"`
	// HistoryStatus is the typed acquisition result used by the investigation
	// terminal contract. It is deliberately separate from HistoryUnavailable so
	// a successful empty history cannot be confused with transport failure.
	HistoryStatus sharedtypes.HistoryStatus `json:"history_status,omitempty"`
	// EvidenceStatus is the source status for this target's enrichment call.
	// It is carried beside prompts and merged into the investigation scope.
	EvidenceStatus *katypes.EvidenceStatus `json:"-"`
	// DetectedLabelsFallback marks labels copied from the signal target after
	// the RCA target's optional label source failed. History is never copied
	// across targets.
	DetectedLabelsFallback bool `json:"detected_labels_fallback,omitempty"`
}

// Enricher resolves owner chain, labels, and remediation history.
type Enricher struct {
	k8s           K8sClient
	ds            DataStorageClient
	auditStore    audit.AuditStore
	logger        logr.Logger
	labelDetector *LabelDetector
	labelResolver func(ctx context.Context) *LabelDetector
	retryConfig   RetryConfig
	k8sResolver   func(ctx context.Context) K8sClient
}

// NewEnricher creates an enricher with the given clients.
func NewEnricher(k8s K8sClient, ds DataStorageClient, auditStore audit.AuditStore, logger logr.Logger) *Enricher {
	return &Enricher{
		k8s:        k8s,
		ds:         ds,
		auditStore: auditStore,
		logger:     logger,
	}
}

// WithLabelDetector attaches a LabelDetector to run during Enrich().
func (e *Enricher) WithLabelDetector(ld *LabelDetector) *Enricher {
	e.labelDetector = ld
	return e
}

// WithLabelDetectorResolver installs a per-call override for label detection,
// matching WithK8sResolver's fleet routing semantics.
func (e *Enricher) WithLabelDetectorResolver(resolver func(ctx context.Context) *LabelDetector) *Enricher {
	e.labelResolver = resolver
	return e
}

// WithRetryConfig sets the retry policy for infrastructure calls.
func (e *Enricher) WithRetryConfig(cfg RetryConfig) *Enricher {
	e.retryConfig = cfg
	return e
}

// WithK8sResolver installs a per-call override for the K8sClient used by
// owner-chain and spec-hash resolution (Issue #2343). When resolver returns
// non-nil, its result is used instead of the K8sClient passed to
// NewEnricher; effectiveK8s falls back to that hub client otherwise -- so a
// hub-local investigation (no resolver installed, or resolver returns nil)
// is byte-identical to pre-#2343 behavior. Production wiring
// (cmd/kubernautagent/datastorage.go's buildEnricher) installs a resolver
// backed by custom.ResolveK8sClient, mirroring the same ctx-based fleet
// overlay routing the get_namespaced_resource_context/get_cluster_resource_context
// tools already use (issue #2306) -- this closes the one remaining call
// site (the automatic pre-fetch enrichment step) that bypassed it.
func (e *Enricher) WithK8sResolver(resolver func(ctx context.Context) K8sClient) *Enricher {
	e.k8sResolver = resolver
	return e
}

// effectiveK8s returns the K8sClient to use for this call: the resolver's
// result when one is installed and returns non-nil, otherwise the hub
// client passed to NewEnricher.
func (e *Enricher) effectiveK8s(ctx context.Context) K8sClient {
	if e.k8sResolver != nil {
		if k8s := e.k8sResolver(ctx); k8s != nil {
			return k8s
		}
	}
	return e.k8s
}

func (e *Enricher) effectiveLabelDetector(ctx context.Context) *LabelDetector {
	if e.labelResolver != nil {
		if detector := e.labelResolver(ctx); detector != nil {
			return detector
		}
	}
	return e.labelDetector
}

// resolveOwnerChainWithRetry calls GetOwnerChain with optional retry logic.
// With MaxRetries=0 (default): single call, best-effort.
// With MaxRetries>0: all errors are retried with exponential backoff,
// matching HAPI v1.2.1 EnrichmentService._retry (except Exception → retry).
// The caller sets HardFail on EnrichmentResult when this returns a non-nil error.
func (e *Enricher) resolveOwnerChainWithRetry(ctx context.Context, kind, name, namespace, apiVersion string) ([]OwnerChainEntry, error) {
	k8s := e.effectiveK8s(ctx)
	chain, err := k8s.GetOwnerChain(ctx, kind, name, namespace, apiVersion)
	if err == nil {
		return chain, nil
	}

	if isForbiddenError(err) {
		return nil, err
	}

	if e.retryConfig.MaxRetries == 0 || IsNotFoundError(err) || IsNoMatchError(err) {
		return nil, err
	}

	boCfg := backoff.Config{
		BasePeriod: e.retryConfig.BaseBackoff,
		Multiplier: 2.0,
		MaxPeriod:  e.retryConfig.BaseBackoff * 4,
	}

	for attempt := 1; attempt <= e.retryConfig.MaxRetries; attempt++ {
		wait := boCfg.Calculate(int32(attempt))
		e.logger.Info("enrichment: retrying GetOwnerChain",
			"attempt", attempt,
			"backoff", wait,
		)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}

		chain, err = k8s.GetOwnerChain(ctx, kind, name, namespace, apiVersion)
		if err == nil {
			return chain, nil
		}
		if isForbiddenError(err) {
			return nil, err
		}
	}

	return nil, err
}

// IsNoMatchError reports whether the error indicates an unknown Kind/resource
// type (CRD not registered with the API server). This distinguishes schema
// limitations (e.g. cert-manager not installed) from actual resource failures.
func IsNoMatchError(err error) bool {
	var noResource *meta.NoResourceMatchError
	var noKind *meta.NoKindMatchError
	return errors.As(err, &noResource) || errors.As(err, &noKind)
}

// ErrRBACForbidden is returned when an enrichment K8s API call fails with
// 403 Forbidden, indicating the impersonated user lacks RBAC permissions in
// the target namespace. Per BR-INTERACTIVE-002, authorization failures must
// be surfaced as errors — not silently swallowed as partial failures.
var ErrRBACForbidden = errors.New("RBAC: access denied")

// isForbiddenError reports whether the error wraps a K8s API 403 Forbidden.
func isForbiddenError(err error) bool {
	return apierrors.IsForbidden(err)
}

// EnrichRequest groups Enrich's parameters. Introduced per AGENTS.md's
// 8+-param Options-pattern/config-struct rule (D5, Issue #1802) rather than
// adding ClusterID as an 8th positional parameter.
type EnrichRequest struct {
	Kind       string
	Name       string
	Namespace  string
	APIVersion string // disambiguates multi-group kinds (e.g. Route); "" when unknown (Issue #1040)
	SpecHash   string // auto-computed via K8sClient.GetSpecHash when empty
	// ClusterID optionally scopes the DataStorage remediation-history query to
	// a single fleet cluster (Issue #1802, main only). Empty means unscoped
	// (release/v1.5 semantics, or main without fleet tracking configured).
	ClusterID     string
	IncidentID    string
	CorrelationID string
}

// Enrich resolves enrichment data for the given resource.
// Implements partial failure: each sub-call is best-effort.
func (e *Enricher) Enrich(ctx context.Context, req EnrichRequest) (*EnrichmentResult, error) {
	kind, name, namespace, apiVersion, specHash, incidentID := req.Kind, req.Name, req.Namespace, req.APIVersion, req.SpecHash, req.IncidentID
	result := &EnrichmentResult{
		ResourceKind:      kind,
		ResourceName:      name,
		ResourceNamespace: namespace,
		HistoryStatus:     sharedtypes.HistoryStatusNotRequested,
	}

	resolvedSpecHash, specHashUnavailable, err := e.resolveSpecHash(ctx, kind, name, namespace, apiVersion, specHash)
	if err != nil {
		return nil, err
	}
	specHash = strings.TrimSpace(resolvedSpecHash)
	result.HistoryUnavailable = specHashUnavailable
	if specHashUnavailable {
		result.HistoryStatus = sharedtypes.HistoryStatusTransportUnavailable
	}

	ownerErr, hardErr := e.populateOwnerChain(ctx, kind, name, namespace, apiVersion, result)
	if hardErr != nil {
		return nil, hardErr
	}

	if labelErr := e.populateDetectedLabels(ctx, kind, name, namespace, result); labelErr != nil {
		return nil, labelErr
	}

	histResult, histErr, historySkipped := e.fetchHistory(ctx, result, historyFetchRequest{
		Kind: kind, Name: name, Namespace: namespace, ClusterID: req.ClusterID,
		IncidentID: incidentID, SpecHash: specHash,
	})
	if histErr != nil {
		result.HistoryUnavailable = result.HistoryUnavailable || specHash != ""
		if specHash != "" {
			result.HistoryStatus = historyStatusForError(histErr)
		}
		e.logger.Error(histErr, "enrichment: remediation history fetch failed",
			"resource", namespace+"/"+name,
		)
	} else if !historySkipped {
		result.RemediationHistory = histResult
	}

	e.emitEnrichmentAuditEvent(ctx, enrichmentAuditParams{
		Kind: kind, Name: name, Namespace: namespace, IncidentID: incidentID,
		ClusterID: req.ClusterID, CorrelationID: req.CorrelationID,
		Result: result, OwnerErr: ownerErr, HistErr: histErr,
	})
	if histErr == nil {
		switch {
		case specHashUnavailable:
			// A successful empty response cannot erase the failed live-hash
			// acquisition that made target-scoped history unverifiable.
			result.HistoryStatus = sharedtypes.HistoryStatusTransportUnavailable
		case historySkipped:
			result.HistoryStatus = sharedtypes.HistoryStatusNotRequested
		case histResult == nil || (len(histResult.Tier1) == 0 && len(histResult.Tier2) == 0):
			result.HistoryStatus = sharedtypes.HistoryStatusEmpty
		default:
			result.HistoryStatus = sharedtypes.HistoryStatusAvailable
		}
	}
	return result, nil
}

func historyStatusForError(err error) sharedtypes.HistoryStatus {
	if errors.Is(err, ErrHistorySchema) {
		return sharedtypes.HistoryStatusSchemaError
	}
	return sharedtypes.HistoryStatusTransportUnavailable
}

type historyFetchRequest struct {
	Kind, Name, Namespace string
	ClusterID, IncidentID string
	SpecHash              string
}

func (e *Enricher) fetchHistory(ctx context.Context, result *EnrichmentResult, req historyFetchRequest) (*RemediationHistoryResult, error, bool) {
	if req.SpecHash == "" {
		outcome := katypes.EvidenceOutcomeSkipped
		if result != nil && result.HistoryUnavailable {
			// A failed hash acquisition is different from a successful empty
			// hash. Both avoid an invalid DataStorage request, but only the
			// former is an unavailable evidence source.
			outcome = katypes.EvidenceOutcomeTransportUnavailable
		}
		status := historySourceStatus(req.Kind, req.Name, req.Namespace, req.ClusterID, req.IncidentID,
			outcome, katypes.EvidenceFailureCodeHashUnavailable)
		history := &RemediationHistoryResult{EvidenceStatus: historyAggregateStatus(status)}
		result.EvidenceStatus = history.EvidenceStatus
		mergeEvidenceSourceStatus(ctx, status)
		return history, nil, true
	}

	history, err := e.ds.GetRemediationHistory(ctx, req.Kind, req.Name, req.Namespace, req.ClusterID, req.SpecHash)
	if err != nil {
		status := historyStatusFromError(err, req.Kind, req.Name, req.Namespace, req.ClusterID, req.IncidentID)
		result.EvidenceStatus = historyAggregateStatus(status)
		mergeEvidenceSourceStatus(ctx, status)
		return nil, err, false
	}
	if history == nil {
		history = &RemediationHistoryResult{}
	}
	if history.EvidenceStatus == nil {
		outcome := katypes.EvidenceOutcomeSuccess
		if len(history.Tier1) == 0 && len(history.Tier2) == 0 {
			outcome = katypes.EvidenceOutcomeValidEmpty
		}
		status := historySourceStatus(req.Kind, req.Name, req.Namespace, req.ClusterID, req.IncidentID, outcome, "")
		history.EvidenceStatus = historyAggregateStatus(status)
	}
	result.EvidenceStatus = history.EvidenceStatus
	for _, source := range history.EvidenceStatus.Sources {
		mergeEvidenceSourceStatus(ctx, source)
	}
	return history, nil, false
}

// resolveSpecHash returns specHash unchanged if already provided, otherwise
// auto-computes it from the live resource. The unavailable flag distinguishes
// a failed live hash lookup from a successful empty/legacy result so Phase 2
// can fail closed without breaking the best-effort enrichment API.
func (e *Enricher) resolveSpecHash(ctx context.Context, kind, name, namespace, apiVersion, specHash string) (string, bool, error) {
	if strings.TrimSpace(specHash) != "" {
		return strings.TrimSpace(specHash), false, nil
	}
	computed, err := e.effectiveK8s(ctx).GetSpecHash(ctx, kind, name, namespace, apiVersion)
	if err != nil {
		if isForbiddenError(err) {
			forbidden := fmt.Errorf("%w: GetSpecHash %s/%s in %s: %w", ErrRBACForbidden, kind, name, namespace, err)
			return "", true, fmt.Errorf("%w: %w", ErrHistoryUnavailable, forbidden)
		}
		// A deleted target or an unavailable API mapping is sparse evidence,
		// not a history backend outage (DD-KA-006 / Issue #1039).
		if IsNotFoundError(err) || IsNoMatchError(err) {
			e.logger.Info("enrichment: target unavailable while computing specHash; proceeding with empty", "resource", namespace+"/"+kind+"/"+name)
			return "", false, nil
		}
		e.logger.Error(err, "enrichment: specHash auto-computation failed, proceeding with empty",
			"resource", namespace+"/"+kind+"/"+name,
		)
		return "", true, nil
	}
	// An empty hash is a valid "history not queryable" outcome, not by itself
	// an unavailable history backend. The caller records the bounded skipped
	// evidence status and avoids sending an invalid empty hash to DataStorage;
	// HistoryUnavailable remains reserved for an actual failed hash acquisition
	// (for example RBAC or transport failure).
	computed = strings.TrimSpace(computed)
	return computed, false, nil
}

// populateOwnerChain resolves the resource's owner chain and stores it (or
// the resulting error state) on result. Returns ownerErr — the raw
// resolution error, used later for audit reporting — and a separate hardErr
// which is non-nil only for a forbidden error, signaling the caller to abort
// enrichment entirely.
func (e *Enricher) populateOwnerChain(ctx context.Context, kind, name, namespace, apiVersion string, result *EnrichmentResult) (ownerErr, hardErr error) {
	chain, ownerErr := e.resolveOwnerChainWithRetry(ctx, kind, name, namespace, apiVersion)
	if ownerErr == nil {
		result.OwnerChain = chain
		return nil, nil
	}
	if isForbiddenError(ownerErr) {
		return ownerErr, fmt.Errorf("%w: GetOwnerChain %s/%s in %s: %w", ErrRBACForbidden, kind, name, namespace, ownerErr)
	}
	result.OwnerChainError = ownerErr
	if IsNotFoundError(ownerErr) {
		result.TargetResourceDeleted = true
	} else if e.retryConfig.MaxRetries > 0 && !IsNoMatchError(ownerErr) {
		result.HardFail = true
	}
	e.logger.Error(ownerErr, "enrichment: owner chain resolution failed",
		"resource", namespace+"/"+kind+"/"+name,
		"target_resource_deleted", result.TargetResourceDeleted,
	)
	return ownerErr, nil
}

// populateDetectedLabels runs label detection (when a detector is wired) and
// stores the results on result. Returns a non-nil error only for a forbidden
// error, signaling the caller to abort enrichment entirely.
func (e *Enricher) populateDetectedLabels(ctx context.Context, kind, name, namespace string, result *EnrichmentResult) error {
	detector := e.effectiveLabelDetector(ctx)
	if detector == nil {
		return nil
	}
	labels, quotaDetails, labelErr := detector.DetectLabels(ctx, kind, name, namespace, result.OwnerChain)
	if labelErr != nil {
		if isForbiddenError(labelErr) {
			return fmt.Errorf("%w: DetectLabels %s/%s in %s: %w", ErrRBACForbidden, kind, name, namespace, labelErr)
		}
		e.logger.Error(labelErr, "enrichment: label detection failed",
			"resource", namespace+"/"+kind+"/"+name,
		)
	}
	if labels != nil {
		result.DetectedLabels = labels
	}
	if len(quotaDetails) > 0 {
		result.QuotaDetails = quotaDetails
	}
	return nil
}

// enrichmentAuditParams groups the fields needed to emit the enrichment
// audit event. Extracted per AGENTS.md's 8+-param Options-pattern rule.
type enrichmentAuditParams struct {
	Kind, Name, Namespace, IncidentID, ClusterID string
	CorrelationID                                string
	Result                                       *EnrichmentResult
	OwnerErr, HistErr                            error
}

// emitEnrichmentAuditEvent records a best-effort audit event summarizing the
// enrichment outcome: failure when both the owner-chain and history lookups
// errored, success (with any partial error detail) otherwise.
func (e *Enricher) emitEnrichmentAuditEvent(ctx context.Context, p enrichmentAuditParams) {
	kind, name, namespace, incidentID, clusterID := p.Kind, p.Name, p.Namespace, p.IncidentID, p.ClusterID
	result, ownerErr, histErr := p.Result, p.OwnerErr, p.HistErr
	correlationID := p.CorrelationID
	if correlationID == "" {
		correlationID = incidentID
	}
	if correlationID == "" {
		correlationID, _ = audit.CorrelationIDFromContext(ctx)
	}

	if ownerErr != nil && histErr != nil {
		event := audit.NewEvent(audit.EventTypeEnrichmentFailed, correlationID)
		event.EventAction = audit.ActionEnriched
		event.EventOutcome = "failure"
		event.Data["incident_id"] = incidentID
		event.Data["reason"] = "all_enrichment_sources_failed"
		event.Data["detail"] = boundedEnrichmentFailureDetail(ownerErr, histErr)
		event.Data["owner_error_class"] = enrichmentErrorClass(ownerErr)
		event.Data["history_error_class"] = enrichmentErrorClass(histErr)
		event.Data["owner_error_code"] = enrichmentFailureCode(ownerErr)
		event.Data["history_error_code"] = enrichmentFailureCode(histErr)
		event.Data["affected_resource_kind"] = kind
		event.Data["affected_resource_name"] = name
		event.Data["affected_resource_namespace"] = namespace
		event.Data["cluster_id"] = clusterID
		if result != nil && result.EvidenceStatus != nil {
			event.Data["evidence_source"] = string(result.EvidenceStatus.Source)
			event.Data["evidence_outcome"] = string(result.EvidenceStatus.Outcome)
			event.Data["evidence_failure_code"] = result.EvidenceStatus.LastFailureCode
		}
		audit.StoreBestEffort(ctx, e.auditStore, event, e.logger)
		return
	}

	event := audit.NewEvent(audit.EventTypeEnrichmentCompleted, correlationID)
	event.EventAction = audit.ActionEnriched
	event.EventOutcome = "success"
	event.Data["incident_id"] = incidentID
	event.Data["cluster_id"] = clusterID

	rootKind, rootName, rootNS := resolveRootOwner(kind, name, namespace, result.OwnerChain)
	event.Data["root_owner_kind"] = rootKind
	event.Data["root_owner_name"] = rootName
	event.Data["root_owner_namespace"] = rootNS
	event.Data["owner_chain_length"] = len(result.OwnerChain)
	event.Data["remediation_history_fetched"] = histErr == nil && result.RemediationHistory != nil
	if result.DetectedLabels != nil {
		event.Data["detected_labels_summary"] = result.DetectedLabels
		event.Data["failed_detections"] = result.DetectedLabels.FailedDetections
	}

	if ownerErr != nil {
		event.Data["owner_error_code"] = enrichmentFailureCode(ownerErr)
		event.Data["owner_error_class"] = enrichmentErrorClass(ownerErr)
	}
	if histErr != nil {
		event.Data["history_error_code"] = enrichmentFailureCode(histErr)
		event.Data["history_error_class"] = enrichmentErrorClass(histErr)
	}
	if result.EvidenceStatus != nil {
		event.Data["evidence_source"] = string(result.EvidenceStatus.Source)
		event.Data["evidence_outcome"] = string(result.EvidenceStatus.Outcome)
		event.Data["evidence_failure_code"] = result.EvidenceStatus.LastFailureCode
	}
	audit.StoreBestEffort(ctx, e.auditStore, event, e.logger)

}

func enrichmentFailureCode(err error) string {
	if err == nil {
		return ""
	}
	var typed *HistoryFetchError
	if errors.As(err, &typed) && typed != nil && typed.Status.LastFailureCode != "" {
		return typed.Status.LastFailureCode
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return katypes.EvidenceFailureCodeCanceled
	}
	if errors.Is(err, ErrRBACForbidden) {
		return "rbac_forbidden"
	}
	return katypes.EvidenceFailureCodeTransport
}

const enrichmentFailureDetailMaxLen = 128

func boundedEnrichmentFailureDetail(ownerErr, historyErr error) string {
	detail := "owner_chain=" + enrichmentErrorClass(ownerErr) + ";history=" + enrichmentErrorClass(historyErr)
	if len(detail) <= enrichmentFailureDetailMaxLen {
		return detail
	}
	return detail[:enrichmentFailureDetailMaxLen]
}

// enrichmentErrorClass is intentionally an enum-like projection. Persisting
// err.Error() here would leak provider URLs, request bodies, credentials, or
// transport internals into the immutable audit record (DD-KA-2520-2521).
func enrichmentErrorClass(err error) string {
	if err == nil {
		return "none"
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded), apierrors.IsTimeout(err):
		return "timeout"
	case isForbiddenError(err):
		return "forbidden"
	case IsNotFoundError(err):
		return "not_found"
	case IsNoMatchError(err), errors.Is(err, ErrHistorySchema):
		return "schema_error"
	default:
		return "transport"
	}
}

func resolveRootOwner(kind, name, namespace string, chain []OwnerChainEntry) (string, string, string) {
	if len(chain) > 0 {
		root := chain[len(chain)-1]
		return root.Kind, root.Name, root.Namespace
	}
	return kind, name, namespace
}
