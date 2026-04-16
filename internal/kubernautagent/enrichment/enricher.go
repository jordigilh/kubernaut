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
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// OwnerChainEntry represents a single entry in a Kubernetes owner chain.
type OwnerChainEntry struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// DetectedLabels holds the structured label detection results matching KA's LabelDetector output.
type DetectedLabels struct {
	FailedDetections         []string `json:"failedDetections"`
	GitOpsManaged            bool     `json:"gitOpsManaged"`
	GitOpsTool               string   `json:"gitOpsTool"`
	PDBProtected             bool     `json:"pdbProtected"`
	HPAEnabled               bool     `json:"hpaEnabled"`
	Stateful                 bool     `json:"stateful"`
	HelmManaged              bool     `json:"helmManaged"`
	NetworkIsolated          bool     `json:"networkIsolated"`
	ServiceMesh              string   `json:"serviceMesh"`
	ResourceQuotaConstrained bool     `json:"resourceQuotaConstrained"`
}

// K8sClient abstracts Kubernetes API access for enrichment.
type K8sClient interface {
	GetOwnerChain(ctx context.Context, kind, name, namespace string) ([]OwnerChainEntry, error)
	GetSpecHash(ctx context.Context, kind, name, namespace string) (string, error)
}

// DataStorageClient abstracts DataStorage API access for enrichment.
type DataStorageClient interface {
	GetRemediationHistory(ctx context.Context, kind, name, namespace, specHash string) (*RemediationHistoryResult, error)
}

// RemediationHistoryResult holds the full DS response mapped to domain types.
type RemediationHistoryResult struct {
	TargetResource     string          `json:"target_resource"`
	RegressionDetected bool            `json:"regression_detected"`
	Tier1              []Tier1Entry    `json:"tier1"`
	Tier1Window        string          `json:"tier1_window"`
	Tier2              []Tier2Summary  `json:"tier2"`
	Tier2Window        string          `json:"tier2_window"`
}

// Tier1Entry is a detailed remediation history record (recent window).
type Tier1Entry struct {
	RemediationUID          string        `json:"remediation_uid"`
	SignalType              string        `json:"signal_type,omitempty"`
	ActionType              string        `json:"action_type,omitempty"`
	Outcome                 string        `json:"outcome,omitempty"`
	EffectivenessScore      *float64      `json:"effectiveness_score,omitempty"`
	SignalResolved          *bool         `json:"signal_resolved,omitempty"`
	HashMatch               string        `json:"hash_match,omitempty"`
	PreRemediationSpecHash  string        `json:"pre_remediation_spec_hash,omitempty"`
	PostRemediationSpecHash string        `json:"post_remediation_spec_hash,omitempty"`
	HealthChecks            *HealthChecks `json:"health_checks,omitempty"`
	MetricDeltas            *MetricDeltas `json:"metric_deltas,omitempty"`
	AssessmentReason        string        `json:"assessment_reason,omitempty"`
	CompletedAt             time.Time     `json:"completed_at"`
}

// Tier2Summary is a compact historical remediation record (wider window).
type Tier2Summary struct {
	RemediationUID     string   `json:"remediation_uid"`
	SignalType         string   `json:"signal_type,omitempty"`
	ActionType         string   `json:"action_type,omitempty"`
	Outcome            string   `json:"outcome,omitempty"`
	EffectivenessScore *float64 `json:"effectiveness_score,omitempty"`
	SignalResolved     *bool    `json:"signal_resolved,omitempty"`
	HashMatch          string   `json:"hash_match,omitempty"`
	AssessmentReason   string   `json:"assessment_reason,omitempty"`
	CompletedAt        time.Time `json:"completed_at"`
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
	CpuBefore         *float64 `json:"cpu_before,omitempty"`
	CpuAfter          *float64 `json:"cpu_after,omitempty"`
	MemoryBefore      *float64 `json:"memory_before,omitempty"`
	MemoryAfter       *float64 `json:"memory_after,omitempty"`
	LatencyP95BeforeMs *float64 `json:"latency_p95_before_ms,omitempty"`
	LatencyP95AfterMs  *float64 `json:"latency_p95_after_ms,omitempty"`
	ErrorRateBefore   *float64 `json:"error_rate_before,omitempty"`
	ErrorRateAfter    *float64 `json:"error_rate_after,omitempty"`
}

// RetryConfig controls retry behavior for infrastructure calls in Enrich().
// With MaxRetries=0 (default), enrichment is best-effort (current behavior).
// With MaxRetries>0, transient errors are retried with exponential backoff
// and permanent failures trigger HardFail on EnrichmentResult.
type RetryConfig struct {
	MaxRetries  int
	BaseBackoff time.Duration
}

// EnrichmentResult is the combined enrichment data.
type EnrichmentResult struct {
	ResourceKind      string                   `json:"resource_kind,omitempty"`
	ResourceName      string                   `json:"resource_name,omitempty"`
	ResourceNamespace string                   `json:"resource_namespace,omitempty"`
	OwnerChain        []OwnerChainEntry        `json:"owner_chain"`
	// OwnerChainError is non-nil when GetOwnerChain fails (resource not found, API error).
	// Set for observability regardless of retry mode.
	OwnerChainError   error                    `json:"-"`
	// HardFail is true when owner chain resolution fails definitively:
	// either a permanent error (NotFound, Forbidden) or transient error
	// after retry exhaustion. The investigator uses this to trigger
	// rca_incomplete (BR-HAPI-261 AC#7, #704). Only set when RetryConfig
	// has MaxRetries > 0 (strict enrichment mode).
	HardFail          bool                     `json:"-"`
	DetectedLabels    *DetectedLabels          `json:"detected_labels,omitempty"`
	QuotaDetails      map[string]string        `json:"quota_details,omitempty"`
	RemediationHistory *RemediationHistoryResult `json:"remediation_history,omitempty"`
}

// Enricher resolves owner chain, labels, and remediation history.
type Enricher struct {
	k8s           K8sClient
	ds            DataStorageClient
	auditStore    audit.AuditStore
	logger        *slog.Logger
	labelDetector *LabelDetector
	retryConfig   RetryConfig
}

// NewEnricher creates an enricher with the given clients.
func NewEnricher(k8s K8sClient, ds DataStorageClient, auditStore audit.AuditStore, logger *slog.Logger) *Enricher {
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

// WithRetryConfig sets the retry policy for infrastructure calls.
func (e *Enricher) WithRetryConfig(cfg RetryConfig) *Enricher {
	e.retryConfig = cfg
	return e
}

// resolveOwnerChainWithRetry calls GetOwnerChain with optional retry logic.
// With MaxRetries=0 (default): single call, best-effort.
// With MaxRetries>0: transient errors are retried with exponential backoff;
// permanent errors return immediately without retry. The caller sets HardFail
// on EnrichmentResult based on whether this returns a non-nil error.
func (e *Enricher) resolveOwnerChainWithRetry(ctx context.Context, kind, name, namespace string) ([]OwnerChainEntry, error) {
	chain, err := e.k8s.GetOwnerChain(ctx, kind, name, namespace)
	if err == nil {
		return chain, nil
	}

	if e.retryConfig.MaxRetries == 0 {
		return nil, err
	}

	if !isTransientK8sError(err) {
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
			slog.Int("attempt", attempt),
			slog.Duration("backoff", wait),
		)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}

		chain, err = e.k8s.GetOwnerChain(ctx, kind, name, namespace)
		if err == nil {
			return chain, nil
		}

		if !isTransientK8sError(err) {
			return nil, err
		}
	}

	return nil, err
}

// isTransientK8sError classifies K8s API errors as retryable (transient) or
// permanent. NotFound, Forbidden, Gone, etc. are permanent. Timeout, 503,
// 500, and 429 are transient. Non-StatusError types are treated as permanent.
func isTransientK8sError(err error) bool {
	return apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsInternalError(err) ||
		apierrors.IsTooManyRequests(err)
}

// Enrich resolves enrichment data for the given resource.
// Implements partial failure: each sub-call is best-effort.
// If specHash is empty, auto-computes it via K8sClient.GetSpecHash.
func (e *Enricher) Enrich(ctx context.Context, kind, name, namespace, specHash, incidentID string) (*EnrichmentResult, error) {
	result := &EnrichmentResult{
		ResourceKind:      kind,
		ResourceName:      name,
		ResourceNamespace: namespace,
	}

	if specHash == "" {
		computed, err := e.k8s.GetSpecHash(ctx, kind, name, namespace)
		if err != nil {
			e.logger.Warn("enrichment: specHash auto-computation failed, proceeding with empty",
				slog.String("resource", namespace+"/"+kind+"/"+name),
				slog.String("error", err.Error()),
			)
		} else {
			specHash = computed
		}
	}

	var ownerErr, histErr error

	chain, ownerErr := e.resolveOwnerChainWithRetry(ctx, kind, name, namespace)
	if ownerErr != nil {
		result.OwnerChainError = ownerErr
		if e.retryConfig.MaxRetries > 0 {
			result.HardFail = true
		}
		e.logger.Warn("enrichment: owner chain resolution failed",
			slog.String("resource", namespace+"/"+kind+"/"+name),
			slog.String("error", ownerErr.Error()),
		)
	} else {
		result.OwnerChain = chain
	}

	if e.labelDetector != nil {
		labels, labelErr := e.labelDetector.DetectLabels(ctx, kind, name, namespace, result.OwnerChain)
		if labelErr != nil {
			e.logger.Warn("enrichment: label detection failed",
				slog.String("resource", namespace+"/"+kind+"/"+name),
				slog.String("error", labelErr.Error()),
			)
		}
		if labels != nil {
			result.DetectedLabels = labels
		}
	}

	histResult, err := e.ds.GetRemediationHistory(ctx, kind, name, namespace, specHash)
	if err != nil {
		histErr = err
		e.logger.Warn("enrichment: remediation history fetch failed",
			slog.String("resource", namespace+"/"+name),
			slog.String("error", err.Error()),
		)
	} else {
		result.RemediationHistory = histResult
	}

	eventID := uuid.New().String()
	correlationID := incidentID
	if correlationID == "" {
		correlationID = eventID
	}

	if ownerErr != nil && histErr != nil {
		event := audit.NewEvent(audit.EventTypeEnrichmentFailed, correlationID)
		event.EventAction = "enriched"
		event.EventOutcome = "failure"
		event.Data["event_id"] = eventID
		event.Data["incident_id"] = incidentID
		event.Data["reason"] = "all_enrichment_sources_failed"
		event.Data["detail"] = "owner_chain: " + ownerErr.Error() + "; history: " + histErr.Error()
		event.Data["affected_resource_kind"] = kind
		event.Data["affected_resource_name"] = name
		event.Data["affected_resource_namespace"] = namespace
		audit.StoreBestEffort(ctx, e.auditStore, event, e.logger)
	} else {
		event := audit.NewEvent(audit.EventTypeEnrichmentCompleted, correlationID)
		event.EventAction = "enriched"
		event.EventOutcome = "success"
		event.Data["event_id"] = eventID
		event.Data["incident_id"] = incidentID

		rootKind, rootName, rootNS := resolveRootOwner(kind, name, namespace, result.OwnerChain)
		event.Data["root_owner_kind"] = rootKind
		event.Data["root_owner_name"] = rootName
		event.Data["root_owner_namespace"] = rootNS
		event.Data["owner_chain_length"] = len(result.OwnerChain)
		event.Data["remediation_history_fetched"] = histErr == nil

		if ownerErr != nil {
			event.Data["owner_error"] = ownerErr.Error()
		}
		if histErr != nil {
			event.Data["history_error"] = histErr.Error()
		}
		audit.StoreBestEffort(ctx, e.auditStore, event, e.logger)
	}

	return result, nil
}

func resolveRootOwner(kind, name, namespace string, chain []OwnerChainEntry) (string, string, string) {
	if len(chain) > 0 {
		root := chain[len(chain)-1]
		return root.Kind, root.Name, root.Namespace
	}
	return kind, name, namespace
}
