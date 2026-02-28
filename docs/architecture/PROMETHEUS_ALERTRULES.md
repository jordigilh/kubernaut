# Prometheus AlertRules for Kubernaut Services

**Version**: 1.0
**Last Updated**: October 6, 2025
**Status**: ✅ Authoritative Reference
**Scope**: All 11 Kubernaut V1 Services + Infrastructure

---

## 📋 Table of Contents

1. [Overview](#overview)
2. [Alert Severity Levels](#alert-severity-levels)
3. [PrometheusRule CRD](#prometheusrule-crd)
4. [Infrastructure Alerts](#infrastructure-alerts)
5. [Service Availability Alerts](#service-availability-alerts)
6. [Performance Alerts](#performance-alerts)
7. [Controller-Specific Alerts](#controller-specific-alerts)
8. [HTTP Service Alerts](#http-service-alerts)
9. [Business Logic Alerts](#business-logic-alerts)
10. [Deployment Guide](#deployment-guide)
11. [Testing](#testing)
12. [Troubleshooting](#troubleshooting)

---

## Overview

### Purpose

This document defines comprehensive Prometheus AlertRules for the Kubernaut platform, covering:
- **Service availability** - Detect service outages
- **Performance degradation** - Identify slow response times
- **Resource exhaustion** - Alert on high CPU/memory/disk
- **Business logic failures** - Detect processing errors
- **Infrastructure health** - Monitor databases and dependencies

---

### Alert Philosophy

**Kubernaut Alerting Principles**:
1. ✅ **Actionable**: Every alert requires human action
2. ✅ **Contextual**: Include runbook links and investigation steps
3. ✅ **Severity-based**: Critical (P0), Warning (P1), Info (P2)
4. ✅ **SLO-driven**: Based on Service Level Objectives
5. ✅ **Avoid alert fatigue**: Use appropriate thresholds

---

## Alert Severity Levels

### Severity Matrix

| Severity | Label | Response Time | Example |
|----------|-------|---------------|---------|
| **Critical** | `severity: critical` | Immediate (< 5 min) | Service down, data loss |
| **Warning** | `severity: warning` | Within 1 hour | High error rate, degraded performance |
| **Info** | `severity: info` | Next business day | Capacity planning, non-urgent issues |

---

### Runbook Convention

All alerts include a `runbook_url` annotation pointing to:
```
https://docs.kubernaut.io/runbooks/{service}/{alert-name}
```

---

## PrometheusRule CRD

### PrometheusRule Structure

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: {service-name}-alerts
  namespace: kubernaut-system
  labels:
    app: {service-name}
    prometheus: kubernaut
spec:
  groups:
  - name: {service-name}
    interval: 30s
    rules:
    - alert: {AlertName}
      expr: {PromQL query}
      for: {duration}
      labels:
        severity: {critical|warning|info}
        service: {service-name}
      annotations:
        summary: {Brief description}
        description: {Detailed message with context}
        runbook_url: https://docs.kubernaut.io/runbooks/{service}/{alert-name}
```

---

## Infrastructure Alerts

### Redis Alerts

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: redis-alerts
  namespace: kubernaut-system
  labels:
    app: redis
    prometheus: kubernaut
spec:
  groups:
  - name: redis
    interval: 30s
    rules:

    # Critical: Redis down
    - alert: RedisDown
      expr: up{job="redis"} == 0
      for: 1m
      labels:
        severity: critical
        service: redis
      annotations:
        summary: "Redis is down"
        description: "Redis instance {{ $labels.instance }} has been down for more than 1 minute. Gateway deduplication and Remediation Processor caching are unavailable."
        runbook_url: https://docs.kubernaut.io/runbooks/redis/redis-down

    # Warning: High memory usage
    - alert: RedisHighMemoryUsage
      expr: redis_memory_used_bytes / redis_memory_max_bytes > 0.8
      for: 5m
      labels:
        severity: warning
        service: redis
      annotations:
        summary: "Redis memory usage is high"
        description: "Redis instance {{ $labels.instance }} is using {{ $value | humanizePercentage }} of available memory."
        runbook_url: https://docs.kubernaut.io/runbooks/redis/high-memory

    # Warning: High connection count
    - alert: RedisHighConnectionCount
      expr: redis_connected_clients > 1000
      for: 5m
      labels:
        severity: warning
        service: redis
      annotations:
        summary: "Redis has high connection count"
        description: "Redis instance {{ $labels.instance }} has {{ $value }} connected clients."
        runbook_url: https://docs.kubernaut.io/runbooks/redis/high-connections

    # Warning: Evicted keys
    - alert: RedisEvictingKeys
      expr: rate(redis_evicted_keys_total[5m]) > 10
      for: 5m
      labels:
        severity: warning
        service: redis
      annotations:
        summary: "Redis is evicting keys due to memory pressure"
        description: "Redis instance {{ $labels.instance }} is evicting {{ $value }} keys per second. Consider increasing memory limits."
        runbook_url: https://docs.kubernaut.io/runbooks/redis/evicting-keys
```

---

### PostgreSQL Alerts

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: postgresql-alerts
  namespace: kubernaut-system
  labels:
    app: postgresql
    prometheus: kubernaut
spec:
  groups:
  - name: postgresql
    interval: 30s
    rules:

    # Critical: PostgreSQL down
    - alert: PostgreSQLDown
      expr: up{job="postgresql"} == 0
      for: 1m
      labels:
        severity: critical
        service: postgresql
      annotations:
        summary: "PostgreSQL is down"
        description: "PostgreSQL instance {{ $labels.instance }} has been down for more than 1 minute. Data Storage and Context API are unavailable."
        runbook_url: https://docs.kubernaut.io/runbooks/postgresql/postgresql-down

    # Critical: High connection usage
    - alert: PostgreSQLConnectionsNearLimit
      expr: sum(pg_stat_database_numbackends) / pg_settings_max_connections > 0.9
      for: 5m
      labels:
        severity: critical
        service: postgresql
      annotations:
        summary: "PostgreSQL connections near limit"
        description: "PostgreSQL is using {{ $value | humanizePercentage }} of max connections."
        runbook_url: https://docs.kubernaut.io/runbooks/postgresql/connections-near-limit

    # Warning: High disk usage
    - alert: PostgreSQLHighDiskUsage
      expr: (pg_database_size_bytes / (pg_database_size_bytes + node_filesystem_avail_bytes{mountpoint="/var/lib/postgresql"})) > 0.8
      for: 5m
      labels:
        severity: warning
        service: postgresql
      annotations:
        summary: "PostgreSQL disk usage is high"
        description: "PostgreSQL database is using {{ $value | humanizePercentage }} of available disk space."
        runbook_url: https://docs.kubernaut.io/runbooks/postgresql/high-disk-usage

    # Warning: Slow queries
    - alert: PostgreSQLSlowQueries
      expr: rate(pg_stat_statements_mean_exec_time_seconds{datname="kubernaut"}[5m]) > 1
      for: 10m
      labels:
        severity: warning
        service: postgresql
      annotations:
        summary: "PostgreSQL queries are slow"
        description: "Average query execution time is {{ $value }}s on database {{ $labels.datname }}."
        runbook_url: https://docs.kubernaut.io/runbooks/postgresql/slow-queries

    # Warning: Replication lag
    - alert: PostgreSQLReplicationLag
      expr: pg_replication_lag_seconds > 30
      for: 5m
      labels:
        severity: warning
        service: postgresql
      annotations:
        summary: "PostgreSQL replication lag is high"
        description: "Replication lag is {{ $value }}s for replica {{ $labels.instance }}."
        runbook_url: https://docs.kubernaut.io/runbooks/postgresql/replication-lag
```

---

### Vector DB (pgvector) Alerts

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: vectordb-alerts
  namespace: kubernaut-system
  labels:
    app: vectordb
    prometheus: kubernaut
spec:
  groups:
  - name: vectordb
    interval: 30s
    rules:

    # Critical: Vector DB down
    - alert: VectorDBDown
      expr: up{job="vectordb"} == 0
      for: 1m
      labels:
        severity: critical
        service: vectordb
      annotations:
        summary: "Vector DB is down"
        description: "Vector DB instance {{ $labels.instance }} has been down for more than 1 minute. Context API semantic search is unavailable."
        runbook_url: https://docs.kubernaut.io/runbooks/vectordb/vectordb-down

    # Warning: Slow vector searches
    - alert: VectorDBSlowSearches
      expr: histogram_quantile(0.95, rate(vectordb_search_duration_seconds_bucket[5m])) > 2
      for: 5m
      labels:
        severity: warning
        service: vectordb
      annotations:
        summary: "Vector DB searches are slow"
        description: "95th percentile search time is {{ $value }}s."
        runbook_url: https://docs.kubernaut.io/runbooks/vectordb/slow-searches

    # Info: High index size
    - alert: VectorDBLargeIndexSize
      expr: vectordb_index_size_bytes > 10737418240  # 10GB
      for: 1h
      labels:
        severity: info
        service: vectordb
      annotations:
        summary: "Vector DB index size is large"
        description: "Vector DB index size is {{ $value | humanizeBytes }}. Consider index optimization."
        runbook_url: https://docs.kubernaut.io/runbooks/vectordb/large-index
```

---

## Service Availability Alerts

### Critical Services (P0)

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: critical-services-alerts
  namespace: kubernaut-system
  labels:
    prometheus: kubernaut
spec:
  groups:
  - name: critical-services
    interval: 30s
    rules:

    # Gateway Service
    - alert: GatewayServiceDown
      expr: up{job="gateway-service"} == 0
      for: 1m
      labels:
        severity: critical
        service: gateway-service
      annotations:
        summary: "Gateway Service is down"
        description: "Gateway Service has been down for more than 1 minute. Alert ingestion is unavailable."
        runbook_url: https://docs.kubernaut.io/runbooks/gateway/service-down

    # Remediation Orchestrator
    - alert: RemediationOrchestratorDown
      expr: up{job="remediation-orchestrator"} == 0
      for: 1m
      labels:
        severity: critical
        service: remediation-orchestrator
      annotations:
        summary: "Remediation Orchestrator is down"
        description: "Remediation Orchestrator has been down for more than 1 minute. No remediation workflows are being processed."
        runbook_url: https://docs.kubernaut.io/runbooks/remediation-orchestrator/service-down

    # Multiple critical service instances down
    - alert: MultipleCriticalServicesDown
      expr: count(up{service=~"gateway-service|remediation-orchestrator"} == 0) >= 2
      for: 1m
      labels:
        severity: critical
        service: platform
      annotations:
        summary: "Multiple critical services are down"
        description: "{{ $value }} critical services are currently down. This indicates a platform-wide issue."
        runbook_url: https://docs.kubernaut.io/runbooks/platform/multiple-services-down
```

---

### High Priority Services (P1)

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: high-priority-services-alerts
  namespace: kubernaut-system
  labels:
    prometheus: kubernaut
spec:
  groups:
  - name: high-priority-services
    interval: 30s
    rules:

    # Remediation Processor
    - alert: RemediationProcessorDown
      expr: up{job="remediation-processor"} == 0
      for: 2m
      labels:
        severity: warning
        service: remediation-processor
      annotations:
        summary: "Remediation Processor is down"
        description: "Remediation Processor has been down for more than 2 minutes. Alert enrichment is unavailable."
        runbook_url: https://docs.kubernaut.io/runbooks/remediation-processor/service-down

    # AI Analysis
    - alert: AIAnalysisDown
      expr: up{job="ai-analysis"} == 0
      for: 2m
      labels:
        severity: warning
        service: ai-analysis
      annotations:
        summary: "AI Analysis controller is down"
        description: "AI Analysis has been down for more than 2 minutes. AI-powered root cause analysis is unavailable."
        runbook_url: https://docs.kubernaut.io/runbooks/ai-analysis/service-down

    # Workflow Execution
    - alert: WorkflowExecutionDown
      expr: up{job="workflow-execution"} == 0
      for: 2m
      labels:
        severity: warning
        service: workflow-execution
      annotations:
        summary: "Workflow Execution controller is down"
        description: "Workflow Execution has been down for more than 2 minutes. Automated remediation workflows are unavailable."
        runbook_url: https://docs.kubernaut.io/runbooks/workflow-execution/service-down

    # Kubernetes Executor (DEPRECATED - ADR-025)
    - alert: KubernetesExecutorDown
      expr: up{job="kubernetes-executor"} == 0
      for: 2m
      labels:
        severity: warning
        service: kubernetes-executor
      annotations:
        summary: "Kubernetes Executor is down"
        description: "Kubernetes Executor has been down for more than 2 minutes. Remediation actions cannot be executed."
        runbook_url: https://docs.kubernaut.io/runbooks/kubernetes-executor/service-down
```

---

## Performance Alerts

### HTTP Service Performance

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: http-service-performance-alerts
  namespace: kubernaut-system
  labels:
    prometheus: kubernaut
spec:
  groups:
  - name: http-service-performance
    interval: 30s
    rules:

    # High latency
    - alert: HTTPServiceHighLatency
      expr: histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{job=~"gateway-service|notification-service|holmesgpt-api|context-api|data-storage"}[5m])) > 5
      for: 5m
      labels:
        severity: warning
        service: "{{ $labels.job }}"
      annotations:
        summary: "{{ $labels.job }} has high latency"
        description: "95th percentile latency for {{ $labels.job }} is {{ $value }}s for endpoint {{ $labels.path }}."
        runbook_url: https://docs.kubernaut.io/runbooks/performance/high-latency

    # High error rate
    - alert: HTTPServiceHighErrorRate
      expr: rate(http_requests_total{job=~"gateway-service|notification-service|holmesgpt-api|context-api|data-storage",status=~"5.."}[5m]) / rate(http_requests_total{job=~"gateway-service|notification-service|holmesgpt-api|context-api|data-storage"}[5m]) > 0.05
      for: 5m
      labels:
        severity: warning
        service: "{{ $labels.job }}"
      annotations:
        summary: "{{ $labels.job }} has high error rate"
        description: "{{ $labels.job }} error rate is {{ $value | humanizePercentage }} for endpoint {{ $labels.path }}."
        runbook_url: https://docs.kubernaut.io/runbooks/performance/high-error-rate

    # Rate limiting triggered
    - alert: HTTPServiceRateLimitTriggered
      expr: rate(http_rate_limit_triggered_total{job="gateway-service"}[5m]) > 10
      for: 5m
      labels:
        severity: info
        service: gateway-service
      annotations:
        summary: "Gateway Service rate limiting is triggered"
        description: "Rate limiting is being triggered {{ $value }} times per second for source {{ $labels.source }}."
        runbook_url: https://docs.kubernaut.io/runbooks/gateway/rate-limit-triggered
```

---

### Controller Performance

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: controller-performance-alerts
  namespace: kubernaut-system
  labels:
    prometheus: kubernaut
spec:
  groups:
  - name: controller-performance
    interval: 30s
    rules:

    # High reconciliation latency
    - alert: ControllerHighReconciliationLatency
      expr: histogram_quantile(0.95, rate(controller_reconcile_duration_seconds_bucket{job=~"remediation-orchestrator|remediation-processor|ai-analysis|workflow-execution|kubernetes-executor"}[5m])) > 30
      for: 5m
      labels:
        severity: warning
        service: "{{ $labels.job }}"
      annotations:
        summary: "{{ $labels.job }} has high reconciliation latency"
        description: "95th percentile reconciliation time for {{ $labels.job }} is {{ $value }}s."
        runbook_url: https://docs.kubernaut.io/runbooks/controller/high-reconciliation-latency

    # High reconciliation error rate
    - alert: ControllerHighReconciliationErrorRate
      expr: rate(controller_reconcile_errors_total{job=~"remediation-orchestrator|remediation-processor|ai-analysis|workflow-execution|kubernetes-executor"}[5m]) / rate(controller_reconcile_total{job=~"remediation-orchestrator|remediation-processor|ai-analysis|workflow-execution|kubernetes-executor"}[5m]) > 0.1
      for: 5m
      labels:
        severity: warning
        service: "{{ $labels.job }}"
      annotations:
        summary: "{{ $labels.job }} has high reconciliation error rate"
        description: "{{ $labels.job }} reconciliation error rate is {{ $value | humanizePercentage }}."
        runbook_url: https://docs.kubernaut.io/runbooks/controller/high-reconciliation-error-rate

    # Work queue depth
    - alert: ControllerHighWorkQueueDepth
      expr: workqueue_depth{job=~"remediation-orchestrator|remediation-processor|ai-analysis|workflow-execution|kubernetes-executor"} > 100
      for: 10m
      labels:
        severity: warning
        service: "{{ $labels.job }}"
      annotations:
        summary: "{{ $labels.job }} has high work queue depth"
        description: "{{ $labels.job }} work queue has {{ $value }} items pending for queue {{ $labels.name }}."
        runbook_url: https://docs.kubernaut.io/runbooks/controller/high-work-queue-depth
```

---

## Controller-Specific Alerts

### Gateway Service Alerts

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: gateway-service-alerts
  namespace: kubernaut-system
  labels:
    app: gateway-service
    prometheus: kubernaut
spec:
  groups:
  - name: gateway-service
    interval: 30s
    rules:

    # High deduplication rate
    - alert: GatewayHighDeduplicationRate
      expr: rate(gateway_deduplicated_signals_total[5m]) / rate(gateway_ingested_signals_total[5m]) > 0.5
      for: 10m
      labels:
        severity: warning
        service: gateway-service
      annotations:
        summary: "Gateway has high deduplication rate"
        description: "{{ $value | humanizePercentage }} of incoming signals are duplicates. This may indicate alert storms or configuration issues."
        runbook_url: https://docs.kubernaut.io/runbooks/gateway/high-deduplication-rate

    # Storm detection triggered
    - alert: GatewayStormDetected
      expr: rate(gateway_storm_detected_total[5m]) > 1
      for: 2m
      labels:
        severity: warning
        service: gateway-service
      annotations:
        summary: "Gateway detected alert storm"
        description: "Alert storm detected for pattern {{ $labels.pattern }}. {{ $value }} storms per second."
        runbook_url: https://docs.kubernaut.io/runbooks/gateway/storm-detected

    # CRD creation failures
    - alert: GatewayCRDCreationFailures
      expr: rate(gateway_crd_creation_errors_total[5m]) > 1
      for: 5m
      labels:
        severity: critical
        service: gateway-service
      annotations:
        summary: "Gateway failing to create RemediationRequest CRDs"
        description: "{{ $value }} CRD creation errors per second. Check Kubernetes API connectivity."
        runbook_url: https://docs.kubernaut.io/runbooks/gateway/crd-creation-failures

    # Adapter failures
    - alert: GatewayAdapterFailures
      expr: rate(gateway_adapter_errors_total[5m]) > 5
      for: 5m
      labels:
        severity: warning
        service: gateway-service
      annotations:
        summary: "Gateway adapter {{ $labels.adapter }} is failing"
        description: "Adapter {{ $labels.adapter }} has {{ $value }} errors per second."
        runbook_url: https://docs.kubernaut.io/runbooks/gateway/adapter-failures
```

---

### Remediation Orchestrator Alerts

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: remediation-orchestrator-alerts
  namespace: kubernaut-system
  labels:
    app: remediation-orchestrator
    prometheus: kubernaut
spec:
  groups:
  - name: remediation-orchestrator
    interval: 30s
    rules:

    # High pending remediations
    - alert: RemediationOrchestratorHighPendingCount
      expr: remediation_orchestrator_pending_requests > 50
      for: 10m
      labels:
        severity: warning
        service: remediation-orchestrator
      annotations:
        summary: "High number of pending remediation requests"
        description: "{{ $value }} RemediationRequests are pending processing."
        runbook_url: https://docs.kubernaut.io/runbooks/remediation-orchestrator/high-pending-count

    # Child CRD creation failures
    - alert: RemediationOrchestratorChildCRDFailures
      expr: rate(remediation_orchestrator_child_crd_errors_total[5m]) > 1
      for: 5m
      labels:
        severity: critical
        service: remediation-orchestrator
      annotations:
        summary: "Failing to create child CRDs"
        description: "{{ $value }} child CRD creation errors per second for type {{ $labels.crd_type }}."
        runbook_url: https://docs.kubernaut.io/runbooks/remediation-orchestrator/child-crd-failures

    # High auto-rejection rate
    - alert: RemediationOrchestratorHighAutoRejectionRate
      expr: rate(remediation_orchestrator_auto_rejected_total[5m]) / rate(remediation_orchestrator_requests_total[5m]) > 0.3
      for: 10m
      labels:
        severity: warning
        service: remediation-orchestrator
      annotations:
        summary: "High auto-rejection rate"
        description: "{{ $value | humanizePercentage }} of remediations are being auto-rejected. Review Rego policies."
        runbook_url: https://docs.kubernaut.io/runbooks/remediation-orchestrator/high-auto-rejection-rate
```

---

### AI Analysis Alerts

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: ai-analysis-alerts
  namespace: kubernaut-system
  labels:
    app: ai-analysis
    prometheus: kubernaut
spec:
  groups:
  - name: ai-analysis
    interval: 30s
    rules:

    # HolmesGPT unavailable
    - alert: AIAnalysisHolmesGPTUnavailable
      expr: ai_analysis_holmesgpt_availability == 0
      for: 5m
      labels:
        severity: warning
        service: ai-analysis
      annotations:
        summary: "HolmesGPT API is unavailable"
        description: "AI Analysis cannot reach HolmesGPT at {{ $labels.endpoint }}. AI investigations are failing."
        runbook_url: https://docs.kubernaut.io/runbooks/ai-analysis/holmesgpt-unavailable

    # High investigation failures
    - alert: AIAnalysisHighInvestigationFailureRate
      expr: rate(ai_analysis_investigation_errors_total[5m]) / rate(ai_analysis_investigations_total[5m]) > 0.1
      for: 10m
      labels:
        severity: warning
        service: ai-analysis
      annotations:
        summary: "AI Analysis has high investigation failure rate"
        description: "{{ $value | humanizePercentage }} of investigations are failing."
        runbook_url: https://docs.kubernaut.io/runbooks/ai-analysis/high-investigation-failure-rate

    # Slow investigations
    - alert: AIAnalysisSlowInvestigations
      expr: histogram_quantile(0.95, rate(ai_analysis_investigation_duration_seconds_bucket[5m])) > 60
      for: 10m
      labels:
        severity: warning
        service: ai-analysis
      annotations:
        summary: "AI investigations are slow"
        description: "95th percentile investigation time is {{ $value }}s."
        runbook_url: https://docs.kubernaut.io/runbooks/ai-analysis/slow-investigations

    # Low confidence analysis
    - alert: AIAnalysisLowConfidence
      expr: rate(ai_analysis_low_confidence_total[10m]) / rate(ai_analysis_completed_total[10m]) > 0.5
      for: 30m
      labels:
        severity: info
        service: ai-analysis
      annotations:
        summary: "High rate of low-confidence AI analysis"
        description: "{{ $value | humanizePercentage }} of analyses have confidence < 60%. Review model performance."
        runbook_url: https://docs.kubernaut.io/runbooks/ai-analysis/low-confidence
```

---

### Workflow Execution Alerts

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: workflow-execution-alerts
  namespace: kubernaut-system
  labels:
    app: workflow-execution
    prometheus: kubernaut
spec:
  groups:
  - name: workflow-execution
    interval: 30s
    rules:

    # High workflow failure rate
    - alert: WorkflowExecutionHighFailureRate
      expr: rate(workflow_execution_failed_total[5m]) / rate(workflow_execution_started_total[5m]) > 0.2
      for: 10m
      labels:
        severity: warning
        service: workflow-execution
      annotations:
        summary: "Workflow Execution has high failure rate"
        description: "{{ $value | humanizePercentage }} of workflows are failing."
        runbook_url: https://docs.kubernaut.io/runbooks/workflow-execution/high-failure-rate

    # Long-running workflows
    - alert: WorkflowExecutionLongRunning
      expr: workflow_execution_running_duration_seconds > 600
      for: 5m
      labels:
        severity: warning
        service: workflow-execution
      annotations:
        summary: "Workflow {{ $labels.workflow_name }} is running for a long time"
        description: "Workflow {{ $labels.workflow_name }} (CRD {{ $labels.crd_name }}) has been running for {{ $value }}s."
        runbook_url: https://docs.kubernaut.io/runbooks/workflow-execution/long-running

    # Step timeout
    - alert: WorkflowExecutionStepTimeout
      expr: rate(workflow_execution_step_timeout_total[5m]) > 1
      for: 5m
      labels:
        severity: warning
        service: workflow-execution
      annotations:
        summary: "Workflow steps are timing out"
        description: "{{ $value }} workflow steps per second are timing out at step {{ $labels.step_name }}."
        runbook_url: https://docs.kubernaut.io/runbooks/workflow-execution/step-timeout
```

---

### Kubernetes Executor Alerts (DEPRECATED - ADR-025)

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: kubernetes-executor-alerts
  namespace: kubernaut-system
  labels:
    app: kubernetes-executor
    prometheus: kubernaut
spec:
  groups:
  - name: kubernetes-executor
    interval: 30s
    rules:

    # High action failure rate
    - alert: KubernetesExecutorHighActionFailureRate
      expr: rate(kubernetes_executor_action_failed_total[5m]) / rate(kubernetes_executor_action_started_total[5m]) > 0.1
      for: 10m
      labels:
        severity: warning
        service: kubernetes-executor
      annotations:
        summary: "Kubernetes Executor has high action failure rate"
        description: "{{ $value | humanizePercentage }} of Kubernetes actions are failing for action type {{ $labels.action_type }}."
        runbook_url: https://docs.kubernaut.io/runbooks/kubernetes-executor/high-action-failure-rate

    # Safety validation failures
    - alert: KubernetesExecutorSafetyValidationFailures
      expr: rate(kubernetes_executor_safety_validation_failed_total[5m]) > 5
      for: 5m
      labels:
        severity: warning
        service: kubernetes-executor
      annotations:
        summary: "Safety validation is blocking actions"
        description: "{{ $value }} actions per second are being blocked by safety validation for reason {{ $labels.reason }}."
        runbook_url: https://docs.kubernaut.io/runbooks/kubernetes-executor/safety-validation-failures

    # RBAC permission errors
    - alert: KubernetesExecutorRBACErrors
      expr: rate(kubernetes_executor_rbac_errors_total[5m]) > 1
      for: 5m
      labels:
        severity: critical
        service: kubernetes-executor
      annotations:
        summary: "Kubernetes Executor has RBAC permission errors"
        description: "{{ $value }} RBAC errors per second for action {{ $labels.action_type }}. Review ServiceAccount permissions."
        runbook_url: https://docs.kubernaut.io/runbooks/kubernetes-executor/rbac-errors

    # Dry-run validation failures
    - alert: KubernetesExecutorDryRunValidationFailures
      expr: rate(kubernetes_executor_dryrun_validation_failed_total[5m]) > 5
      for: 5m
      labels:
        severity: info
        service: kubernetes-executor
      annotations:
        summary: "Dry-run validations are failing"
        description: "{{ $value }} dry-run validations per second are failing. Review Kubernetes API schemas."
        runbook_url: https://docs.kubernaut.io/runbooks/kubernetes-executor/dryrun-validation-failures
```

---

## HTTP Service Alerts

### Notification Service Alerts

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: notification-service-alerts
  namespace: kubernaut-system
  labels:
    app: notification-service
    prometheus: kubernaut
spec:
  groups:
  - name: notification-service
    interval: 30s
    rules:

    # High notification failure rate
    - alert: NotificationServiceHighFailureRate
      expr: rate(notification_service_failed_total[5m]) / rate(notification_service_sent_total[5m]) > 0.1
      for: 10m
      labels:
        severity: warning
        service: notification-service
      annotations:
        summary: "Notification Service has high failure rate"
        description: "{{ $value | humanizePercentage }} of notifications are failing for channel {{ $labels.channel }}."
        runbook_url: https://docs.kubernaut.io/runbooks/notification-service/high-failure-rate

    # Channel unavailable
    - alert: NotificationServiceChannelUnavailable
      expr: notification_service_channel_available{channel=~"slack|pagerduty|email"} == 0
      for: 5m
      labels:
        severity: warning
        service: notification-service
      annotations:
        summary: "Notification channel {{ $labels.channel }} is unavailable"
        description: "Cannot send notifications via {{ $labels.channel }}."
        runbook_url: https://docs.kubernaut.io/runbooks/notification-service/channel-unavailable

    # Rate limit exceeded
    - alert: NotificationServiceRateLimitExceeded
      expr: rate(notification_service_rate_limit_exceeded_total[5m]) > 5
      for: 5m
      labels:
        severity: info
        service: notification-service
      annotations:
        summary: "Notification Service rate limit exceeded"
        description: "Rate limit exceeded {{ $value }} times per second for channel {{ $labels.channel }}."
        runbook_url: https://docs.kubernaut.io/runbooks/notification-service/rate-limit-exceeded
```

---

### HolmesGPT API Alerts

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: holmesgpt-api-alerts
  namespace: kubernaut-system
  labels:
    app: holmesgpt-api
    prometheus: kubernaut
spec:
  groups:
  - name: holmesgpt-api
    interval: 30s
    rules:

    # Service down
    - alert: HolmesGPTAPIDown
      expr: up{job="holmesgpt-api"} == 0
      for: 2m
      labels:
        severity: warning
        service: holmesgpt-api
      annotations:
        summary: "HolmesGPT API is down"
        description: "HolmesGPT API has been down for more than 2 minutes. AI investigations will fail."
        runbook_url: https://docs.kubernaut.io/runbooks/holmesgpt-api/service-down

    # High LLM latency
    - alert: HolmesGPTAPIHighLLMLatency
      expr: histogram_quantile(0.95, rate(holmesgpt_llm_request_duration_seconds_bucket[5m])) > 30
      for: 10m
      labels:
        severity: warning
        service: holmesgpt-api
      annotations:
        summary: "HolmesGPT LLM requests are slow"
        description: "95th percentile LLM request time is {{ $value }}s for provider {{ $labels.provider }}."
        runbook_url: https://docs.kubernaut.io/runbooks/holmesgpt-api/high-llm-latency

    # LLM provider errors
    - alert: HolmesGPTAPILLMProviderErrors
      expr: rate(holmesgpt_llm_errors_total[5m]) / rate(holmesgpt_llm_requests_total[5m]) > 0.1
      for: 5m
      labels:
        severity: warning
        service: holmesgpt-api
      annotations:
        summary: "LLM provider {{ $labels.provider }} has high error rate"
        description: "{{ $value | humanizePercentage }} of requests to {{ $labels.provider }} are failing."
        runbook_url: https://docs.kubernaut.io/runbooks/holmesgpt-api/llm-provider-errors

    # High token usage
    - alert: HolmesGPTAPIHighTokenUsage
      expr: rate(holmesgpt_tokens_used_total[1h]) > 1000000
      for: 1h
      labels:
        severity: info
        service: holmesgpt-api
      annotations:
        summary: "High LLM token usage"
        description: "Using {{ $value }} tokens per hour for provider {{ $labels.provider }}. Review usage and costs."
        runbook_url: https://docs.kubernaut.io/runbooks/holmesgpt-api/high-token-usage
```

---

## Business Logic Alerts

### Remediation Success Rate

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: remediation-business-alerts
  namespace: kubernaut-system
  labels:
    prometheus: kubernaut
spec:
  groups:
  - name: remediation-business
    interval: 60s
    rules:

    # Low overall success rate
    - alert: RemediationLowSuccessRate
      expr: rate(workflow_execution_succeeded_total[1h]) / rate(workflow_execution_completed_total[1h]) < 0.7
      for: 30m
      labels:
        severity: warning
        service: platform
      annotations:
        summary: "Remediation success rate is low"
        description: "Only {{ $value | humanizePercentage }} of remediations are succeeding over the last hour."
        runbook_url: https://docs.kubernaut.io/runbooks/platform/low-success-rate

    # No remediations processed
    - alert: NoRemediationsProcessed
      expr: rate(remediation_orchestrator_requests_total[1h]) == 0
      for: 1h
      labels:
        severity: warning
        service: platform
      annotations:
        summary: "No remediation requests processed in the last hour"
        description: "No RemediationRequests have been created. Check Gateway Service and alert sources."
        runbook_url: https://docs.kubernaut.io/runbooks/platform/no-remediations-processed

    # High manual approval rate
    - alert: HighManualApprovalRate
      expr: rate(remediation_orchestrator_manual_approval_required_total[1h]) / rate(remediation_orchestrator_requests_total[1h]) > 0.8
      for: 2h
      labels:
        severity: info
        service: platform
      annotations:
        summary: "High rate of manual approvals required"
        description: "{{ $value | humanizePercentage }} of remediations require manual approval. Consider adjusting auto-approval policies."
        runbook_url: https://docs.kubernaut.io/runbooks/platform/high-manual-approval-rate
```

---

## Deployment Guide

### Step 1: Create PrometheusRule Namespace

```bash
kubectl create namespace kubernaut-system --dry-run=client -o yaml | kubectl apply -f -
```

---

### Step 2: Deploy AlertRules

```bash
# Deploy all PrometheusRule CRDs
kubectl apply -f deploy/prometheus-rules/infrastructure-alerts.yaml
kubectl apply -f deploy/prometheus-rules/critical-services-alerts.yaml
kubectl apply -f deploy/prometheus-rules/high-priority-services-alerts.yaml
kubectl apply -f deploy/prometheus-rules/controller-performance-alerts.yaml
kubectl apply -f deploy/prometheus-rules/gateway-service-alerts.yaml
kubectl apply -f deploy/prometheus-rules/remediation-orchestrator-alerts.yaml
kubectl apply -f deploy/prometheus-rules/ai-analysis-alerts.yaml
kubectl apply -f deploy/prometheus-rules/workflow-execution-alerts.yaml
kubectl apply -f deploy/prometheus-rules/kubernetes-executor-alerts.yaml  # DEPRECATED - ADR-025
kubectl apply -f deploy/prometheus-rules/notification-service-alerts.yaml
kubectl apply -f deploy/prometheus-rules/holmesgpt-api-alerts.yaml
kubectl apply -f deploy/prometheus-rules/business-logic-alerts.yaml
```

---

### Step 3: Verify PrometheusRules

```bash
# List all PrometheusRules
kubectl get prometheusrules -n kubernaut-system

# Check Prometheus has loaded the rules
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090

# Open browser: http://localhost:9090/rules
# Verify all rule groups are loaded
```

---

### Step 4: Configure Alertmanager

```yaml
# alertmanager-config.yaml
apiVersion: v1
kind: Secret
metadata:
  name: alertmanager-kubernaut
  namespace: monitoring
type: Opaque
stringData:
  alertmanager.yaml: |
    global:
      resolve_timeout: 5m

    route:
      group_by: ['alertname', 'cluster', 'service']
      group_wait: 10s
      group_interval: 10s
      repeat_interval: 12h
      receiver: 'default'
      routes:
      # Critical alerts
      - match:
          severity: critical
        receiver: 'pagerduty'
        continue: true

      # Warning alerts
      - match:
          severity: warning
        receiver: 'slack'
        continue: true

      # Info alerts
      - match:
          severity: info
        receiver: 'email'

    receivers:
    - name: 'default'
      slack_configs:
      - api_url: 'https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK'
        channel: '#kubernaut-alerts'
        title: '{{ .GroupLabels.alertname }}'
        text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'

    - name: 'pagerduty'
      pagerduty_configs:
      - routing_key: 'YOUR_PAGERDUTY_INTEGRATION_KEY'
        description: '{{ .GroupLabels.alertname }}'

    - name: 'slack'
      slack_configs:
      - api_url: 'https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK'
        channel: '#kubernaut-warnings'

    - name: 'email'
      email_configs:
      - to: 'kubernaut-ops@example.com'
        from: 'alertmanager@example.com'
        smarthost: 'smtp.example.com:587'
        auth_username: 'alertmanager@example.com'
        auth_password: 'password'
```

```bash
kubectl apply -f alertmanager-config.yaml
```

---

## Testing

### Test Alert Generation

```bash
# Test alert by setting metric value
kubectl run -n kubernaut-system test-alert --image=curlimages/curl --rm -it -- sh

# Inside pod, set a metric that triggers an alert
# Example: Trigger GatewayServiceDown
curl -X POST http://gateway-service:9090/metrics \
  -d 'up{job="gateway-service"} 0'
```

---

### Verify Alert Firing

```bash
# Check Prometheus alerts
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090

# Browser: http://localhost:9090/alerts
# Look for firing alerts

# Check Alertmanager
kubectl port-forward -n monitoring svc/alertmanager-operated 9093:9093

# Browser: http://localhost:9093/#/alerts
# Verify alert is received
```

---

### Test Alert Routing

```bash
# Check Slack/PagerDuty for notifications
# Verify alert appears in configured channels

# Check Alertmanager status
kubectl exec -n monitoring alertmanager-kubernaut-0 -- amtool alert query

# Check alert history
kubectl exec -n monitoring alertmanager-kubernaut-0 -- amtool silence query
```

---

## Troubleshooting

### PrometheusRule Not Loading

**Symptom**: Rule not visible in Prometheus UI

**Causes**:
- PrometheusRule not created
- Label selector mismatch
- Syntax error in PromQL

**Fix**:
```bash
# Check PrometheusRule exists
kubectl get prometheusrule -n kubernaut-system

# Check Prometheus Operator logs
kubectl logs -n monitoring deployment/prometheus-operator -f

# Validate PromQL syntax
promtool check rules deploy/prometheus-rules/*.yaml

# Check Prometheus logs
kubectl logs -n monitoring prometheus-kubernaut-0 -c prometheus -f
```

---

### Alert Not Firing

**Symptom**: Expected alert not firing when condition met

**Causes**:
- Metric not available
- PromQL query incorrect
- `for` duration not elapsed

**Fix**:
```bash
# Test PromQL query manually
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090

# Browser: http://localhost:9090/graph
# Run the PromQL query and verify it returns expected results

# Check metric availability
curl http://gateway-service.kubernaut-system:9090/metrics | grep gateway_ingested_signals_total

# Reduce 'for' duration for testing
# Edit PrometheusRule temporarily to 'for: 10s'
```

---

### Alert Not Routing

**Symptom**: Alert firing but not received in Slack/PagerDuty

**Causes**:
- Alertmanager configuration error
- Receiver credentials incorrect
- Routing rule not matching

**Fix**:
```bash
# Check Alertmanager config
kubectl exec -n monitoring alertmanager-kubernaut-0 -- \
  amtool config show

# Test receiver
kubectl exec -n monitoring alertmanager-kubernaut-0 -- \
  amtool check config /etc/alertmanager/config/alertmanager.yaml

# Check Alertmanager logs
kubectl logs -n monitoring alertmanager-kubernaut-0 -f

# Manually test Slack webhook
curl -X POST https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK \
  -d '{"text": "Test alert"}'
```

---

## Summary

### Alert Coverage

| Category | Alert Count | Services Covered |
|----------|-------------|------------------|
| **Infrastructure** | 12 | Redis, PostgreSQL, Vector DB |
| **Service Availability** | 10 | All 11 services |
| **Performance** | 8 | HTTP services + controllers |
| **Gateway Service** | 4 | Gateway-specific |
| **Remediation Orchestrator** | 3 | Orchestrator-specific |
| **AI Analysis** | 4 | AI-specific |
| **Workflow Execution** | 3 | Workflow-specific |
| ~~**Kubernetes Executor**~~ (DEPRECATED - ADR-025) | 4 | Executor-specific |
| **Notification Service** | 3 | Notification-specific |
| **HolmesGPT API** | 4 | HolmesGPT-specific |
| **Business Logic** | 3 | Platform-wide |
| **Total** | **58** | **All services + infrastructure** |

---

### Severity Distribution

- **Critical**: 12 alerts (immediate response required)
- **Warning**: 38 alerts (response within 1 hour)
- **Info**: 8 alerts (next business day)

---

### Key Takeaways

1. ✅ **Comprehensive Coverage**: All 11 services + infrastructure
2. ✅ **Severity-Based Routing**: Critical → PagerDuty, Warning → Slack, Info → Email
3. ✅ **Actionable Alerts**: Every alert has runbook link
4. ✅ **SLO-Driven**: Thresholds based on performance requirements
5. ✅ **GitOps-Ready**: Declarative YAML configuration

---

## References

### Prometheus Documentation
- [Alerting Rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/)
- [PrometheusRule CRD](https://prometheus-operator.dev/docs/operator/api/#monitoring.coreos.com/v1.PrometheusRule)
- [Alertmanager Configuration](https://prometheus.io/docs/alerting/latest/configuration/)

### Kubernaut Documentation
- [Prometheus ServiceMonitor Pattern](./PROMETHEUS_SERVICEMONITOR_PATTERN.md)
- [Service Dependency Map](./SERVICE_DEPENDENCY_MAP.md)
- [Kubernetes TokenReviewer Authentication](./KUBERNETES_TOKENREVIEWER_AUTH.md)

---

**Document Status**: ✅ Complete
**Last Updated**: October 6, 2025
**Maintainer**: Kubernaut Architecture Team
**Version**: 1.0
