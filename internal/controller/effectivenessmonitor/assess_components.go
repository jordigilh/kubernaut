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

package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/alert"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/hash"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/health"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	emtypes "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

// healthAssessResult contains both the component result and the structured TargetStatus
// for populating the health_checks typed sub-object in audit events (DD-017 v2.5).
type healthAssessResult struct {
	Component emtypes.ComponentResult
	Status    health.TargetStatus
}

// assessHealth evaluates the target resource's health via K8s API (BR-EM-001).
func (r *Reconciler) assessHealth(ctx context.Context, ea *eav1.EffectivenessAssessment) healthAssessResult {
	logger := log.FromContext(ctx)

	// Build target status from K8s API (DD-EM-003: health uses RemediationTarget).
	// The remediation target (e.g. Deployment) survives rolling restarts, whereas the
	// signal target (e.g. the original Pod) may be deleted and replaced (#275).
	// Pass RemediationCreatedAt so restarts can be counted relative to remediation time (#246).
	status := r.getTargetHealthStatus(ctx, ea.Spec.RemediationTarget, ea.Spec.RemediationCreatedAt)

	result := r.healthScorer.Score(ctx, status)
	logger.Info("Health assessment complete",
		"score", result.Score,
		"details", result.Details,
	)

	return healthAssessResult{Component: result, Status: status}
}

// assessHash computes the spec hash of the target resource and compares
// with the pre-remediation hash (BR-EM-004, DD-EM-002).
func (r *Reconciler) assessHash(ctx context.Context, ea *eav1.EffectivenessAssessment) hash.ComputeResult {
	logger := log.FromContext(ctx)

	// Step 1: Fetch target spec from K8s API (DD-EM-003: hash uses RemediationTarget)
	spec, postHashDegradedReason := r.getTargetSpec(ctx, ea.Spec.RemediationTarget)

	// Issue #546: Set EA condition based on spec fetch result
	if postHashDegradedReason != "" {
		conditions.SetCondition(ea, conditions.ConditionPostHashCaptured,
			metav1.ConditionFalse, conditions.ReasonPostHashCaptureFailed, postHashDegradedReason)
		logger.Info("Post-remediation spec fetch degraded, hash comparison will be unreliable",
			"degradedReason", postHashDegradedReason)
	} else {
		conditions.SetCondition(ea, conditions.ConditionPostHashCaptured,
			metav1.ConditionTrue, conditions.ReasonPostHashCaptured, "Post-remediation spec hash captured")
	}

	// Step 2: Read pre-remediation hash from EA spec (set by RO via RR status).
	// Falls back to DataStorage query for backward compatibility with EAs created
	// before the RO started populating PreRemediationSpecHash.
	preHash := ea.Spec.PreRemediationSpecHash
	if preHash == "" {
		logger.V(1).Info("PreRemediationSpecHash not in EA spec, falling back to DataStorage query")
		preHash = r.queryPreRemediationHash(ctx, ea.Spec.CorrelationID)
	}

	// Step 3: Resolve ConfigMap content hashes (#396, BR-EM-004)
	configMapHashes := r.resolveConfigMapHashes(ctx, spec, ea.Spec.RemediationTarget)

	// Step 4: Compute post-hash (composite when ConfigMaps present) and compare with pre-hash
	result := r.hashComputer.Compute(hash.SpecHashInput{
		Spec:            spec,
		PreHash:         preHash,
		ConfigMapHashes: configMapHashes,
	})

	if result.Hash != "" {
		logger.Info("Hash computation complete",
			"hash", result.Hash[:min(23, len(result.Hash))]+"...",
			"preHash", preHash,
			"match", result.Match,
		)
	}
	return result
}

// alertAssessResult contains both the component result and the structured alert data
// for populating the alert_resolution typed sub-object in audit events (DD-017 v2.5).
type alertAssessResult struct {
	Component             emtypes.ComponentResult
	AlertResolved         bool
	ActiveCount           int32
	ResolutionTimeSeconds *float64
}

func (r *Reconciler) assessAlert(ctx context.Context, ea *eav1.EffectivenessAssessment) alertAssessResult {
	logger := log.FromContext(ctx)

	// OBS-1: Use SignalName (the actual alert name) when available,
	// falling back to CorrelationID for backward compatibility.
	alertName := ea.Spec.SignalName
	if alertName == "" {
		alertName = ea.Spec.CorrelationID
	}
	alertCtx := alert.AlertContext{
		AlertName: alertName,
		Namespace: ea.Spec.SignalTarget.Namespace,
	}

	// #269: Resolve active pod names from SignalTarget so the scorer can filter
	// out stale alerts for pods deleted during rolling restarts.
	if podNames := r.listActivePodNames(ctx, ea.Spec.SignalTarget); podNames != nil {
		alertCtx.ActivePodNames = podNames
		logger.V(1).Info("Alert pod correlation enabled",
			"signalTarget", ea.Spec.SignalTarget.Name, "activePods", len(podNames))
	}

	result := r.alertScorer.Score(ctx, r.AlertManagerClient, alertCtx)
	logger.Info("Alert assessment complete",
		"score", result.Score,
		"details", result.Details,
	)

	alertResolved := result.Score != nil && *result.Score == 1.0

	var activeCount int32
	if !alertResolved && result.Score != nil {
		activeCount = 1
	}

	// ADR-EM-001 Section 9.2.3: resolution_time_seconds = time from remediation to alert resolution.
	var resolutionTime *float64
	if alertResolved && ea.Spec.RemediationCreatedAt != nil {
		rt := time.Since(ea.Spec.RemediationCreatedAt.Time).Seconds()
		resolutionTime = &rt
	}

	return alertAssessResult{
		Component:             result,
		AlertResolved:         alertResolved,
		ActiveCount:           activeCount,
		ResolutionTimeSeconds: resolutionTime,
	}
}

// isAlertDecay detects Prometheus alert decay: the resource is healthy and spec is stable,
// but the alert is still firing due to Prometheus lookback window lag.
// Returns true only when all conditions are met (Issue #369, BR-EM-012):
//   - Health has been assessed with a positive score (resource is healthy)
//   - Hash has been computed (spec is stable, no drift since remediation)
//   - Metrics (if assessed) are not negative — proactive signal gate (#369 Option D)
//   - The alert was just assessed as still firing (score == 0.0)
func (r *Reconciler) isAlertDecay(ea *eav1.EffectivenessAssessment, ar alertAssessResult) bool {
	if !ea.Status.Components.HealthAssessed || ea.Status.Components.HealthScore == nil || *ea.Status.Components.HealthScore <= 0 {
		return false
	}
	if !ea.Status.Components.HashComputed {
		return false
	}
	if ea.Status.Components.MetricsAssessed && ea.Status.Components.MetricsScore != nil && *ea.Status.Components.MetricsScore <= 0.0 {
		return false
	}
	if !ar.Component.Assessed || ar.Component.Score == nil || *ar.Component.Score != 0.0 {
		return false
	}
	return true
}

// metricsAssessResult contains both the component result and the structured metric deltas
// for populating the metric_deltas typed sub-object in audit events (DD-017 v2.5).
type metricsAssessResult struct {
	Component            emtypes.ComponentResult
	CPUBefore            *float64
	CPUAfter             *float64
	MemoryBefore         *float64
	MemoryAfter          *float64
	LatencyP95BeforeMs   *float64
	LatencyP95AfterMs    *float64
	ErrorRateBefore      *float64
	ErrorRateAfter       *float64
	ThroughputBeforeRPS  *float64
	ThroughputAfterRPS   *float64
}

// metricQuerySpec defines a PromQL query for a single metric type.
type metricQuerySpec struct {
	Name          string
	Query         string
	LowerIsBetter bool
}

// metricQueryResult contains the before/after values from a single metric query.
type metricQueryResult struct {
	Spec       metricQuerySpec
	PreValue   float64
	PostValue  float64
	Available  bool
	QueryError error
}

// assessMetrics compares pre/post remediation metrics (BR-EM-003, DD-017 v2.5 Phase B).
//
// Executes up to 5 independent PromQL queries (CPU, memory, latency p95, error rate, throughput).
// Each query is independent — individual query failures don't prevent overall assessment
// (graceful degradation). The score is the average of all available metric improvements.
func (r *Reconciler) assessMetrics(ctx context.Context, ea *eav1.EffectivenessAssessment) metricsAssessResult {
	logger := log.FromContext(ctx)
	ns := ea.Spec.SignalTarget.Namespace

	start := ea.CreationTimestamp.Add(-r.Config.PrometheusLookback)
	end := time.Now()
	step := 1 * time.Second

	queries := []metricQuerySpec{
		{
			Name:          "container_cpu_usage_seconds_total",
			Query:         fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{namespace="%s"}[5m]))`, ns),
			LowerIsBetter: true,
		},
		{
			Name:          "container_memory_working_set_bytes",
			Query:         fmt.Sprintf(`sum(container_memory_working_set_bytes{namespace="%s"})`, ns),
			LowerIsBetter: true,
		},
		{
			Name:          "http_request_duration_p95_ms",
			Query:         fmt.Sprintf(`histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{namespace="%s"}[5m])) * 1000`, ns),
			LowerIsBetter: true,
		},
		{
			Name:          "http_error_rate",
			Query:         fmt.Sprintf(`sum(rate(http_requests_total{namespace="%s",code=~"5.."}[5m])) / sum(rate(http_requests_total{namespace="%s"}[5m]))`, ns, ns),
			LowerIsBetter: true,
		},
		{
			Name:          "http_throughput_rps",
			Query:         fmt.Sprintf(`sum(rate(http_requests_total{namespace="%s"}[5m]))`, ns),
			LowerIsBetter: false,
		},
	}

	queryResults := make([]metricQueryResult, len(queries))
	for i, spec := range queries {
		queryResults[i] = r.executeMetricQuery(ctx, spec, start, end, step)
	}

	var comparisons []emmetrics.MetricComparison
	for _, qr := range queryResults {
		if qr.Available {
			comparisons = append(comparisons, emmetrics.MetricComparison{
				Name:          qr.Spec.Name,
				PreValue:      qr.PreValue,
				PostValue:     qr.PostValue,
				LowerIsBetter: qr.Spec.LowerIsBetter,
			})
		}
	}

	result := emtypes.ComponentResult{
		Component: emtypes.ComponentMetrics,
	}
	if len(comparisons) == 0 {
		result.Assessed = false
		result.Details = "no metric data available for comparison"
		return metricsAssessResult{Component: result}
	}

	scored := r.metricScorer.Score(comparisons)
	result = scored.Component

	logger.Info("Metrics assessment complete",
		"score", result.Score,
		"queriesAvailable", len(comparisons),
		"queriesTotal", len(queries),
	)

	mr := metricsAssessResult{Component: result}
	for _, qr := range queryResults {
		if !qr.Available {
			continue
		}
		switch qr.Spec.Name {
		case "container_cpu_usage_seconds_total":
			mr.CPUBefore = &qr.PreValue
			mr.CPUAfter = &qr.PostValue
		case "container_memory_working_set_bytes":
			mr.MemoryBefore = &qr.PreValue
			mr.MemoryAfter = &qr.PostValue
		case "http_request_duration_p95_ms":
			mr.LatencyP95BeforeMs = &qr.PreValue
			mr.LatencyP95AfterMs = &qr.PostValue
		case "http_error_rate":
			mr.ErrorRateBefore = &qr.PreValue
			mr.ErrorRateAfter = &qr.PostValue
		case "http_throughput_rps":
			mr.ThroughputBeforeRPS = &qr.PreValue
			mr.ThroughputAfterRPS = &qr.PostValue
		}
	}

	return mr
}

// executeMetricQuery runs a single PromQL range query and extracts before/after values.
// Returns Available=false if the query fails or returns insufficient data (graceful degradation).
func (r *Reconciler) executeMetricQuery(ctx context.Context, spec metricQuerySpec, start, end time.Time, step time.Duration) metricQueryResult {
	logger := log.FromContext(ctx)
	result := metricQueryResult{Spec: spec}

	queryResult, err := r.PrometheusClient.QueryRange(ctx, spec.Query, start, end, step)
	if err != nil {
		logger.V(1).Info("Prometheus query failed (graceful degradation)",
			"metric", spec.Name, "error", err)
		result.QueryError = err
		r.Metrics.RecordExternalCallError("prometheus", "query_range", "query_error")
		return result
	}

	if len(queryResult.Samples) < 2 {
		logger.V(1).Info("Insufficient samples for metric comparison",
			"metric", spec.Name, "samples", len(queryResult.Samples))
		return result
	}

	result.PreValue = queryResult.Samples[0].Value
	result.PostValue = queryResult.Samples[len(queryResult.Samples)-1].Value
	result.Available = true

	return result
}
