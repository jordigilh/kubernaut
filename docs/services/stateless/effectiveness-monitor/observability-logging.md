# Effectiveness Monitor Service - Observability & Logging

**Version**: 1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service (Assessment & Analysis)
**Logging Library**: `go.uber.org/zap`

---

## 📋 Overview

Comprehensive observability strategy for Effectiveness Monitor Service, covering:
- **Structured Logging** (Zap with JSON encoding)
- **Prometheus Metrics** (assessment duration, confidence distribution, side effects)
- **Health Probes** (liveness, readiness with data availability)
- **Distributed Tracing** (correlation ID propagation)
- **Alert Rules** (Prometheus AlertManager)

---

## 📊 Structured Logging

### **Logging Library: go.uber.org/zap**

Per [LOGGING_STANDARD.md](../../../LOGGING_STANDARD.md), HTTP services use `go.uber.org/zap` for:
- High-performance structured logging
- JSON encoding for machine-readable logs
- Strongly typed fields

**Initialization**:
```go
// cmd/effectiveness-monitor/main.go
package main

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

func initLogger() (*zap.Logger, error) {
    config := zap.NewProductionConfig()
    config.Encoding = "json"
    config.EncoderConfig.TimeKey = "timestamp"
    config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

    logger, err := config.Build()
    if err != nil {
        return nil, err
    }

    return logger, nil
}

func main() {
    logger, err := initLogger()
    if err != nil {
        panic(err)
    }
    defer logger.Sync()

    logger.Info("Effectiveness Monitor Service starting",
        zap.String("version", "v1.0"),
        zap.Int("port", 8080),
    )

    // Initialize service with logger
    service := effectiveness.NewEffectivenessMonitorService(logger, deps...)
    // ...
}
```

---

### **Log Levels**

| Level | Purpose | Examples |
|-------|---------|----------|
| **ERROR** | Unrecoverable errors, requires intervention | Data Storage unavailable, assessment persistence failure |
| **WARN** | Recoverable errors, degraded mode | Infrastructure Monitoring unavailable (graceful degradation), insufficient data |
| **INFO** | Normal operations, state transitions | Assessment requests, completions, data availability milestones |
| **DEBUG** | Detailed flow for troubleshooting | Action history queries, side effect detection, pattern insights |

---

### **Correlation ID Propagation**

**Request Context**:
```go
package effectiveness

import (
    "context"
    "net/http"

    "github.com/google/uuid"
    "go.uber.org/zap"
)

type correlationIDKey struct{}

func (s *EffectivenessMonitorService) CorrelationIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract or generate correlation ID
        correlationID := r.Header.Get("X-Correlation-ID")
        if correlationID == "" {
            correlationID = uuid.New().String()
        }

        // Add to response headers
        w.Header().Set("X-Correlation-ID", correlationID)

        // Add to context
        ctx := context.WithValue(r.Context(), correlationIDKey{}, correlationID)

        // Log request
        s.logger.Info("Incoming request",
            zap.String("correlation_id", correlationID),
            zap.String("method", r.Method),
            zap.String("path", r.URL.Path),
            zap.String("client_ip", r.RemoteAddr),
        )

        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func extractCorrelationID(ctx context.Context) string {
    if id, ok := ctx.Value(correlationIDKey{}).(string); ok {
        return id
    }
    return "unknown"
}
```

---

### **Assessment Lifecycle Logging**

#### **1. Assessment Request**

```go
func (s *EffectivenessMonitorService) AssessEffectiveness(ctx context.Context, req *AssessmentRequest) (*EffectivenessScore, error) {
    correlationID := extractCorrelationID(ctx)
    start := time.Now()

    log := s.logger.With(
        zap.String("correlation_id", correlationID),
        zap.String("action_id", req.ActionID),
        zap.String("action_type", req.ActionType),
    )

    log.Info("Assessment request received",
        zap.String("namespace", req.Namespace),
        zap.String("cluster", req.Cluster),
        zap.Bool("wait_for_stabilization", req.WaitForStabilization),
    )

    // ... assessment logic ...

    duration := time.Since(start)
    log.Info("Assessment completed",
        zap.String("assessment_id", assessment.AssessmentID),
        zap.Float64("traditional_score", assessment.TraditionalScore),
        zap.Float64("confidence", assessment.Confidence),
        zap.String("trend_direction", assessment.TrendDirection),
        zap.Duration("duration", duration),
    )

    return assessment, nil
}
```

**Example Log Output** (JSON):
```json
{
  "level": "info",
  "timestamp": "2025-10-06T10:15:30.123Z",
  "caller": "effectiveness/service.go:142",
  "msg": "Assessment request received",
  "correlation_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "action_id": "act-abc123",
  "action_type": "restart-pod",
  "namespace": "prod-payment-service",
  "cluster": "us-west-2",
  "wait_for_stabilization": true
}
```

#### **2. Data Availability Check (Week 5 vs Week 13+)**

```go
func (s *EffectivenessMonitorService) checkDataAvailability(ctx context.Context) (int, bool) {
    log := s.logger.With(
        zap.String("correlation_id", extractCorrelationID(ctx)),
    )

    weeks, err := s.dataStorageClient.GetDataAvailabilityWeeks(ctx)
    if err != nil {
        log.Error("Failed to check data availability",
            zap.Error(err),
        )
        return 0, false
    }

    sufficient := weeks >= 8

    if !sufficient {
        log.Warn("Insufficient historical data for high-confidence assessment",
            zap.Int("data_weeks", weeks),
            zap.Int("required_weeks", 8),
            zap.String("estimated_availability", time.Now().Add(time.Duration(8-weeks)*7*24*time.Hour).Format(time.RFC3339)),
        )
    } else {
        log.Info("Sufficient historical data available",
            zap.Int("data_weeks", weeks),
            zap.String("confidence_level", "high"),
        )
    }

    return weeks, sufficient
}
```

**Example Log Output** (Week 5 - Insufficient Data):
```json
{
  "level": "warn",
  "timestamp": "2025-10-06T10:15:30.456Z",
  "msg": "Insufficient historical data for high-confidence assessment",
  "correlation_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "data_weeks": 0,
  "required_weeks": 8,
  "estimated_availability": "2025-11-19T00:00:00Z"
}
```

#### **3. Action History Retrieval**

```go
func (c *DataStorageClient) GetActionHistory(ctx context.Context, actionType string, window time.Duration) ([]ActionHistory, error) {
    log := c.logger.With(
        zap.String("correlation_id", extractCorrelationID(ctx)),
        zap.String("action_type", actionType),
        zap.Duration("window", window),
    )

    start := time.Now()
    log.Debug("Querying action history from Data Storage")

    history, err := c.queryHistory(ctx, actionType, window)
    if err != nil {
        log.Error("Failed to retrieve action history",
            zap.Error(err),
            zap.Duration("query_duration", time.Since(start)),
        )
        return nil, err
    }

    log.Info("Action history retrieved successfully",
        zap.Int("count", len(history)),
        zap.Duration("query_duration", time.Since(start)),
    )

    return history, nil
}
```

#### **4. Environmental Metrics Correlation (Graceful Degradation)**

```go
func (c *InfrastructureMonitoringClient) GetMetricsAfterAction(ctx context.Context, actionID string, window time.Duration) (*EnvironmentalMetrics, error) {
    log := c.logger.With(
        zap.String("correlation_id", extractCorrelationID(ctx)),
        zap.String("action_id", actionID),
        zap.Duration("window", window),
    )

    start := time.Now()
    log.Debug("Querying environmental metrics from Infrastructure Monitoring")

    metrics, err := c.queryMetrics(ctx, actionID, window)
    if err != nil {
        log.Warn("Failed to retrieve environmental metrics, continuing with basic assessment",
            zap.Error(err),
            zap.Duration("query_duration", time.Since(start)),
            zap.String("degradation_mode", "no_environmental_impact"),
        )
        // Graceful degradation: return zero-impact metrics
        return &EnvironmentalMetrics{}, nil
    }

    log.Debug("Environmental metrics retrieved successfully",
        zap.Float64("memory_improvement", metrics.MemoryImprovement),
        zap.Float64("cpu_impact", metrics.CPUImpact),
        zap.Float64("network_stability", metrics.NetworkStability),
        zap.Duration("query_duration", time.Since(start)),
    )

    return metrics, nil
}
```

**Example Log Output** (Graceful Degradation):
```json
{
  "level": "warn",
  "timestamp": "2025-10-06T10:15:31.789Z",
  "msg": "Failed to retrieve environmental metrics, continuing with basic assessment",
  "correlation_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "action_id": "act-abc123",
  "window": "10m0s",
  "error": "connection refused",
  "query_duration": "2.123s",
  "degradation_mode": "no_environmental_impact"
}
```

#### **5. Side Effect Detection**

```go
func (c *Calculator) DetectSideEffects(metrics *EnvironmentalMetrics) (bool, string) {
    log := c.logger.With(
        zap.Float64("cpu_impact", metrics.CPUImpact),
        zap.Float64("network_stability", metrics.NetworkStability),
    )

    if metrics.CPUImpact < -0.3 {
        log.Warn("High severity side effect detected",
            zap.String("severity", "high"),
            zap.Float64("cpu_increase_percent", math.Abs(metrics.CPUImpact)*100),
        )
        return true, "high"
    } else if metrics.CPUImpact < -0.1 || metrics.NetworkStability < 0.7 {
        log.Info("Low severity side effect detected",
            zap.String("severity", "low"),
            zap.Float64("cpu_increase_percent", math.Abs(metrics.CPUImpact)*100),
            zap.Float64("network_stability_percent", metrics.NetworkStability*100),
        )
        return true, "low"
    }

    log.Debug("No side effects detected")
    return false, "none"
}
```

#### **6. Assessment Persistence (Best-Effort)**

```go
func (c *DataStorageClient) PersistAssessment(ctx context.Context, assessment *EffectivenessScore) error {
    log := c.logger.With(
        zap.String("correlation_id", extractCorrelationID(ctx)),
        zap.String("assessment_id", assessment.AssessmentID),
        zap.String("action_id", assessment.ActionID),
    )

    start := time.Now()
    log.Debug("Persisting assessment to Data Storage")

    if err := c.writeAssessment(ctx, assessment); err != nil {
        log.Error("Failed to persist assessment (best-effort, continuing)",
            zap.Error(err),
            zap.Duration("write_duration", time.Since(start)),
        )
        return err
    }

    log.Info("Assessment persisted successfully",
        zap.Duration("write_duration", time.Since(start)),
    )

    return nil
}
```

---

## 🤖 AI Analysis Logging

### **AI Trigger Decision Logging**

```go
// Log AI decision logic
logger.Info("AI decision evaluation",
    zap.String("workflow_id", workflow.ID),
    zap.String("priority", workflow.Priority),
    zap.Bool("success", workflow.Success),
    zap.Bool("is_new_action_type", workflow.IsNewActionType),
    zap.Int("anomaly_count", len(anomalies)),
    zap.Bool("is_recurring_failure", workflow.IsRecurringFailure),
    zap.Bool("ai_analysis_triggered", aiDecision),
    zap.String("trigger_reason", triggerReason), // "p0_failure", "new_action_type", "anomaly_detected", "oscillation", "routine_skipped"
)
```

**Example Log Entries**:

```json
// P0 failure - AI triggered
{
  "level": "info",
  "timestamp": "2025-10-06T10:15:30Z",
  "message": "AI decision evaluation",
  "workflow_id": "wf-abc123",
  "priority": "P0",
  "success": false,
  "is_new_action_type": false,
  "anomaly_count": 0,
  "is_recurring_failure": false,
  "ai_analysis_triggered": true,
  "trigger_reason": "p0_failure"
}

// Routine success - AI skipped
{
  "level": "info",
  "timestamp": "2025-10-06T10:15:35Z",
  "message": "AI decision evaluation",
  "workflow_id": "wf-def456",
  "priority": "P2",
  "success": true,
  "is_new_action_type": false,
  "anomaly_count": 0,
  "is_recurring_failure": false,
  "ai_analysis_triggered": false,
  "trigger_reason": "routine_skipped"
}

// Anomaly detected - AI triggered
{
  "level": "info",
  "timestamp": "2025-10-06T10:15:40Z",
  "message": "AI decision evaluation",
  "workflow_id": "wf-ghi789",
  "priority": "P1",
  "success": true,
  "is_new_action_type": false,
  "anomaly_count": 3,
  "is_recurring_failure": false,
  "ai_analysis_triggered": true,
  "trigger_reason": "anomaly_detected"
}
```

### **AI Call Execution Logging**

```go
// Log AI API call start
logger.Info("Calling HolmesGPT API for post-execution analysis",
    zap.String("workflow_id", workflow.ID),
    zap.String("endpoint", "/api/v1/postexec/analyze"),
    zap.String("execution_id", execID),
)

// Log AI API call success
logger.Info("HolmesGPT API call successful",
    zap.String("workflow_id", workflow.ID),
    zap.Duration("duration", duration),
    zap.Float64("effectiveness_score", response.EffectivenessScore),
    zap.Int("lessons_learned_count", len(response.LessonsLearned)),
    zap.Float64("confidence", response.Confidence),
    zap.Float64("estimated_cost", 0.50), // $0.50 per call
)

// Log AI API call failure
logger.Error("HolmesGPT API call failed",
    zap.String("workflow_id", workflow.ID),
    zap.Duration("duration", duration),
    zap.Error(err),
    zap.String("fallback", "using automated assessment only"),
)
```

**Example Log Entries**:

```json
// AI call success
{
  "level": "info",
  "timestamp": "2025-10-06T10:15:45Z",
  "message": "HolmesGPT API call successful",
  "workflow_id": "wf-abc123",
  "duration": "2.3s",
  "effectiveness_score": 0.85,
  "lessons_learned_count": 3,
  "confidence": 0.90,
  "estimated_cost": 0.50
}

// AI call failure (fallback to automated)
{
  "level": "error",
  "timestamp": "2025-10-06T10:16:00Z",
  "message": "HolmesGPT API call failed",
  "workflow_id": "wf-jkl012",
  "duration": "30s",
  "error": "context deadline exceeded",
  "fallback": "using automated assessment only"
}
```

### **Cost Tracking Logs**

```go
// Log daily cost summary (cron job)
logger.Info("Daily AI cost summary",
    zap.Int("total_ai_calls", dailyCount),
    zap.Int("p0_failures", p0Count),
    zap.Int("new_action_types", newActionCount),
    zap.Int("anomalies", anomalyCount),
    zap.Int("oscillations", oscillationCount),
    zap.Float64("daily_cost_usd", dailyCount*0.50),
    zap.Float64("projected_monthly_cost_usd", dailyCount*0.50*30),
)
```

**Example Log Entry**:

```json
{
  "level": "info",
  "timestamp": "2025-10-06T23:59:59Z",
  "message": "Daily AI cost summary",
  "total_ai_calls": 70,
  "p0_failures": 50,
  "new_action_types": 10,
  "anomalies": 5,
  "oscillations": 5,
  "daily_cost_usd": 35.00,
  "projected_monthly_cost_usd": 1050.00
}
```

---

## 📈 Prometheus Metrics

### **Metric Definitions**

```go
package effectiveness

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Assessment duration histogram
    assessmentDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "effectiveness_assessment_duration_seconds",
            Help:    "Duration of effectiveness assessments",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to 51.2s
        },
        []string{"action_type", "confidence_level", "status"},
    )

    // Traditional effectiveness score distribution
    effectivenessScore = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "effectiveness_traditional_score",
            Help:    "Traditional effectiveness score distribution",
            Buckets: prometheus.LinearBuckets(0, 0.1, 11), // 0.0 to 1.0
        },
        []string{"action_type", "environment"},
    )

    // Data availability gauge
    dataAvailabilityWeeks = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "effectiveness_data_availability_weeks",
            Help: "Number of weeks of historical data available",
        },
    )

    // Insufficient data responses counter
    insufficientDataResponses = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "effectiveness_insufficient_data_responses_total",
            Help: "Total number of insufficient_data responses",
        },
    )

    // Side effects detected counter
    sideEffectsDetected = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "effectiveness_side_effects_detected_total",
            Help: "Total number of assessments with side effects detected",
        },
        []string{"severity"}, // "high", "low", "none"
    )

    // Assessments total counter
    assessmentsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "effectiveness_assessments_total",
            Help: "Total number of effectiveness assessments by status",
        },
        []string{"status"}, // "assessed", "insufficient_data", "error"
    )

    // Data Storage query duration
    dataStorageQueryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "effectiveness_data_storage_query_duration_seconds",
            Help:    "Duration of Data Storage queries",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 8), // 10ms to 1.28s
        },
        []string{"query_type"}, // "action_history", "oldest_action", "persist_assessment"
    )

    // Infrastructure Monitoring query duration
    infraMonitorQueryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "effectiveness_infrastructure_monitoring_query_duration_seconds",
            Help:    "Duration of Infrastructure Monitoring queries",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 8), // 10ms to 1.28s
        },
        []string{"status"}, // "success", "error", "timeout"
    )

    // Circuit breaker state
    circuitBreakerState = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "effectiveness_circuit_breaker_state",
            Help: "Circuit breaker state for Infrastructure Monitoring (0=closed, 1=open)",
        },
        []string{"dependency"}, // "infrastructure_monitoring"
    )
)
```

### **Metric Instrumentation**

```go
func (s *EffectivenessMonitorService) AssessEffectiveness(ctx context.Context, req *AssessmentRequest) (*EffectivenessScore, error) {
    start := time.Now()

    // ... assessment logic ...

    // Determine confidence level bucket
    confidenceLevel := "low"
    if assessment.Confidence >= 0.8 {
        confidenceLevel = "high"
    } else if assessment.Confidence >= 0.5 {
        confidenceLevel = "medium"
    }

    // Record assessment duration
    assessmentDuration.WithLabelValues(
        req.ActionType,
        confidenceLevel,
        assessment.Status,
    ).Observe(time.Since(start).Seconds())

    // Record traditional score
    effectivenessScore.WithLabelValues(
        req.ActionType,
        "production", // Extract from req.Namespace if available
    ).Observe(assessment.TraditionalScore)

    // Increment assessments counter
    assessmentsTotal.WithLabelValues(assessment.Status).Inc()

    // Record side effects
    sideEffectsDetected.WithLabelValues(assessment.SideEffectSeverity).Inc()

    return assessment, nil
}
```

### **Metrics Endpoint**

```go
func (s *EffectivenessMonitorService) metricsHandler() http.Handler {
    // Apply TokenReviewer authentication
    return s.AuthMiddleware()(promhttp.Handler())
}

func (s *EffectivenessMonitorService) RegisterRoutes(mux *http.ServeMux) {
    // Metrics on port 9090
    metricsServer := &http.Server{
        Addr:    ":9090",
        Handler: s.metricsHandler(),
    }
    go metricsServer.ListenAndServe()
}
```

---

## 🔍 Health Probes

### **Liveness Probe** (`/health`)

```go
func (s *EffectivenessMonitorService) healthHandler(w http.ResponseWriter, r *http.Request) {
    // Simple liveness check: service is running
    response := map[string]interface{}{
        "status":    "healthy",
        "timestamp": time.Now().Format(time.RFC3339),
    }

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}
```

**Kubernetes Configuration**:
```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3
```

---

### **Readiness Probe** (`/ready`)

```go
func (s *EffectivenessMonitorService) readinessHandler(w http.ResponseWriter, r *http.Request) {
    // Check critical dependencies
    if !s.dataStorageClient.Healthy() {
        s.logger.Error("Readiness check failed: Data Storage unavailable")
        http.Error(w, "Data Storage unavailable", http.StatusServiceUnavailable)
        return
    }

    // Check data availability (informational only)
    dataWeeks, sufficient := s.checkDataAvailability(r.Context())

    currentCapability := "insufficient_data_responses"
    if sufficient {
        currentCapability = "full_assessment"
    }

    response := map[string]interface{}{
        "status":             "ready",
        "data_weeks":         dataWeeks,
        "full_capability":    sufficient,
        "current_capability": currentCapability,
        "timestamp":          time.Now().Format(time.RFC3339),
    }

    // Update data availability metric
    dataAvailabilityWeeks.Set(float64(dataWeeks))

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}
```

**Kubernetes Configuration**:
```yaml
readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 5
  failureThreshold: 3
```

---

## 🚨 Prometheus Alert Rules

### **Critical Alerts**

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: effectiveness-monitor-alerts
  namespace: prometheus-alerts-slm
spec:
  groups:
  - name: effectiveness-monitor.critical
    interval: 30s
    rules:
    # Data Storage Unavailable
    - alert: EffectivenessMonitorDataStorageUnavailable
      expr: up{job="effectiveness-monitor-service"} == 0
      for: 2m
      labels:
        severity: critical
        service: effectiveness-monitor
      annotations:
        summary: "Effectiveness Monitor Service cannot reach Data Storage"
        description: "Data Storage is unavailable for {{ $value }} minutes"

    # High Error Rate
    - alert: EffectivenessMonitorHighErrorRate
      expr: |
        sum(rate(effectiveness_assessments_total{status="error"}[5m])) /
        sum(rate(effectiveness_assessments_total[5m])) > 0.1
      for: 5m
      labels:
        severity: critical
        service: effectiveness-monitor
      annotations:
        summary: "Effectiveness Monitor Service has high error rate"
        description: "Error rate is {{ $value | humanizePercentage }}"

    # Insufficient Data for Extended Period (Week 15+)
    - alert: EffectivenessMonitorInsufficientDataExtended
      expr: |
        effectiveness_data_availability_weeks < 8 and
        time() - process_start_time_seconds{job="effectiveness-monitor-service"} > 10080 * 60
      for: 1h
      labels:
        severity: critical
        service: effectiveness-monitor
      annotations:
        summary: "Effectiveness Monitor still insufficient data after 10+ weeks deployment"
        description: "Data weeks: {{ $value }}, expected ≥8 by now"
```

### **Warning Alerts**

```yaml
  - name: effectiveness-monitor.warnings
    interval: 30s
    rules:
    # High Assessment Latency
    - alert: EffectivenessMonitorHighLatency
      expr: |
        histogram_quantile(0.95,
          rate(effectiveness_assessment_duration_seconds_bucket[5m])
        ) > 5
      for: 10m
      labels:
        severity: warning
        service: effectiveness-monitor
      annotations:
        summary: "Effectiveness Monitor Service has high assessment latency"
        description: "P95 latency is {{ $value }}s (target: <5s)"

    # High Side Effect Detection Rate
    - alert: EffectivenessMonitorHighSideEffects
      expr: |
        sum(rate(effectiveness_side_effects_detected_total{severity="high"}[1h])) /
        sum(rate(effectiveness_assessments_total{status="assessed"}[1h])) > 0.15
      for: 30m
      labels:
        severity: warning
        service: effectiveness-monitor
      annotations:
        summary: "High rate of side effects detected in assessments"
        description: "High severity side effects: {{ $value | humanizePercentage }}"

    # Circuit Breaker Open for Extended Period
    - alert: EffectivenessMonitorCircuitBreakerOpen
      expr: effectiveness_circuit_breaker_state{dependency="infrastructure_monitoring"} == 1
      for: 15m
      labels:
        severity: warning
        service: effectiveness-monitor
      annotations:
        summary: "Circuit breaker open for Infrastructure Monitoring"
        description: "Degraded mode active for {{ $value }} minutes"
```

---

## 📊 Grafana Dashboard

### **Key Panels**

1. **Assessment Volume**: `rate(effectiveness_assessments_total[5m])`
2. **Assessment Latency**: P50, P95, P99 of `effectiveness_assessment_duration_seconds`
3. **Data Availability**: `effectiveness_data_availability_weeks` (gauge)
4. **Traditional Score Distribution**: Heatmap of `effectiveness_traditional_score`
5. **Side Effects Rate**: `rate(effectiveness_side_effects_detected_total[5m])` by severity
6. **Insufficient Data Rate**: `rate(effectiveness_insufficient_data_responses_total[5m])`
7. **Confidence Distribution**: Histogram of confidence levels
8. **Data Storage Query Duration**: P95 of `effectiveness_data_storage_query_duration_seconds`
9. **Circuit Breaker State**: `effectiveness_circuit_breaker_state`

---

## ✅ Observability Checklist

### **Pre-Deployment**

- [ ] Zap logger initialized with JSON encoding
- [ ] Correlation ID middleware configured
- [ ] All Prometheus metrics registered
- [ ] Health and readiness probes tested
- [ ] Metrics endpoint secured with TokenReviewer
- [ ] Alert rules deployed to Prometheus

### **Runtime Monitoring**

- [ ] Assessment request/response logs visible
- [ ] Data availability tracked in metrics
- [ ] Side effect detection logged and alerted
- [ ] Graceful degradation events logged
- [ ] Circuit breaker state monitored
- [ ] Assessment persistence failures alerted

### **Week 5 Validation**

- [ ] `insufficient_data` responses logged correctly
- [ ] Data availability metric shows 0 weeks
- [ ] Estimated availability date calculated
- [ ] No false positive alerts

### **Week 13+ Validation**

- [ ] Full assessment logs include all fields
- [ ] Data availability metric shows ≥8 weeks
- [ ] Confidence ≥80% for assessments
- [ ] No `insufficient_data` responses

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ Complete Specification

