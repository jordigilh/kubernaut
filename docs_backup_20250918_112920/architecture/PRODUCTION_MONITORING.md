# Production Monitoring and Observability Architecture

## Overview

This document describes the comprehensive monitoring, observability, and metrics collection architecture for the Kubernaut system in production environments, ensuring operational excellence and proactive issue detection.

## Business Requirements Addressed

- **BR-HEALTH-020 to BR-HEALTH-034**: Comprehensive health monitoring system
- **BR-PERF-001 to BR-PERF-025**: Performance monitoring and optimization
- **BR-METRICS-001 to BR-METRICS-015**: Business and technical metrics collection
- **BR-OBSERVABILITY-001 to BR-OBSERVABILITY-010**: System observability and tracing
- **BR-SLA-001**: 99%+ uptime monitoring and alerting

## Monitoring Architecture Overview

### Multi-Layer Monitoring Strategy

```ascii
┌─────────────────────────────────────────────────────────────────┐
│                    MONITORING ARCHITECTURE                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Layer 1: Infrastructure Monitoring                             │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ Kubernetes      │  │ Container       │  │ Network         │ │
│ │ Cluster         │  │ Runtime         │  │ Performance     │ │
│ │ Metrics         │  │ Metrics         │  │ Monitoring      │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│          │                    │                    │           │
│          ▼                    ▼                    ▼           │
│ Layer 2: Application Monitoring                                │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ Service Health  │  │ API Performance │  │ Resource        │ │
│ │ & Availability  │  │ & Throughput    │  │ Utilization     │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│          │                    │                    │           │
│          ▼                    ▼                    ▼           │
│ Layer 3: Business Monitoring                                   │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ Alert           │  │ Investigation   │  │ Action          │ │
│ │ Processing      │  │ Accuracy        │  │ Success Rate    │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│          │                    │                    │           │
│          ▼                    ▼                    ▼           │
│ Layer 4: AI/ML Monitoring                                      │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ Model           │  │ Inference       │  │ Context         │ │
│ │ Performance     │  │ Quality         │  │ Effectiveness   │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Health Monitoring Integration

### Comprehensive Health Endpoints

**Health Check Hierarchy:**
```ascii
                        ┌─────────────────┐
                        │ /health         │
                        │ (Overall)       │
                        └─────────────────┘
                                  │
                    ┌─────────────┼─────────────┐
                    ▼             ▼             ▼
        ┌─────────────────┐ ┌──────────────┐ ┌──────────────┐
        │ /health/llm     │ │ /health/     │ │ /health/     │
        │ (AI Services)   │ │ dependencies │ │ components   │
        └─────────────────┘ └──────────────┘ └──────────────┘
                    │             │             │
                    ▼             ▼             ▼
        ┌─────────────────┐ ┌──────────────┐ ┌──────────────┐
        │ /health/llm/    │ │ External     │ │ Internal     │
        │ liveness        │ │ Services     │ │ Components   │
        └─────────────────┘ └──────────────┘ └──────────────┘
                    │
                    ▼
        ┌─────────────────┐
        │ /health/llm/    │
        │ readiness       │
        └─────────────────┘
```

**Implementation Examples:**
```go
// Primary health check endpoint
func (cc *ContextController) HealthCheck(w http.ResponseWriter, r *http.Request) {
    health := map[string]interface{}{
        "status":         "healthy",
        "service":        "context-api",
        "timestamp":      time.Now().UTC(),
        "version":        "1.0.0",
        "cache_hit_rate": cc.contextCache.GetHitRate(),
        "context_types":  len(cc.discovery.contextTypes),
    }

    cc.writeJSONResponse(w, http.StatusOK, health)
}

// LLM health monitoring
func (cc *ContextController) LLMHealthCheck(w http.ResponseWriter, r *http.Request) {
    healthStatus, err := cc.healthMonitor.GetHealthStatus(ctx)
    if err != nil {
        cc.writeErrorResponse(w, http.StatusInternalServerError, "Health check failed", err.Error())
        return
    }

    statusCode := http.StatusOK
    if !healthStatus.IsHealthy {
        statusCode = http.StatusServiceUnavailable
    }

    response := map[string]interface{}{
        "is_healthy":       healthStatus.IsHealthy,
        "component_type":   healthStatus.ComponentType,
        "response_time":    healthStatus.ResponseTime.String(),
        "health_metrics":   healthStatus.HealthMetrics,
    }

    cc.writeJSONResponse(w, statusCode, response)
}
```

### Kubernetes Integration

**Probe Configuration:**
```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: kubernaut-context-api
    livenessProbe:
      httpGet:
        path: /health/llm/liveness
        port: 8091
      initialDelaySeconds: 30
      periodSeconds: 10
      timeoutSeconds: 5
      failureThreshold: 3

    readinessProbe:
      httpGet:
        path: /health/llm/readiness
        port: 8091
      initialDelaySeconds: 5
      periodSeconds: 5
      timeoutSeconds: 3
      failureThreshold: 3
```

## Metrics Collection Architecture

### Prometheus Metrics Integration

**Metrics Categories:**
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                     METRICS TAXONOMY                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Business Metrics                                                │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ Alert           │  │ Investigation   │  │ Action          │ │
│ │ Processing      │  │ Accuracy        │  │ Success Rate    │ │
│ │ Rate            │  │ Score           │  │                 │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│                                                                 │
│ Technical Metrics                                               │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ API Response    │  │ Cache Hit       │  │ Error Rate      │ │
│ │ Time            │  │ Rate            │  │ & Types         │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│                                                                 │
│ AI/ML Metrics                                                   │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ Model           │  │ Inference       │  │ Context         │ │
│ │ Latency         │  │ Quality         │  │ Optimization    │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│                                                                 │
│ Infrastructure Metrics                                          │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ Resource        │  │ Network         │  │ Storage         │ │
│ │ Utilization     │  │ Performance     │  │ Performance     │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### Key Performance Indicators (KPIs)

**Business KPIs:**
```prometheus
# Alert processing performance
kubernaut_alerts_processed_total{status="success"} 1250
kubernaut_alerts_processed_total{status="filtered"} 85
kubernaut_alerts_filtered_ratio 0.064

# Investigation accuracy and confidence
kubernaut_investigation_confidence_score{service="holmesgpt"} 0.87
kubernaut_investigation_confidence_score{service="llm"} 0.82
kubernaut_investigation_accuracy_rate{service="holmesgpt"} 0.91

# Action execution success
kubernaut_actions_executed_total{action="scale_deployment",status="success"} 145
kubernaut_actions_executed_total{action="restart_pod",status="success"} 89
kubernaut_action_success_rate{action="scale_deployment"} 0.94
```

**Technical KPIs:**
```prometheus
# API Performance
kubernaut_api_request_duration_seconds{endpoint="/api/v1/context/kubernetes"} 0.089
kubernaut_api_requests_total{endpoint="/api/v1/context/kubernetes",status="200"} 2341
kubernaut_api_error_rate{endpoint="/api/v1/context/kubernetes"} 0.002

# Cache Performance
kubernaut_cache_hit_ratio{cache_type="context"} 0.84
kubernaut_cache_operations_total{operation="hit",cache_type="context"} 1967
kubernaut_cache_operations_total{operation="miss",cache_type="context"} 374

# Service Health
kubernaut_service_availability_ratio{service="holmesgpt"} 0.997
kubernaut_service_availability_ratio{service="llm"} 0.995
kubernaut_service_availability_ratio{service="context_api"} 0.999
```

**AI/ML KPIs:**
```prometheus
# Model Performance
kubernaut_llm_inference_duration_seconds{model="granite3.1-dense:8b"} 2.34
kubernaut_llm_token_usage_total{model="granite3.1-dense:8b",type="input"} 2456789
kubernaut_llm_token_usage_total{model="granite3.1-dense:8b",type="output"} 892345

# Context Optimization
kubernaut_context_size_bytes{complexity="simple"} 1024
kubernaut_context_size_bytes{complexity="complex"} 8192
kubernaut_context_reduction_ratio{complexity="simple"} 0.75
```

### Custom Metrics Implementation

**Metrics Collection in Code:**
```go
// Business metrics
var (
    alertsProcessedTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_alerts_processed_total",
            Help: "Total number of alerts processed",
        },
        []string{"status", "severity", "namespace"},
    )

    investigationConfidenceScore = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "kubernaut_investigation_confidence_score",
            Help: "Investigation confidence score from AI services",
        },
        []string{"service", "alert_type"},
    )

    actionExecutionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "kubernaut_action_execution_duration_seconds",
            Help: "Time taken to execute remediation actions",
            Buckets: prometheus.DefBuckets,
        },
        []string{"action", "namespace", "status"},
    )
)

// Usage in application code
func (p *Processor) ProcessAlert(alert types.Alert) error {
    start := time.Now()
    defer func() {
        alertsProcessedTotal.WithLabelValues("success", alert.Severity, alert.Namespace).Inc()
    }()

    // Process alert logic...
    return nil
}
```

## Distributed Tracing

### Request Tracing Architecture

**Trace Flow:**
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                      DISTRIBUTED TRACING                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Incoming Alert                                                  │
│      │ (Trace ID: abc123)                                       │
│      ▼                                                          │
│ ┌─────────────────┐     ┌─────────────────┐     ┌─────────────┐ │
│ │ Webhook         │────▶│ Alert           │────▶│ AI Service  │ │
│ │ Handler         │     │ Processor       │     │ Integrator  │ │
│ │ (Span: webhook) │     │ (Span: process) │     │ (Span: ai)  │ │
│ └─────────────────┘     └─────────────────┘     └─────────────┘ │
│      │                           │                     │        │
│      ▼                           ▼                     ▼        │
│ ┌─────────────────┐     ┌─────────────────┐     ┌─────────────┐ │
│ │ Context         │     │ HolmesGPT       │     │ Action      │ │
│ │ Enrichment      │     │ Investigation   │     │ Executor    │ │
│ │ (Span: context) │     │ (Span: holmes)  │     │ (Span: exec)│ │
│ └─────────────────┘     └─────────────────┘     └─────────────┘ │
│                                                                 │
│ All spans share Trace ID: abc123                               │
│ Each span has unique Span ID and parent relationships         │
└─────────────────────────────────────────────────────────────────┘
```

**Tracing Implementation:**
```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/trace"
)

func (h *WebhookHandler) HandleAlert(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    tracer := otel.Tracer("kubernaut-webhook")

    ctx, span := tracer.Start(ctx, "webhook.handle_alert")
    defer span.End()

    span.SetAttributes(
        attribute.String("alert.source", "prometheus"),
        attribute.String("request.method", r.Method),
    )

    // Process alert with traced context
    if err := h.processor.ProcessAlert(ctx, alert); err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return
    }

    span.SetStatus(codes.Ok, "Alert processed successfully")
}
```

### Correlation and Context Propagation

**Trace Context Headers:**
- `traceparent`: W3C trace context header
- `tracestate`: Vendor-specific trace state
- `x-correlation-id`: Custom correlation identifier
- `x-request-id`: Request-specific identifier

## Log Aggregation and Analysis

### Structured Logging Strategy

**Log Format Standardization:**
```json
{
  "timestamp": "2024-01-15T10:30:45.123Z",
  "level": "info",
  "service": "context-api",
  "trace_id": "abc123def456",
  "span_id": "def456ghi789",
  "correlation_id": "req-789012",
  "component": "ai_integrator",
  "operation": "investigate_alert",
  "alert_id": "alert-456789",
  "namespace": "production",
  "duration_ms": 2340,
  "confidence_score": 0.87,
  "action_recommended": "scale_deployment",
  "metadata": {
    "model": "granite3.1-dense:8b",
    "context_size": 4096,
    "fallback_used": false
  },
  "message": "Investigation completed successfully"
}
```

**Log Levels and Usage:**
- **ERROR**: System errors, failures, exceptions
- **WARN**: Degraded performance, fallback usage, retries
- **INFO**: Normal operations, successful completions
- **DEBUG**: Detailed execution flow, performance data

### Centralized Logging Architecture

**Log Collection Flow:**
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                      LOG AGGREGATION                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Application Logs                                                │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ Context API     │  │ HolmesGPT API   │  │ Webhook         │ │
│ │ Logs            │  │ Logs            │  │ Handler Logs    │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│          │                    │                    │           │
│          ▼                    ▼                    ▼           │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │                  Fluent Bit / Vector                       │ │
│ │              (Log Collection Agent)                        │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │                   Elasticsearch                            │ │
│ │                 (Log Storage & Search)                     │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │                     Kibana                                 │ │
│ │               (Log Analysis & Visualization)               │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Alerting and Notification

### Alert Classification and Routing

**Alert Severity Levels:**
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                    ALERT SEVERITY MATRIX                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ CRITICAL (P0)                                                   │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • System down (>5 min)        • Data loss detected         │ │
│ │ • Security breach             • All AI services failed     │ │
│ │ • Action: Immediate response  • Escalation: Automatic      │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ HIGH (P1)                                                       │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • Primary service degraded    • High error rate (>5%)      │ │
│ │ • Investigation failures      • Context API unavailable    │ │
│ │ • Action: 15 min response     • Escalation: If unresolved  │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ MEDIUM (P2)                                                     │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • Performance degradation     • Fallback usage increase    │ │
│ │ • Cache hit rate drop         • Circuit breaker events     │ │
│ │ • Action: 1 hour response     • Escalation: Business hours │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ LOW (P3)                                                        │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • Minor performance issues    • Informational events       │ │
│ │ • Capacity planning alerts   • Maintenance notifications   │ │
│ │ • Action: Next business day   • Escalation: None required  │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### AlertManager Configuration

**Routing Rules:**
```yaml
route:
  group_by: ['alertname', 'cluster', 'service']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  receiver: 'default'
  routes:
  - match:
      severity: critical
      service: kubernaut
    receiver: 'pager-critical'
    group_wait: 0s
    repeat_interval: 5m

  - match:
      severity: high
      service: kubernaut
    receiver: 'slack-high'
    group_wait: 30s
    repeat_interval: 1h

receivers:
- name: 'pager-critical'
  pagerduty_configs:
  - severity: critical
    description: 'Kubernaut Critical Alert: {{ .GroupLabels.alertname }}'

- name: 'slack-high'
  slack_configs:
  - api_url: '{{ .SlackWebhookURL }}'
    channel: '#kubernaut-alerts'
    title: 'Kubernaut Alert: {{ .GroupLabels.alertname }}'
```

## Dashboard and Visualization

### Grafana Dashboard Architecture

**Dashboard Hierarchy:**
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                    DASHBOARD STRUCTURE                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Level 1: Executive Dashboard                                    │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • System Health Overview      • SLA Compliance Status      │ │
│ │ • Business KPI Summary        • Alert Volume Trends        │ │
│ │ • Cost and Efficiency Metrics • Capacity Planning          │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Level 2: Operational Dashboard                                  │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • Service Health Status       • Performance Metrics        │ │
│ │ • Error Rates and Types       • Resource Utilization       │ │
│ │ • Investigation Accuracy      • Action Success Rates       │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Level 3: Technical Dashboard                                    │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • API Response Times          • Cache Performance          │ │
│ │ • Database Metrics            • Network Performance        │ │
│ │ • AI Model Performance        • Context Optimization       │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Level 4: Debug Dashboard                                        │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • Detailed Trace Analysis     • Log Correlation View       │ │
│ │ • Circuit Breaker States      • Fallback Usage Patterns    │ │
│ │ • Resource Allocation Details • Error Investigation        │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### Key Dashboard Panels

**Executive Dashboard KPIs:**
- System availability (99%+ target)
- Alert processing rate (alerts/hour)
- Investigation accuracy (confidence scores)
- Action success rate (percentage)
- Mean time to resolution (MTTR)
- Cost per investigation (resource usage)

**Operational Dashboard Metrics:**
- Service health matrix (green/yellow/red status)
- API endpoint performance (response times, throughput)
- Error rate trends (5-minute intervals)
- Resource utilization (CPU, memory, storage)
- Cache performance (hit rates, eviction rates)
- AI service performance (inference times, accuracy)

## Performance Monitoring Integration

### Real-time Performance Tracking

**Performance Metrics Collection Points:**
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                  PERFORMANCE MONITORING POINTS                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Alert Ingestion (Point 1)                                      │
│ ┌─────────────────┐     Metrics: Throughput, Latency          │
│ │ Webhook         │     Targets: 1000+ alerts/min, <50ms      │
│ │ Reception       │                                            │ │
│ └─────────────────┘                                            │ │
│          │                                                      │
│          ▼                                                      │ │
│ Context Enrichment (Point 2)                                    │
│ ┌─────────────────┐     Metrics: Cache hit rate, Fetch time   │ │
│ │ Context API     │     Targets: >80% hit rate, <100ms        │ │
│ │ Operations      │                                            │ │
│ └─────────────────┘                                            │ │
│          │                                                      │
│          ▼                                                      │ │
│ AI Investigation (Point 3)                                      │
│ ┌─────────────────┐     Metrics: Inference time, Quality      │ │
│ │ HolmesGPT/LLM   │     Targets: <5s, >85% confidence        │ │
│ │ Analysis        │                                            │ │
│ └─────────────────┘                                            │ │
│          │                                                      │
│          ▼                                                      │ │
│ Action Execution (Point 4)                                      │
│ ┌─────────────────┐     Metrics: Execution time, Success rate │ │
│ │ Kubernetes      │     Targets: <30s, >90% success          │ │
│ │ Operations      │                                            │ │
│ └─────────────────┘                                            │ │
└─────────────────────────────────────────────────────────────────┘
```

### SLA Monitoring and Compliance

**Service Level Agreements:**
```yaml
slas:
  system_availability:
    target: 99.9%
    measurement_window: 30d
    exclusions:
      - planned_maintenance
      - external_dependency_failures

  investigation_latency:
    target: 95th_percentile < 5s
    measurement_window: 24h
    conditions:
      - ai_services_available: true

  action_success_rate:
    target: 90%
    measurement_window: 7d
    categories:
      - safe_actions: 95%
      - complex_actions: 85%

  context_api_performance:
    target: 99th_percentile < 100ms
    measurement_window: 1h
    conditions:
      - cache_enabled: true
```

## Security and Compliance Monitoring

### Security Event Monitoring

**Security Metrics:**
```prometheus
# Authentication and authorization
kubernaut_auth_attempts_total{status="success"} 2456
kubernaut_auth_attempts_total{status="failed"} 12
kubernaut_auth_failures_rate 0.005

# API security
kubernaut_api_requests_total{endpoint="/health",status="403"} 0
kubernaut_rate_limit_violations_total{client="external"} 3
kubernaut_suspicious_activity_events_total{type="unusual_pattern"} 1
```

**Compliance Monitoring:**
- Data retention compliance (log rotation, metric retention)
- Access control audit trails (RBAC usage, permission changes)
- Encryption status monitoring (TLS certificate expiry, key rotation)
- Privacy compliance (PII handling, data anonymization)

---

## Integration and Deployment

### Monitoring Stack Deployment

**Kubernetes Deployment:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernaut-monitoring
spec:
  replicas: 2
  selector:
    matchLabels:
      app: kubernaut-monitoring
  template:
    spec:
      containers:
      - name: prometheus
        image: prom/prometheus:latest
        ports:
        - containerPort: 9090
        volumeMounts:
        - name: prometheus-config
          mountPath: /etc/prometheus

      - name: grafana
        image: grafana/grafana:latest
        ports:
        - containerPort: 3000
        env:
        - name: GF_SECURITY_ADMIN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: grafana-credentials
              key: admin-password
```

### Configuration Management

**Monitoring Configuration as Code:**
- Prometheus rules and alerts in version control
- Grafana dashboards exported as JSON
- AlertManager routing rules templated
- Monitoring infrastructure automated deployment

---

## Related Documentation

- [Alert Processing Flow](ALERT_PROCESSING_FLOW.md)
- [Resilience Patterns](RESILIENCE_PATTERNS.md)
- [Performance Requirements](PERFORMANCE_REQUIREMENTS.md)
- [Health Monitoring Design](HEARTBEAT_MONITORING_DESIGN.md)

---

*This document is maintained as part of the Kubernaut production architecture and is updated regularly based on operational feedback and monitoring best practices.*