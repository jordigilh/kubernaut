# Phase 2 (P1) Implementation Guide: High Priority Enhancements

**Date**: October 8, 2025
**Status**: ✅ **APPROVED** - Ready for planning
**Estimated Time**: 3 hours
**Priority**: HIGH PRIORITY (Recommended for V1)
**Dependencies**: Phase 1 (P0) must be implemented first

---

## 🎯 **Implementation Objectives**

**Goal**: Add monitoring and business context to RemediationProcessing.status to enable:
1. Signal correlation across related signals
2. Business-aware approval policies based on criticality/SLA
3. Enhanced AI investigation with metrics and logs context

**Success Criteria**:
1. ✅ RemediationProcessing.status includes MonitoringContext
2. ✅ RemediationProcessing.status includes BusinessContext
3. ✅ AIAnalysis can correlate with related signals
4. ✅ Approval policies use business metadata (owner, criticality, SLA)

---

## 📋 **Implementation Tasks Breakdown**

### **Task 1: Add MonitoringContext to RemediationProcessing.status** (2 hours)

**File**: `api/remediationprocessing/v1/remediationprocessing_types.go`

**Changes**: Add MonitoringContext field + 3 supporting types

#### **Current State** (Without Monitoring Context):
```go
type EnrichmentResults struct {
    OriginalSignal    *OriginalSignal    `json:"originalSignal"`
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`

    // ❌ MISSING: Monitoring context for signal correlation

    EnrichmentQuality float64 `json:"enrichmentQuality,omitempty"`
}
```

#### **Target State** (With Monitoring Context):
```go
type EnrichmentResults struct {
    OriginalSignal    *OriginalSignal    `json:"originalSignal"`
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`

    // ✅ ADD: Monitoring context for correlation
    MonitoringContext *MonitoringContext `json:"monitoringContext,omitempty"`

    EnrichmentQuality float64 `json:"enrichmentQuality,omitempty"`
}

// ✅ ADD: New type - Monitoring context container
type MonitoringContext struct {
    // Related signals for correlation (e.g., multiple alerts from same pod)
    RelatedSignals []RelatedSignal `json:"relatedSignals,omitempty"`

    // Recent metric samples for analysis (optional in V1, populated if available)
    Metrics []MetricSample `json:"metrics,omitempty"`

    // Recent log entries for investigation (optional in V1, populated if available)
    Logs []LogEntry `json:"logs,omitempty"`

    // Context quality score (0.0-1.0)
    ContextQuality float64 `json:"contextQuality,omitempty"`
}

// ✅ ADD: New type - Related signal summary
type RelatedSignal struct {
    // Signal identification
    Fingerprint string `json:"fingerprint"`
    Name        string `json:"name"`
    Severity    string `json:"severity"`

    // Temporal context
    FiringTime metav1.Time `json:"firingTime"`

    // Correlation metadata
    Labels      map[string]string `json:"labels,omitempty"`
    CorrelationScore float64      `json:"correlationScore,omitempty"` // 0.0-1.0
}

// ✅ ADD: New type - Metric sample
type MetricSample struct {
    // Metric identification
    Name      string            `json:"name"`
    Labels    map[string]string `json:"labels,omitempty"`

    // Metric value and timestamp
    Value     float64     `json:"value"`
    Timestamp metav1.Time `json:"timestamp"`

    // Metric metadata
    Unit        string `json:"unit,omitempty"` // e.g., "bytes", "percent", "count"
    Description string `json:"description,omitempty"`
}

// ✅ ADD: New type - Log entry
type LogEntry struct {
    // Log identification
    Source    string      `json:"source"` // e.g., "pod/payment-api-abc123"
    Timestamp metav1.Time `json:"timestamp"`

    // Log content
    Level   string `json:"level"`   // e.g., "ERROR", "WARN", "INFO"
    Message string `json:"message"` // Log message content

    // Structured fields (optional)
    Fields map[string]string `json:"fields,omitempty"`
}
```

#### **Subtasks**:

##### **1.1 Update Go Types** (10 min)
- File: `api/remediationprocessing/v1/remediationprocessing_types.go`
- Add `MonitoringContext` field to `EnrichmentResults`
- Add 4 new types: `MonitoringContext`, `RelatedSignal`, `MetricSample`, `LogEntry`
- Run: `make generate`

##### **1.2 Implement Signal Correlation Logic** (45 min)
- File: `internal/controller/remediationprocessing/correlation.go` (new file)
- Implement signal correlation finder:

```go
package remediationprocessing

import (
    "context"
    "time"

    remediationprocessingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// findRelatedSignals finds signals correlated with current signal
// Correlation criteria:
// 1. Same namespace
// 2. Same target resource (kind + name)
// 3. Within last 15 minutes
// 4. Different fingerprint (not the same signal)
func (r *RemediationProcessingReconciler) findRelatedSignals(
    ctx context.Context,
    remProcessing *remediationprocessingv1.RemediationProcessing,
) ([]remediationprocessingv1.RelatedSignal, error) {

    // Query RemediationRequest CRDs for correlation
    var remRequests remediationv1.RemediationRequestList
    err := r.List(ctx, &remRequests,
        client.InNamespace(remProcessing.Namespace),
        client.MatchingLabels{
            "kubernaut.io/target-namespace": remProcessing.Spec.TargetResource.Namespace,
        },
    )
    if err != nil {
        return nil, err
    }

    relatedSignals := []remediationprocessingv1.RelatedSignal{}
    currentTime := time.Now()
    correlationWindow := 15 * time.Minute

    for _, remReq := range remRequests.Items {
        // Skip self
        if remReq.Spec.SignalFingerprint == remProcessing.Spec.SignalFingerprint {
            continue
        }

        // Check temporal correlation (within 15 minutes)
        firingTime := remReq.Spec.FiringTime.Time
        if currentTime.Sub(firingTime) > correlationWindow {
            continue
        }

        // Check resource correlation (same target)
        if !isSameTarget(remReq, remProcessing) {
            continue
        }

        // Calculate correlation score (based on label similarity)
        score := calculateCorrelationScore(remReq.Spec.SignalLabels, remProcessing.Spec.SignalLabels)

        relatedSignals = append(relatedSignals, remediationprocessingv1.RelatedSignal{
            Fingerprint:      remReq.Spec.SignalFingerprint,
            Name:             remReq.Spec.SignalName,
            Severity:         remReq.Spec.Severity,
            FiringTime:       remReq.Spec.FiringTime,
            Labels:           remReq.Spec.SignalLabels,
            CorrelationScore: score,
        })
    }

    return relatedSignals, nil
}

func isSameTarget(remReq *remediationv1.RemediationRequest, remProcessing *remediationprocessingv1.RemediationProcessing) bool {
    // Extract target from remReq provider data
    // Compare with remProcessing.Spec.TargetResource
    // For V1: Simple namespace + resource kind/name match

    targetNamespace := remReq.Spec.SignalLabels["namespace"]
    if targetNamespace != remProcessing.Spec.TargetResource.Namespace {
        return false
    }

    // Check if targeting same resource (e.g., same pod name)
    targetName := remReq.Spec.SignalLabels["pod"]
    if targetName == "" {
        targetName = remReq.Spec.SignalLabels["deployment"]
    }

    return targetName == remProcessing.Spec.TargetResource.Name
}

func calculateCorrelationScore(labels1, labels2 map[string]string) float64 {
    if len(labels1) == 0 || len(labels2) == 0 {
        return 0.0
    }

    // Calculate Jaccard similarity: intersection / union
    intersection := 0
    union := len(labels1)

    for key, val1 := range labels1 {
        if val2, exists := labels2[key]; exists {
            if val1 == val2 {
                intersection++
            }
        } else {
            union++
        }
    }

    // Add labels only in labels2
    for key := range labels2 {
        if _, exists := labels1[key]; !exists {
            union++
        }
    }

    if union == 0 {
        return 0.0
    }

    return float64(intersection) / float64(union)
}
```

##### **1.3 Implement Metrics Collection (Optional for V1)** (30 min)
- File: `internal/controller/remediationprocessing/metrics_collector.go` (new file)
- Implement metrics collection interface:

```go
package remediationprocessing

import (
    "context"

    remediationprocessingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
)

// MetricsCollector interface for fetching related metrics
// V1 Implementation: Optional - can return empty array
// V2 Implementation: Required - query Prometheus/Context API
type MetricsCollector interface {
    CollectMetrics(ctx context.Context, target ResourceTarget, timeWindow TimeWindow) ([]remediationprocessingv1.MetricSample, error)
}

type ResourceTarget struct {
    Namespace string
    Kind      string
    Name      string
}

type TimeWindow struct {
    Start time.Time
    End   time.Time
}

// V1 Implementation: No-op metrics collector
type NoOpMetricsCollector struct{}

func (n *NoOpMetricsCollector) CollectMetrics(ctx context.Context, target ResourceTarget, timeWindow TimeWindow) ([]remediationprocessingv1.MetricSample, error) {
    // V1: Return empty - metrics collection optional
    return []remediationprocessingv1.MetricSample{}, nil
}

// V2 Implementation: Prometheus metrics collector (placeholder)
type PrometheusMetricsCollector struct {
    PrometheusURL string
}

func (p *PrometheusMetricsCollector) CollectMetrics(ctx context.Context, target ResourceTarget, timeWindow TimeWindow) ([]remediationprocessingv1.MetricSample, error) {
    // V2: Query Prometheus for related metrics
    // Examples:
    // - container_memory_usage_bytes{pod="payment-api-abc123"}
    // - container_cpu_usage_seconds{pod="payment-api-abc123"}
    // - kube_pod_status_phase{pod="payment-api-abc123"}

    // For V1: Not implemented
    return []remediationprocessingv1.MetricSample{}, nil
}
```

##### **1.4 Implement Log Collection (Optional for V1)** (30 min)
- File: `internal/controller/remediationprocessing/log_collector.go` (new file)
- Implement log collection interface:

```go
package remediationprocessing

import (
    "context"

    remediationprocessingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
)

// LogCollector interface for fetching related logs
// V1 Implementation: Optional - can return empty array
// V2 Implementation: Required - query Kubernetes API / Loki
type LogCollector interface {
    CollectLogs(ctx context.Context, target ResourceTarget, timeWindow TimeWindow, maxLines int) ([]remediationprocessingv1.LogEntry, error)
}

// V1 Implementation: No-op log collector
type NoOpLogCollector struct{}

func (n *NoOpLogCollector) CollectLogs(ctx context.Context, target ResourceTarget, timeWindow TimeWindow, maxLines int) ([]remediationprocessingv1.LogEntry, error) {
    // V1: Return empty - log collection optional
    return []remediationprocessingv1.LogEntry{}, nil
}

// V2 Implementation: Kubernetes log collector (placeholder)
type KubernetesLogCollector struct {
    K8sClient client.Client
}

func (k *KubernetesLogCollector) CollectLogs(ctx context.Context, target ResourceTarget, timeWindow TimeWindow, maxLines int) ([]remediationprocessingv1.LogEntry, error) {
    // V2: Query Kubernetes API for pod logs
    // Use: kubectl logs <pod> --since-time=<start> --tail=<maxLines>

    // For V1: Not implemented
    return []remediationprocessingv1.LogEntry{}, nil
}
```

##### **1.5 Update RemediationProcessor Controller** (15 min)
- File: `internal/controller/remediationprocessing/enrichment.go`
- Add MonitoringContext population to enrichment phase:

```go
func (r *RemediationProcessingReconciler) enrichSignal(
    ctx context.Context,
    remProcessing *remediationprocessingv1.RemediationProcessing,
) error {

    // ... existing enrichment logic ...

    // ✅ ADD: Build monitoring context
    monitoringContext, err := r.buildMonitoringContext(ctx, remProcessing)
    if err != nil {
        log.Error(err, "Failed to build monitoring context (non-critical)")
        // Non-critical: Continue with empty monitoring context
        monitoringContext = &remediationprocessingv1.MonitoringContext{
            ContextQuality: 0.0,
        }
    }

    // ✅ ADD: Populate monitoring context in status
    remProcessing.Status.EnrichmentResults.MonitoringContext = monitoringContext

    return r.Status().Update(ctx, remProcessing)
}

func (r *RemediationProcessingReconciler) buildMonitoringContext(
    ctx context.Context,
    remProcessing *remediationprocessingv1.RemediationProcessing,
) (*remediationprocessingv1.MonitoringContext, error) {

    // Find related signals (REQUIRED)
    relatedSignals, err := r.findRelatedSignals(ctx, remProcessing)
    if err != nil {
        return nil, err
    }

    // Collect metrics (OPTIONAL in V1)
    target := ResourceTarget{
        Namespace: remProcessing.Spec.TargetResource.Namespace,
        Kind:      remProcessing.Spec.TargetResource.Kind,
        Name:      remProcessing.Spec.TargetResource.Name,
    }
    timeWindow := TimeWindow{
        Start: time.Now().Add(-15 * time.Minute),
        End:   time.Now(),
    }
    metrics, _ := r.metricsCollector.CollectMetrics(ctx, target, timeWindow)

    // Collect logs (OPTIONAL in V1)
    logs, _ := r.logCollector.CollectLogs(ctx, target, timeWindow, 50) // Last 50 lines

    // Calculate context quality
    quality := calculateContextQuality(relatedSignals, metrics, logs)

    return &remediationprocessingv1.MonitoringContext{
        RelatedSignals: relatedSignals,
        Metrics:        metrics,
        Logs:           logs,
        ContextQuality: quality,
    }, nil
}

func calculateContextQuality(relatedSignals []remediationprocessingv1.RelatedSignal, metrics []remediationprocessingv1.MetricSample, logs []remediationprocessingv1.LogEntry) float64 {
    // Simple quality scoring:
    // - Related signals: 0.5 weight
    // - Metrics: 0.3 weight
    // - Logs: 0.2 weight

    signalScore := 0.0
    if len(relatedSignals) > 0 {
        signalScore = 1.0
    }

    metricScore := 0.0
    if len(metrics) > 0 {
        metricScore = 1.0
    }

    logScore := 0.0
    if len(logs) > 0 {
        logScore = 1.0
    }

    return (signalScore * 0.5) + (metricScore * 0.3) + (logScore * 0.2)
}
```

##### **1.6 Update CRD Schema Documentation** (10 min)
- File: `docs/architecture/CRD_SCHEMAS.md`
- Document MonitoringContext and supporting types

**Validation**:
```bash
# Test monitoring context population
make test-processor-monitoring

# Verify related signals found
kubectl get remediationprocessing <name> -o yaml | grep -A 10 "monitoringContext:"

# Check correlation
kubectl get remediationprocessing <name> -o jsonpath='{.status.enrichmentResults.monitoringContext.relatedSignals[*].fingerprint}'
```

---

### **Task 2: Add BusinessContext to RemediationProcessing.status** (1 hour)

**File**: `api/remediationprocessing/v1/remediationprocessing_types.go`

**Changes**: Add BusinessContext field + 1 supporting type

#### **Current State** (Without Business Context):
```go
type EnrichmentResults struct {
    OriginalSignal    *OriginalSignal    `json:"originalSignal"`
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    MonitoringContext *MonitoringContext `json:"monitoringContext,omitempty"`

    // ❌ MISSING: Business context for approval policies

    EnrichmentQuality float64 `json:"enrichmentQuality,omitempty"`
}
```

#### **Target State** (With Business Context):
```go
type EnrichmentResults struct {
    OriginalSignal    *OriginalSignal    `json:"originalSignal"`
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    MonitoringContext *MonitoringContext `json:"monitoringContext,omitempty"`

    // ✅ ADD: Business context for approval policies
    BusinessContext *BusinessContext `json:"businessContext,omitempty"`

    EnrichmentQuality float64 `json:"enrichmentQuality,omitempty"`
}

// ✅ ADD: New type - Business context
type BusinessContext struct {
    // Service ownership
    ServiceOwner string `json:"serviceOwner,omitempty"` // e.g., "payments-team"
    ContactInfo  string `json:"contactInfo,omitempty"`  // e.g., "payments-team@company.com"

    // Business criticality
    Criticality string `json:"criticality,omitempty"` // e.g., "P0", "P1", "P2", "P3"
    SLA         string `json:"sla,omitempty"`         // e.g., "5m", "15m", "30m", "1h"

    // Cost and project metadata
    CostCenter  string `json:"costCenter,omitempty"`  // e.g., "payments-org"
    ProjectName string `json:"projectName,omitempty"` // e.g., "checkout-v2"

    // Business impact
    ImpactLevel string `json:"impactLevel,omitempty"` // e.g., "critical", "high", "medium", "low"
}
```

#### **Subtasks**:

##### **2.1 Update Go Types** (5 min)
- File: `api/remediationprocessing/v1/remediationprocessing_types.go`
- Add `BusinessContext` field to `EnrichmentResults`
- Add `BusinessContext` type definition
- Run: `make generate`

##### **2.2 Implement Business Context Extraction** (30 min)
- File: `internal/controller/remediationprocessing/business_context.go` (new file)
- Implement business context extraction from namespace labels/annotations:

```go
package remediationprocessing

import (
    "context"

    remediationprocessingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// extractBusinessContext extracts business metadata from namespace labels/annotations
// Extraction sources:
// 1. Namespace labels (e.g., kubernaut.io/owner, kubernaut.io/criticality)
// 2. Namespace annotations (e.g., kubernaut.io/sla, kubernaut.io/cost-center)
// 3. ConfigMaps (e.g., kubernaut-business-metadata in namespace)
func (r *RemediationProcessingReconciler) extractBusinessContext(
    ctx context.Context,
    namespace string,
) (*remediationprocessingv1.BusinessContext, error) {

    // Fetch namespace
    var ns corev1.Namespace
    err := r.Get(ctx, client.ObjectKey{Name: namespace}, &ns)
    if err != nil {
        return nil, err
    }

    businessCtx := &remediationprocessingv1.BusinessContext{
        // Extract from namespace labels
        ServiceOwner: ns.Labels["kubernaut.io/owner"],
        Criticality:  ns.Labels["kubernaut.io/criticality"],
        CostCenter:   ns.Labels["kubernaut.io/cost-center"],
        ProjectName:  ns.Labels["kubernaut.io/project"],

        // Extract from namespace annotations
        ContactInfo: ns.Annotations["kubernaut.io/contact"],
        SLA:         ns.Annotations["kubernaut.io/sla"],
        ImpactLevel: ns.Annotations["kubernaut.io/impact-level"],
    }

    // Try to load additional metadata from ConfigMap (optional)
    businessCtx = r.enrichFromConfigMap(ctx, namespace, businessCtx)

    return businessCtx, nil
}

func (r *RemediationProcessingReconciler) enrichFromConfigMap(
    ctx context.Context,
    namespace string,
    businessCtx *remediationprocessingv1.BusinessContext,
) *remediationprocessingv1.BusinessContext {

    // Try to fetch kubernaut-business-metadata ConfigMap
    var cm corev1.ConfigMap
    err := r.Get(ctx, client.ObjectKey{
        Name:      "kubernaut-business-metadata",
        Namespace: namespace,
    }, &cm)

    if err != nil {
        // ConfigMap not found - use namespace metadata only
        return businessCtx
    }

    // Override with ConfigMap data if present
    if owner := cm.Data["owner"]; owner != "" {
        businessCtx.ServiceOwner = owner
    }
    if contact := cm.Data["contact"]; contact != "" {
        businessCtx.ContactInfo = contact
    }
    if criticality := cm.Data["criticality"]; criticality != "" {
        businessCtx.Criticality = criticality
    }
    if sla := cm.Data["sla"]; sla != "" {
        businessCtx.SLA = sla
    }
    if costCenter := cm.Data["cost-center"]; costCenter != "" {
        businessCtx.CostCenter = costCenter
    }
    if project := cm.Data["project"]; project != "" {
        businessCtx.ProjectName = project
    }
    if impact := cm.Data["impact-level"]; impact != "" {
        businessCtx.ImpactLevel = impact
    }

    return businessCtx
}
```

##### **2.3 Update RemediationProcessor Controller** (15 min)
- File: `internal/controller/remediationprocessing/enrichment.go`
- Add BusinessContext extraction to enrichment phase:

```go
func (r *RemediationProcessingReconciler) enrichSignal(
    ctx context.Context,
    remProcessing *remediationprocessingv1.RemediationProcessing,
) error {

    // ... existing enrichment logic ...

    // ✅ ADD: Extract business context
    businessContext, err := r.extractBusinessContext(ctx, remProcessing.Spec.TargetResource.Namespace)
    if err != nil {
        log.Error(err, "Failed to extract business context (non-critical)")
        // Non-critical: Continue with empty business context
        businessContext = &remediationprocessingv1.BusinessContext{}
    }

    // ✅ ADD: Populate business context in status
    remProcessing.Status.EnrichmentResults.BusinessContext = businessContext

    return r.Status().Update(ctx, remProcessing)
}
```

##### **2.4 Update CRD Schema Documentation** (10 min)
- File: `docs/architecture/CRD_SCHEMAS.md`
- Document BusinessContext type and extraction logic

**Validation**:
```bash
# Test business context extraction
make test-processor-business-context

# Verify business context populated
kubectl get remediationprocessing <name> -o yaml | grep -A 10 "businessContext:"

# Check specific fields
kubectl get remediationprocessing <name> -o jsonpath='{.status.enrichmentResults.businessContext.serviceOwner}'
kubectl get remediationprocessing <name> -o jsonpath='{.status.enrichmentResults.businessContext.criticality}'
```

---

## 🧪 **Testing Strategy**

### **Unit Tests** (Parallel with implementation)

#### **Test 1: Signal Correlation**
```go
// File: internal/controller/remediationprocessing/correlation_test.go
func TestFindRelatedSignals_SameNamespace(t *testing.T) {
    // Create 3 RemediationRequest CRDs in same namespace
    remReq1 := createRemediationRequest("high-memory", "production", "pod/payment-api-abc")
    remReq2 := createRemediationRequest("high-cpu", "production", "pod/payment-api-abc")
    remReq3 := createRemediationRequest("high-memory", "staging", "pod/payment-api-xyz")

    remProcessing := &remediationprocessingv1.RemediationProcessing{
        Spec: remediationprocessingv1.RemediationProcessingSpec{
            SignalFingerprint: "high-memory-fingerprint",
            TargetResource: remediationprocessingv1.ResourceIdentifier{
                Namespace: "production",
                Kind:      "Pod",
                Name:      "payment-api-abc",
            },
        },
    }

    relatedSignals, err := findRelatedSignals(ctx, remProcessing)
    require.NoError(t, err)

    // Should find remReq2 (same namespace, same target)
    // Should NOT find remReq1 (self)
    // Should NOT find remReq3 (different namespace)
    assert.Equal(t, 1, len(relatedSignals))
    assert.Equal(t, "high-cpu-fingerprint", relatedSignals[0].Fingerprint)
}
```

#### **Test 2: Correlation Score Calculation**
```go
// File: internal/controller/remediationprocessing/correlation_test.go
func TestCalculateCorrelationScore_JaccardSimilarity(t *testing.T) {
    labels1 := map[string]string{
        "alertname": "HighMemoryUsage",
        "namespace": "production",
        "pod":       "payment-api-abc",
    }

    labels2 := map[string]string{
        "alertname": "HighCPUUsage",
        "namespace": "production",
        "pod":       "payment-api-abc",
    }

    score := calculateCorrelationScore(labels1, labels2)

    // Intersection: {namespace: production, pod: payment-api-abc} = 2
    // Union: {alertname, namespace, pod} = 3
    // Jaccard: 2/3 = 0.666...
    assert.InDelta(t, 0.666, score, 0.01)
}
```

#### **Test 3: Business Context Extraction**
```go
// File: internal/controller/remediationprocessing/business_context_test.go
func TestExtractBusinessContext_FromNamespaceLabels(t *testing.T) {
    ns := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{
            Name: "production",
            Labels: map[string]string{
                "kubernaut.io/owner":       "payments-team",
                "kubernaut.io/criticality": "P0",
                "kubernaut.io/cost-center": "payments-org",
                "kubernaut.io/project":     "checkout-v2",
            },
            Annotations: map[string]string{
                "kubernaut.io/contact":      "payments-team@company.com",
                "kubernaut.io/sla":          "5m",
                "kubernaut.io/impact-level": "critical",
            },
        },
    }

    k8sClient := fake.NewClientBuilder().WithObjects(ns).Build()
    reconciler := &RemediationProcessingReconciler{Client: k8sClient}

    businessCtx, err := reconciler.extractBusinessContext(ctx, "production")
    require.NoError(t, err)

    assert.Equal(t, "payments-team", businessCtx.ServiceOwner)
    assert.Equal(t, "P0", businessCtx.Criticality)
    assert.Equal(t, "5m", businessCtx.SLA)
    assert.Equal(t, "payments-team@company.com", businessCtx.ContactInfo)
    assert.Equal(t, "critical", businessCtx.ImpactLevel)
}
```

#### **Test 4: Business Context ConfigMap Override**
```go
// File: internal/controller/remediationprocessing/business_context_test.go
func TestExtractBusinessContext_ConfigMapOverride(t *testing.T) {
    ns := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{
            Name: "production",
            Labels: map[string]string{
                "kubernaut.io/owner": "old-team", // Will be overridden
            },
        },
    }

    cm := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "kubernaut-business-metadata",
            Namespace: "production",
        },
        Data: map[string]string{
            "owner":      "new-team",  // Override
            "contact":    "new-team@company.com",
            "criticality": "P1",
        },
    }

    k8sClient := fake.NewClientBuilder().WithObjects(ns, cm).Build()
    reconciler := &RemediationProcessingReconciler{Client: k8sClient}

    businessCtx, err := reconciler.extractBusinessContext(ctx, "production")
    require.NoError(t, err)

    // Should use ConfigMap value (override)
    assert.Equal(t, "new-team", businessCtx.ServiceOwner)
    assert.Equal(t, "P1", businessCtx.Criticality)
    assert.Equal(t, "new-team@company.com", businessCtx.ContactInfo)
}
```

### **Integration Tests**

#### **Test 5: End-to-End Enrichment with Monitoring and Business Context**
```go
// File: test/integration/phase2_enrichment_test.go
func TestEnrichment_MonitoringAndBusinessContext(t *testing.T) {
    // Setup: Create namespace with business labels
    ns := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{
            Name: "production",
            Labels: map[string]string{
                "kubernaut.io/owner":       "payments-team",
                "kubernaut.io/criticality": "P0",
            },
            Annotations: map[string]string{
                "kubernaut.io/sla": "5m",
            },
        },
    }
    k8sClient.Create(ctx, ns)

    // Create related signal (for correlation)
    relatedRemReq := createRemediationRequest("high-cpu", "production", "pod/payment-api-abc")
    k8sClient.Create(ctx, relatedRemReq)

    // Create main signal
    remReq := createRemediationRequest("high-memory", "production", "pod/payment-api-abc")
    k8sClient.Create(ctx, remReq)

    // Wait for RemediationProcessing to complete enrichment
    waitForPhase(t, remReq.Name, "completed")

    // Fetch RemediationProcessing
    remProcessing := getRemediationProcessing(t, remReq.Name)

    // Verify MonitoringContext
    assert.NotNil(t, remProcessing.Status.EnrichmentResults.MonitoringContext)
    assert.Equal(t, 1, len(remProcessing.Status.EnrichmentResults.MonitoringContext.RelatedSignals))
    assert.Equal(t, "high-cpu-fingerprint",
                 remProcessing.Status.EnrichmentResults.MonitoringContext.RelatedSignals[0].Fingerprint)

    // Verify BusinessContext
    assert.NotNil(t, remProcessing.Status.EnrichmentResults.BusinessContext)
    assert.Equal(t, "payments-team", remProcessing.Status.EnrichmentResults.BusinessContext.ServiceOwner)
    assert.Equal(t, "P0", remProcessing.Status.EnrichmentResults.BusinessContext.Criticality)
    assert.Equal(t, "5m", remProcessing.Status.EnrichmentResults.BusinessContext.SLA)
}
```

---

## ✅ **Validation Checklist**

### **Pre-Implementation**
- [ ] Phase 1 (P0) is complete and merged
- [ ] Review all 2 tasks with team
- [ ] Understand monitoring correlation logic
- [ ] Understand business context extraction
- [ ] Create feature branch: `feature/phase2-monitoring-business-context`

### **Task 1: MonitoringContext**
- [ ] Go types updated with MonitoringContext + 3 supporting types
- [ ] Signal correlation logic implemented
- [ ] Metrics collector interface defined (V1: no-op)
- [ ] Log collector interface defined (V1: no-op)
- [ ] RemediationProcessor populates MonitoringContext
- [ ] Unit tests pass (correlation, scoring)
- [ ] CRD documentation updated

### **Task 2: BusinessContext**
- [ ] Go types updated with BusinessContext type
- [ ] Business context extraction implemented
- [ ] ConfigMap override logic implemented
- [ ] RemediationProcessor populates BusinessContext
- [ ] Unit tests pass (namespace labels, ConfigMap)
- [ ] CRD documentation updated

### **End-to-End Validation**
- [ ] Related signals correlation works
- [ ] Business context extraction works
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] Documentation updated

---

## 📊 **Progress Tracking**

### **Time Estimates**

| Task | Estimated | Actual | Status |
|------|-----------|--------|--------|
| Task 1: MonitoringContext | 2h | - | ⏸️ Pending |
| Task 2: BusinessContext | 1h | - | ⏸️ Pending |
| **Total** | **3h** | **-** | **⏸️ Pending** |

### **Completion Percentage**

```
[░░░░░░░░░░░░░░░░░░░░] 0% Complete
```

---

## 🚨 **Risk Mitigation**

### **Risk 1: Correlation Performance Impact**

**Mitigation**:
- Limit correlation window to 15 minutes
- Index RemediationRequest by namespace and labels
- Cache correlation results for 5 minutes

### **Risk 2: Missing Namespace Labels**

**Mitigation**:
- Business context is optional (non-critical)
- Default to empty values if labels missing
- Document recommended namespace labeling

### **Risk 3: ConfigMap Conflicts**

**Mitigation**:
- ConfigMap name is fixed (`kubernaut-business-metadata`)
- ConfigMap overrides namespace labels (documented)
- Test both scenarios (with/without ConfigMap)

---

## 📚 **Reference Documents**

1. **Triage Reports**:
   - [`CRD_DATA_FLOW_TRIAGE_REMEDIATION_TO_AI.md`](./CRD_DATA_FLOW_TRIAGE_REMEDIATION_TO_AI.md)

2. **Architecture**:
   - [`CRD_SCHEMAS.md`](../architecture/CRD_SCHEMAS.md)

3. **Phase 1 Guide**:
   - [`PHASE_1_IMPLEMENTATION_GUIDE.md`](./PHASE_1_IMPLEMENTATION_GUIDE.md)

---

## 🎯 **Success Criteria**

**Phase 2 is COMPLETE when**:
1. ✅ RemediationProcessing.status includes MonitoringContext
2. ✅ RemediationProcessing.status includes BusinessContext
3. ✅ Related signals correlation works
4. ✅ Business context extraction from namespace works
5. ✅ All unit tests pass
6. ✅ All integration tests pass
7. ✅ Documentation updated
8. ✅ Code review approved
9. ✅ Merged to main branch

---

## 🔄 **Integration with AIAnalysis**

**How AIAnalysis will use these enhancements**:

### **MonitoringContext Usage**:
```go
// AIAnalysis controller reads MonitoringContext
if remProcessing.Status.EnrichmentResults.MonitoringContext != nil {
    monCtx := remProcessing.Status.EnrichmentResults.MonitoringContext

    // Include related signals in HolmesGPT prompt
    for _, relSignal := range monCtx.RelatedSignals {
        prompt += fmt.Sprintf("Related signal: %s (severity: %s, correlation: %.2f)\n",
                             relSignal.Name, relSignal.Severity, relSignal.CorrelationScore)
    }

    // Include metrics if available
    for _, metric := range monCtx.Metrics {
        prompt += fmt.Sprintf("Metric: %s = %.2f %s\n",
                             metric.Name, metric.Value, metric.Unit)
    }

    // Include logs if available
    for _, log := range monCtx.Logs {
        prompt += fmt.Sprintf("Log [%s]: %s\n", log.Level, log.Message)
    }
}
```

### **BusinessContext Usage**:
```go
// AIAnalysis controller reads BusinessContext
if remProcessing.Status.EnrichmentResults.BusinessContext != nil {
    bizCtx := remProcessing.Status.EnrichmentResults.BusinessContext

    // Use for approval policy decisions
    if bizCtx.Criticality == "P0" {
        // Require manual approval for P0 services
        aiAnalysis.Spec.RequiresApproval = true
        aiAnalysis.Spec.ApprovalReason = fmt.Sprintf(
            "P0 service (owner: %s, SLA: %s)",
            bizCtx.ServiceOwner, bizCtx.SLA)
    }

    // Use for notification routing
    if bizCtx.ContactInfo != "" {
        // Notify service owner about AI recommendations
        notifyServiceOwner(bizCtx.ContactInfo, aiAnalysis.Status.Recommendations)
    }
}
```

---

**Status**: ✅ **APPROVED** - Ready for implementation after Phase 1
**Estimated Duration**: 3 hours
**Priority**: HIGH PRIORITY (Recommended for V1)
**Dependencies**: Phase 1 must be complete
**Next Action**: Wait for Phase 1 completion, then begin Task 1

