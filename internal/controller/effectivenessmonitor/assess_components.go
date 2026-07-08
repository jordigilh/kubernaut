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
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
func (r *Reconciler) assessHealth(ctx context.Context, reader client.Reader, ea *eav1.EffectivenessAssessment) healthAssessResult {
	logger := log.FromContext(ctx)

	// Build target status from K8s API (DD-EM-003: health uses RemediationTarget).
	// The remediation target (e.g. Deployment) survives rolling restarts, whereas the
	// signal target (e.g. the original Pod) may be deleted and replaced (#275).
	// Pass RemediationCreatedAt so restarts can be counted relative to remediation time (#246).
	status := r.getTargetHealthStatus(ctx, reader, ea.Spec.RemediationTarget, ea.Spec.RemediationCreatedAt)

	result := r.healthScorer.Score(ctx, status)
	logger.Info("Health assessment complete",
		"score", result.Score,
		"details", result.Details,
	)

	return healthAssessResult{Component: result, Status: status}
}

// assessHash computes the resource fingerprint and compares
// with the pre-remediation hash (BR-EM-004, DD-EM-002 v2.0, #765).
func (r *Reconciler) assessHash(ctx context.Context, reader client.Reader, ea *eav1.EffectivenessAssessment) hash.ComputeResult {
	logger := log.FromContext(ctx)

	functionalState, spec, postHashDegradedReason := r.getTargetFunctionalState(ctx, reader, ea.Spec.RemediationTarget)

	if postHashDegradedReason != "" {
		conditions.SetCondition(ea, conditions.ConditionPostHashCaptured,
			metav1.ConditionFalse, conditions.ReasonPostHashCaptureFailed, postHashDegradedReason)
		logger.Info("Post-remediation spec fetch degraded, hash comparison will be unreliable",
			"degradedReason", postHashDegradedReason)
	} else {
		conditions.SetCondition(ea, conditions.ConditionPostHashCaptured,
			metav1.ConditionTrue, conditions.ReasonPostHashCaptured, "Post-remediation spec hash captured")
	}

	preHash := ea.Spec.PreRemediationSpecHash
	if preHash == "" {
		logger.V(1).Info("PreRemediationSpecHash not in EA spec, falling back to DataStorage query")
		preHash = r.queryPreRemediationHash(ctx, ea.Spec.CorrelationID)
	}

	configMapHashes := r.resolveConfigMapHashes(ctx, reader, spec, ea.Spec.RemediationTarget)

	result := r.hashComputer.Compute(hash.SpecHashInput{
		FunctionalState: functionalState,
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

func (r *Reconciler) assessAlert(ctx context.Context, reader client.Reader, ea *eav1.EffectivenessAssessment) alertAssessResult {
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

	// Issue #193, DD-EM-005: for cluster-scoped targets (empty Namespace), populate
	// AlertLabels so buildMatchers() scopes the AlertManager query to the specific
	// resource (e.g. node="worker-1"), instead of degrading to alertname-only
	// matching that cannot distinguish "Node A" from "Node B" firing the same alert.
	if ea.Spec.SignalTarget.Namespace == "" {
		if labelKey, ok := clusterScopedAlertLabelKey(ea.Spec.SignalTarget.Kind); ok {
			alertCtx.AlertLabels = map[string]string{labelKey: ea.Spec.SignalTarget.Name}
		}
	}

	// #269: Resolve active pod names from SignalTarget so the scorer can filter
	// out stale alerts for pods deleted during rolling restarts.
	if podNames := r.listActivePodNames(ctx, reader, ea.Spec.SignalTarget); podNames != nil {
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
	Component           emtypes.ComponentResult
	CPUBefore           *float64
	CPUAfter            *float64
	MemoryBefore        *float64
	MemoryAfter         *float64
	LatencyP95BeforeMs  *float64
	LatencyP95AfterMs   *float64
	ErrorRateBefore     *float64
	ErrorRateAfter      *float64
	ThroughputBeforeRPS *float64
	ThroughputAfterRPS  *float64
	// Cluster-scoped fields (Issue #193, DD-EM-005 v1.1): populated only for
	// Node/PersistentVolume targets, nil for namespace-scoped assessments.
	NodeNotReadyBefore       *float64
	NodeNotReadyAfter        *float64
	NodeMemoryPressureBefore *float64
	NodeMemoryPressureAfter  *float64
	NodeDiskPressureBefore   *float64
	NodeDiskPressureAfter    *float64
	PVPhaseFailedBefore      *float64
	PVPhaseFailedAfter       *float64
	PVPhasePendingBefore     *float64
	PVPhasePendingAfter      *float64
	PVUsageRatioBefore       *float64
	PVUsageRatioAfter        *float64
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
	target := ea.Spec.SignalTarget

	start := ea.CreationTimestamp.Add(-r.Config.PrometheusLookback)
	end := time.Now()
	step := 1 * time.Second

	queries := buildMetricQuerySpecsForTarget(target)

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
		result.Details = fmt.Sprintf("no metric data available for comparison (kind=%s, namespace=%q)", target.Kind, target.Namespace)
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
	populateMetricsAssessResult(&mr, queryResults)
	return mr
}

// buildMetricQuerySpecsForTarget dispatches to the correct query-spec builder
// for target: the namespace-scoped 5-metric set when a namespace is present,
// or a Kind-specific cluster-scoped builder when it is empty (Issue #193,
// DD-EM-005). An unrecognized cluster-scoped Kind returns an empty slice,
// which assessMetrics reports via its "no metric data available" fallback.
func buildMetricQuerySpecsForTarget(target eav1.TargetResource) []metricQuerySpec {
	if target.Namespace != "" {
		return buildMetricQuerySpecs(target.Namespace)
	}
	switch target.Kind {
	case "Node":
		return buildNodeMetricQuerySpecs(target.Name)
	case "PersistentVolume":
		return buildPVMetricQuerySpecs(target.Name)
	default:
		return nil
	}
}

// buildMetricQuerySpecs returns the 5 independent PromQL queries (CPU,
// memory, latency p95, error rate, throughput) for namespace ns. Extracted
// from assessMetrics (Wave 6 6a GREEN: funlen remediation) — pure code
// motion, no behavior change.
func buildMetricQuerySpecs(ns string) []metricQuerySpec {
	return []metricQuerySpec{
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
}

// buildKSMFlagQuerySpec builds a metricQuerySpec for a kube-state-metrics
// "badness flag" series: a metric scoped to a resource by resourceLabel="resourceName",
// plus any additional label matchers (e.g. condition/phase), where a value of
// 1 means the resource is in a bad state and 0 means it isn't -- so every
// spec built this way is LowerIsBetter. Shared by buildNodeMetricQuerySpecs
// (Node conditions) and buildPVMetricQuerySpecs (PV phases) to eliminate the
// duplicated Sprintf/struct-literal boilerplate between them (Issue #193
// REFACTOR).
func buildKSMFlagQuerySpec(specName, metric, resourceLabel, resourceName string, extraMatchers ...string) metricQuerySpec {
	matchers := append([]string{fmt.Sprintf(`%s="%s"`, resourceLabel, resourceName)}, extraMatchers...)
	return metricQuerySpec{
		Name:          specName,
		Query:         fmt.Sprintf(`%s{%s}`, metric, strings.Join(matchers, ",")),
		LowerIsBetter: true,
	}
}

// buildNodeMetricQuerySpecs returns deterministic, kube-state-metrics-backed
// PromQL query specs for a cluster-scoped Node target (Issue #193, DD-EM-005).
// Each query tracks a "badness" condition (1 = bad, 0 = good), so all three
// are LowerIsBetter: a firing NotReady/MemoryPressure/DiskPressure condition
// resolving after remediation is an improvement.
func buildNodeMetricQuerySpecs(name string) []metricQuerySpec {
	const metric = "kube_node_status_condition"
	return []metricQuerySpec{
		buildKSMFlagQuerySpec("kube_node_status_condition_ready", metric, "node", name,
			`condition="Ready"`, `status="false"`),
		buildKSMFlagQuerySpec("kube_node_status_condition_memorypressure", metric, "node", name,
			`condition="MemoryPressure"`, `status="true"`),
		buildKSMFlagQuerySpec("kube_node_status_condition_diskpressure", metric, "node", name,
			`condition="DiskPressure"`, `status="true"`),
	}
}

// buildPVMetricQuerySpecs returns deterministic, kube-state-metrics-backed
// PromQL query specs for a cluster-scoped PersistentVolume target (Issue #193,
// DD-EM-005). Failed/Pending phase flags are "badness" conditions
// (LowerIsBetter). The usage ratio joins kubelet_volume_stats_used_bytes
// (PVC-keyed) to kube_persistentvolume_claim_ref (PV-keyed) via label_replace,
// then divides by kube_persistentvolume_capacity_bytes -- verified live
// end-to-end against a real cluster; label names (persistentvolume,
// claim_namespace, name) are STABLE per the upstream kube-state-metrics docs.
// The usage ratio doesn't fit the single-metric "flag" shape above (it's a
// two-metric join), so it remains a standalone literal.
func buildPVMetricQuerySpecs(name string) []metricQuerySpec {
	return []metricQuerySpec{
		buildKSMFlagQuerySpec("kube_persistentvolume_status_phase_failed", "kube_persistentvolume_status_phase", "persistentvolume", name,
			`phase="Failed"`),
		buildKSMFlagQuerySpec("kube_persistentvolume_status_phase_pending", "kube_persistentvolume_status_phase", "persistentvolume", name,
			`phase="Pending"`),
		{
			Name: "kubelet_volume_stats_used_bytes_ratio",
			Query: fmt.Sprintf(
				`(kubelet_volume_stats_used_bytes `+
					`* on(namespace, persistentvolumeclaim) group_left(persistentvolume) `+
					`label_replace(label_replace(kube_persistentvolume_claim_ref{persistentvolume="%s"}, `+
					`"namespace", "$1", "claim_namespace", "(.*)"), `+
					`"persistentvolumeclaim", "$1", "name", "(.*)")) `+
					`/ on(persistentvolume) group_left() kube_persistentvolume_capacity_bytes{persistentvolume="%s"}`,
				name, name),
			LowerIsBetter: true,
		},
	}
}

// clusterScopedAlertLabelKey maps a cluster-scoped SignalTarget Kind to the
// label key that kube-state-metrics/kubernetes-mixin alerting rules carry for
// that resource type in AlertManager (Issue #193, DD-EM-005). Verified live:
// firing alert instances inherit ALL labels from the underlying PromQL query
// result, so a KubeNodeNotReady alert (query: kube_node_status_condition{...})
// carries a "node" label at fire time. ok=false for any Kind without a known
// mapping (assessAlert falls back to alertname-only matching, same as today).
func clusterScopedAlertLabelKey(kind string) (string, bool) {
	switch kind {
	case "Node":
		return "node", true
	case "PersistentVolume":
		return "persistentvolume", true
	default:
		return "", false
	}
}

// populateMetricsAssessResult fills mr's before/after fields from the
// available query results, keyed by metric name. Extracted from
// assessMetrics (Wave 6 6a GREEN: funlen remediation) — pure code motion,
// no behavior change.
func populateMetricsAssessResult(mr *metricsAssessResult, queryResults []metricQueryResult) {
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
		case "kube_node_status_condition_ready":
			mr.NodeNotReadyBefore = &qr.PreValue
			mr.NodeNotReadyAfter = &qr.PostValue
		case "kube_node_status_condition_memorypressure":
			mr.NodeMemoryPressureBefore = &qr.PreValue
			mr.NodeMemoryPressureAfter = &qr.PostValue
		case "kube_node_status_condition_diskpressure":
			mr.NodeDiskPressureBefore = &qr.PreValue
			mr.NodeDiskPressureAfter = &qr.PostValue
		case "kube_persistentvolume_status_phase_failed":
			mr.PVPhaseFailedBefore = &qr.PreValue
			mr.PVPhaseFailedAfter = &qr.PostValue
		case "kube_persistentvolume_status_phase_pending":
			mr.PVPhasePendingBefore = &qr.PreValue
			mr.PVPhasePendingAfter = &qr.PostValue
		case "kubelet_volume_stats_used_bytes_ratio":
			mr.PVUsageRatioBefore = &qr.PreValue
			mr.PVUsageRatioAfter = &qr.PostValue
		}
	}
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
