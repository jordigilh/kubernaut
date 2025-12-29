# Multi-Step Workflow Examples for Kubernaut

**Date**: October 17, 2025
**Purpose**: Document realistic multi-step remediation workflows to explain how workflows are created
**Related**: ADR-018 (Approval Notification Integration), `docs/value-proposition/TECHNICAL_SCENARIOS.md`

---

## 🎯 **Overview**

This document provides comprehensive examples of multi-step workflows in Kubernaut, demonstrating:
- How HolmesGPT generates multi-step recommendations with dependencies
- How WorkflowExecution orchestrates parallel and sequential steps
- How approval gates integrate into multi-step workflows
- Real-world scenarios with timing and MTTR targets

---

## 📋 **Table of Contents**

1. [Example 1: OOMKill Memory Leak (7-Step Workflow)](#example-1-oomkill-memory-leak)
2. [Example 2: Cascading Failure (9-Step Workflow)](#example-2-cascading-failure)
3. [Example 3: Alert Storm Correlation (5-Step Workflow)](#example-3-alert-storm)
4. [Example 4: Database Deadlock (6-Step Workflow)](#example-4-database-deadlock)
5. [Workflow Patterns](#workflow-patterns)

---

## 📊 **Example 1: OOMKill Memory Leak (7-Step Workflow)**

### **Scenario**

**Alert**: `OOMKilled payment-service` (Prometheus)
**Context**: Payment service pod killed due to out-of-memory
**HolmesGPT Investigation**: Memory leak in payment processing coroutine (50MB/hr growth)
**Confidence**: 72.5% (Medium - requires approval)

---

### **HolmesGPT Recommendations (Self-Documenting JSON)**

**Source**: AIAnalysis CRD `status.approvalContext.recommendedActions`

```json
{
  "recommendations": [
    {
      "id": "rec-001",
      "action": "collect_diagnostics",
      "description": "Capture heap dump before any changes",
      "confidence": 0.98,
      "parameters": {
        "type": "heap_dump",
        "deployment": "payment-service",
        "namespace": "production"
      },
      "dependencies": [],
      "rationale": "Preserve memory state for root cause analysis",
      "risk": "low"
    },
    {
      "id": "rec-002",
      "action": "backup_data",
      "description": "Backup recent transaction logs",
      "confidence": 0.95,
      "parameters": {
        "deployment": "payment-service",
        "namespace": "production",
        "paths": ["/var/log/payments"]
      },
      "dependencies": [],
      "rationale": "Preserve logs before restart clears them",
      "risk": "low"
    },
    {
      "id": "rec-003",
      "action": "increase_resources",
      "description": "Increase memory limit from 2Gi to 3Gi",
      "confidence": 0.88,
      "parameters": {
        "deployment": "payment-service",
        "namespace": "production",
        "memory": "3Gi"
      },
      "dependencies": ["rec-001", "rec-002"],
      "rationale": "Based on 50MB/hr growth, 3Gi provides 20h buffer",
      "risk": "low"
    },
    {
      "id": "rec-004",
      "action": "restart_pod",
      "description": "Rolling restart to clear leaked memory",
      "confidence": 0.95,
      "parameters": {
        "deployment": "payment-service",
        "namespace": "production",
        "strategy": "rolling"
      },
      "dependencies": ["rec-003"],
      "rationale": "Clear accumulated leaked memory (92% historical success)",
      "risk": "medium",
      "requiresApproval": true
    },
    {
      "id": "rec-005",
      "action": "enable_debug_mode",
      "description": "Enable memory profiling for ongoing analysis",
      "confidence": 0.85,
      "parameters": {
        "deployment": "payment-service",
        "namespace": "production",
        "debug_flags": ["--memory-profile=true"]
      },
      "dependencies": ["rec-004"],
      "rationale": "Monitor memory usage to validate fix",
      "risk": "low"
    },
    {
      "id": "rec-006",
      "action": "update_hpa",
      "description": "Cap max replicas to prevent resource exhaustion",
      "confidence": 0.80,
      "parameters": {
        "hpa": "payment-service-hpa",
        "namespace": "production",
        "maxReplicas": 8
      },
      "dependencies": ["rec-004"],
      "rationale": "Prevent cascading failure if leak persists",
      "risk": "low"
    },
    {
      "id": "rec-007",
      "action": "notify_only",
      "description": "File bug report with heap dump analysis",
      "confidence": 1.0,
      "parameters": {
        "type": "jira",
        "project": "PAYMENT",
        "priority": "high",
        "attachments": ["heap_dump_rec-001"]
      },
      "dependencies": ["rec-005", "rec-006"],
      "rationale": "Ensure development team addresses root cause",
      "risk": "none"
    }
  ],
  "estimatedDuration": "4 minutes",
  "historicalSuccessRate": 0.92
}
```

---

### **WorkflowExecution CRD (Generated)**

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: aianalysis-oomkill-12345-workflow
  namespace: production
spec:
  remediationRequestRef:
    name: remediation-oomkill-12345
  workflowDefinition:
    steps:
      # Parallel: Diagnostics + Backup (no dependencies)
      - stepNumber: 1
        name: "Collect Heap Dump"
        action: "collect_diagnostics"
        parameters:
          type: "heap_dump"
          deployment: "payment-service"
          namespace: "production"
        dependencies: []
        timeout: "2m"

      - stepNumber: 2
        name: "Backup Transaction Logs"
        action: "backup_data"
        parameters:
          deployment: "payment-service"
          namespace: "production"
          paths: ["/var/log/payments"]
        dependencies: []
        timeout: "1m"

      # Sequential: Increase Memory (after diagnostics)
      - stepNumber: 3
        name: "Increase Memory Limit"
        action: "increase_resources"
        parameters:
          deployment: "payment-service"
          namespace: "production"
          memory: "3Gi"
        dependencies: [1, 2]  # Wait for diagnostics + backup
        timeout: "30s"

      # Approval Gate: Restart Pod (after memory increase)
      - stepNumber: 4
        name: "Rolling Restart"
        action: "restart_pod"
        parameters:
          deployment: "payment-service"
          namespace: "production"
          strategy: "rolling"
        dependencies: [3]
        requiresApproval: true  # ⚠️ APPROVAL GATE
        timeout: "5m"

      # Parallel: Enable Debug + Update HPA (after restart)
      - stepNumber: 5
        name: "Enable Memory Profiling"
        action: "enable_debug_mode"
        parameters:
          deployment: "payment-service"
          namespace: "production"
          debug_flags: ["--memory-profile=true"]
        dependencies: [4]
        timeout: "30s"

      - stepNumber: 6
        name: "Cap Max Replicas"
        action: "update_hpa"
        parameters:
          hpa: "payment-service-hpa"
          namespace: "production"
          maxReplicas: 8
        dependencies: [4]
        timeout: "30s"

      # Final: File Bug Report (after monitoring setup)
      - stepNumber: 7
        name: "File Bug Report"
        action: "notify_only"
        parameters:
          type: "jira"
          project: "PAYMENT"
          priority: "high"
          attachments: ["heap_dump_step-1"]
        dependencies: [5, 6]
        timeout: "1m"

  executionStrategy:
    approvalRequired: false  # Step-level approval (step 4)
    dryRunFirst: true
    rollbackStrategy: "automatic"
status:
  phase: "planning"
```

---

### **Execution Timeline**

```
┌─────────────────────────────────────────────────────────────────┐
│                   WORKFLOW EXECUTION TIMELINE                     │
└─────────────────────────────────────────────────────────────────┘

T+0s: Workflow Created
      ├─> Step 1: collect_diagnostics (parallel) → 2min
      └─> Step 2: backup_data (parallel) → 1min

T+2m: Steps 1 & 2 Complete
      └─> Step 3: increase_resources → 30s

T+2m30s: Step 3 Complete
      └─> Step 4: restart_pod ⚠️ APPROVAL GATE
          ⏸️ WAITING FOR APPROVAL

T+2m30s: NotificationRequest Created
      ├─> Slack notification sent
      └─> Email notification sent

T+3m30s: Operator Approves (1min review)
      └─> Step 4: restart_pod (rolling) → 3min

T+6m30s: Step 4 Complete
      ├─> Step 5: enable_debug_mode (parallel) → 30s
      └─> Step 6: update_hpa (parallel) → 30s

T+7m: Steps 5 & 6 Complete
      └─> Step 7: notify_only → 10s

T+7m10s: Workflow Complete ✅

┌─────────────────────────────────────────────────────────────────┐
│ TOTAL MTTR: 7 minutes (includes 1min approval review)          │
│ Target MTTR: 4-5 minutes (if auto-approved, high confidence)   │
│ Manual MTTR: 60-90 minutes (without Kubernaut)                 │
│ Improvement: 88-93% reduction                                   │
└─────────────────────────────────────────────────────────────────┘
```

---

### **Dependency Graph Visualization (For Approval Notification)**

**Format**: Used in NotificationRequest body

```
Recommended Workflow:
┌─────────────────────────────────────────┐
│ Step 1: collect_diagnostics (parallel)  │
│ Step 2: backup_data (parallel)          │
└─────────────────┬───────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│ Step 3: increase_resources              │
│   Dependencies: [1, 2]                  │
└─────────────────┬───────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│ Step 4: restart_pod (APPROVAL GATE) ⚠️  │
│   Dependencies: [3]                     │
│   Risk: Medium (production restart)     │
└─────────────────┬───────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│ Step 5: enable_debug_mode (parallel)    │
│ Step 6: update_hpa (parallel)           │
│   Dependencies: [4]                     │
└─────────────────┬───────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│ Step 7: notify_only (bug report)        │
│   Dependencies: [5, 6]                  │
└─────────────────────────────────────────┘

Total Steps: 7 (2 parallel groups)
Estimated Duration: 4 minutes (without approval delay)
Historical Success Rate: 92%
```

---

## 📊 **Example 2: Cascading Failure (9-Step Workflow)**

### **Scenario**

**Alert**: `CheckoutServiceDown` (Multi-service failure)
**Context**: Checkout service cascading failure affecting 4 downstream services
**HolmesGPT Investigation**: PostgreSQL connection pool exhaustion (root cause)
**Confidence**: 85% (High - auto-approved)

---

### **HolmesGPT Recommendations**

```json
{
  "recommendations": [
    {
      "id": "rec-001",
      "action": "collect_diagnostics",
      "description": "Capture PostgreSQL connection metrics",
      "parameters": {
        "database": "fraud-detection-db",
        "metrics": ["connection_pool_size", "active_connections"]
      },
      "dependencies": [],
      "risk": "low"
    },
    {
      "id": "rec-002",
      "action": "patch_config_map",
      "description": "Increase PostgreSQL connection pool size",
      "parameters": {
        "configMap": "fraud-detection-config",
        "namespace": "production",
        "patch": {
          "DATABASE_POOL_SIZE": "200",
          "DATABASE_POOL_TIMEOUT": "30s"
        }
      },
      "dependencies": ["rec-001"],
      "rationale": "Increase from 100 to 200 based on load analysis",
      "risk": "low"
    },
    {
      "id": "rec-003",
      "action": "rolling_restart_pods",
      "description": "Rolling restart fraud-detection service",
      "parameters": {
        "deployment": "fraud-detection-service",
        "strategy": "one-at-a-time",
        "waitForReady": true
      },
      "dependencies": ["rec-002"],
      "rationale": "Apply new connection pool configuration",
      "risk": "medium"
    },
    {
      "id": "rec-004",
      "action": "scale_deployment",
      "description": "Scale fraud-detection from 5 to 8 replicas",
      "parameters": {
        "deployment": "fraud-detection-service",
        "replicas": 8
      },
      "dependencies": ["rec-003"],
      "rationale": "Increase capacity to handle load",
      "risk": "low"
    },
    {
      "id": "rec-005",
      "action": "health_check",
      "description": "Verify fraud-detection health",
      "parameters": {
        "deployment": "fraud-detection-service",
        "endpoint": "/health",
        "expectedStatus": 200
      },
      "dependencies": ["rec-004"],
      "risk": "none"
    },
    {
      "id": "rec-006",
      "action": "rolling_restart_pods",
      "description": "Restart checkout-service (depends on fraud-detection)",
      "parameters": {
        "deployment": "checkout-service",
        "strategy": "one-at-a-time"
      },
      "dependencies": ["rec-005"],
      "rationale": "Clear stale connections after upstream fix",
      "risk": "medium"
    },
    {
      "id": "rec-007",
      "action": "rolling_restart_pods",
      "description": "Restart payment-gateway (depends on fraud-detection)",
      "parameters": {
        "deployment": "payment-gateway",
        "strategy": "one-at-a-time"
      },
      "dependencies": ["rec-005"],
      "rationale": "Clear stale connections",
      "risk": "medium"
    },
    {
      "id": "rec-008",
      "action": "health_check",
      "description": "Verify all services healthy",
      "parameters": {
        "services": ["checkout-service", "payment-gateway", "fraud-detection-service"],
        "endpoint": "/health"
      },
      "dependencies": ["rec-006", "rec-007"],
      "risk": "none"
    },
    {
      "id": "rec-009",
      "action": "update_git_manifests",
      "description": "Update connection pool size in Git",
      "parameters": {
        "repository": "k8s-configs",
        "path": "production/fraud-detection/config.yaml",
        "patch": {"DATABASE_POOL_SIZE": "200"},
        "createPR": true
      },
      "dependencies": ["rec-008"],
      "rationale": "Persist configuration change",
      "risk": "none"
    }
  ],
  "estimatedDuration": "5 minutes",
  "historicalSuccessRate": 0.89
}
```

---

### **Execution Timeline**

```
T+0s: Workflow Created (Auto-approved, 85% confidence)

T+0s: Step 1: collect_diagnostics → 30s
      └─> Capture PostgreSQL metrics

T+30s: Step 2: patch_config_map → 10s
       └─> Increase pool size 100 → 200

T+40s: Step 3: rolling_restart_pods (fraud-detection) → 2min
       └─> Apply new configuration

T+2m40s: Step 4: scale_deployment → 30s
         └─> Scale 5 → 8 replicas

T+3m10s: Step 5: health_check → 10s
         └─> Verify fraud-detection healthy ✅

T+3m20s: Parallel Restarts
         ├─> Step 6: restart checkout-service → 1m30s
         └─> Step 7: restart payment-gateway → 1m30s

T+4m50s: Step 8: health_check (all services) → 10s
         └─> Verify end-to-end health ✅

T+5m: Step 9: update_git_manifests → 30s
      └─> Create PR for persistent config

T+5m30s: Workflow Complete ✅

┌─────────────────────────────────────────────────────────────────┐
│ TOTAL MTTR: 5.5 minutes                                         │
│ Manual MTTR: 45-60 minutes (trace dependency chain manually)   │
│ Improvement: 88-91% reduction                                   │
└─────────────────────────────────────────────────────────────────┘
```

---

### **Dependency Graph**

```
Step 1: collect_diagnostics
    ↓
Step 2: patch_config_map
    ↓
Step 3: rolling_restart (fraud-detection)
    ↓
Step 4: scale_deployment
    ↓
Step 5: health_check (fraud-detection)
    ↓
    ├─> Step 6: restart checkout-service (parallel)
    └─> Step 7: restart payment-gateway (parallel)
            ↓
        Step 8: health_check (all services)
            ↓
        Step 9: update_git_manifests

Total Steps: 9 (1 parallel group)
Parallel Savings: ~1.5 minutes
```

---

## 📊 **Example 3: Alert Storm Correlation (5-Step Workflow)**

### **Scenario**

**Alert**: `AlertStormDetected` (50 alerts in 2 minutes)
**Context**: Multiple services failing, alert storm triggered
**HolmesGPT Investigation**: Shared Redis cache failure (single root cause)
**Confidence**: 90% (High - auto-approved)

---

### **HolmesGPT Recommendations**

```json
{
  "recommendations": [
    {
      "id": "rec-001",
      "action": "correlate_alerts",
      "description": "Correlate 50 alerts to single root cause",
      "parameters": {
        "alerts": ["ServiceADown", "ServiceBSlow", "ServiceCTimeout", "..."],
        "correlationWindow": "5m"
      },
      "dependencies": [],
      "rationale": "Identify common failure point",
      "risk": "none"
    },
    {
      "id": "rec-002",
      "action": "health_check",
      "description": "Check Redis cluster health",
      "parameters": {
        "service": "redis-cluster",
        "namespace": "infrastructure"
      },
      "dependencies": ["rec-001"],
      "risk": "none"
    },
    {
      "id": "rec-003",
      "action": "restart_statefulset",
      "description": "Restart Redis cluster (detected unhealthy)",
      "parameters": {
        "statefulset": "redis-cluster",
        "namespace": "infrastructure",
        "strategy": "rolling"
      },
      "dependencies": ["rec-002"],
      "rationale": "Redis nodes in failed state",
      "risk": "high",
      "requiresApproval": false  # Auto-approved at 90%
    },
    {
      "id": "rec-004",
      "action": "silence_alerts",
      "description": "Silence correlated alerts during recovery",
      "parameters": {
        "alerts": ["ServiceADown", "ServiceBSlow", "..."],
        "duration": "5m",
        "reason": "Root cause identified and fixing"
      },
      "dependencies": ["rec-003"],
      "rationale": "Reduce alert noise during recovery",
      "risk": "low"
    },
    {
      "id": "rec-005",
      "action": "verify_recovery",
      "description": "Verify all services recovered",
      "parameters": {
        "services": ["service-a", "service-b", "service-c"],
        "healthEndpoint": "/health"
      },
      "dependencies": ["rec-003"],
      "risk": "none"
    }
  ],
  "estimatedDuration": "8 minutes",
  "historicalSuccessRate": 0.87
}
```

---

### **Execution Timeline**

```
T+0s: Alert Storm Detected (50 alerts)

T+0s: Step 1: correlate_alerts → 30s
      └─> Identify Redis as root cause

T+30s: Step 2: health_check (Redis) → 10s
       └─> Redis cluster unhealthy ❌

T+40s: Step 3: restart_statefulset (Redis) → 5min
       └─> Rolling restart Redis cluster

T+5m40s: Parallel Recovery
         ├─> Step 4: silence_alerts → 10s
         └─> Step 5: verify_recovery → 2min

T+7m40s: All Services Recovered ✅
         └─> Alert storm resolved

┌─────────────────────────────────────────────────────────────────┐
│ TOTAL MTTR: 8 minutes                                           │
│ Manual MTTR: 90-120 minutes (trace 50 alerts manually)         │
│ Improvement: 87-93% reduction                                   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 📋 **Workflow Patterns**

### **Pattern 1: Diagnostic → Fix → Verify**

**Use Case**: Most remediations follow this pattern

```
Step 1: collect_diagnostics (gather evidence)
    ↓
Step 2: apply_fix (remediation action)
    ↓
Step 3: health_check (verify success)
```

**Examples**:
- OOMKill: heap_dump → increase_memory → restart_pod → verify
- Disk Full: check_usage → cleanup_storage → verify_space
- Network Issue: trace_route → restart_network → verify_connectivity

---

### **Pattern 2: Parallel Diagnostics → Sequential Fix**

**Use Case**: Multiple diagnostic sources, single fix

```
Step 1: collect_diagnostics (parallel)
Step 2: backup_data (parallel)
    ↓
Step 3: apply_fix
    ↓
Step 4: verify_recovery
```

**Examples**:
- OOMKill example (collect_diagnostics + backup_data → increase_resources)
- Database issue (check_logs + check_connections → restart_database)

---

### **Pattern 3: Fix Root Cause → Cascade Restart**

**Use Case**: Cascading failures with dependency chain

```
Step 1: fix_root_cause (e.g., database)
    ↓
Step 2: restart_service_a (parallel)
Step 3: restart_service_b (parallel)
Step 4: restart_service_c (parallel)
    ↓
Step 5: verify_all_services
```

**Examples**:
- Cascading failure example (fix PostgreSQL → restart dependent services)
- Cache failure (fix Redis → restart all cache consumers)

---

### **Pattern 4: Correlate → Fix → Silence**

**Use Case**: Alert storm scenarios

```
Step 1: correlate_alerts
    ↓
Step 2: identify_root_cause
    ↓
Step 3: apply_fix
    ↓
Step 4: silence_correlated_alerts
    ↓
Step 5: verify_recovery
```

**Examples**:
- Alert storm example (correlate 50 alerts → fix Redis → silence alerts)

---

## 🎯 **Key Takeaways**

### **1. Multi-Step Workflows Are Common**

- **70% of remediations** involve 3+ steps (from HolmesGPT analysis)
- **Dependency-aware execution** enables parallel optimization
- **Target MTTR: 4-8 minutes** (vs. 60-90 minutes manual)

---

### **2. Approval Gates Enable Safety**

- **Step-level approval** (not workflow-level)
- **Risk-based gating**: High-risk steps (e.g., production restart) require approval
- **Approval notifications** provide full context for informed decisions

---

### **3. Dependency Graphs Optimize Execution**

- **Parallel execution**: Steps with no dependencies run simultaneously
- **Sequential execution**: Steps with dependencies wait for completion
- **Estimated duration**: HolmesGPT provides timing based on historical data

---

### **4. Workflows Are Reproducible**

- **GitOps integration**: Persist configuration changes via PR
- **Audit trail**: Every step tracked in WorkflowExecution status
- **Rollback support**: Automatic rollback if step fails

---

## 📚 **References**

1. **HolmesGPT Prompt Engineering**: `docs/services/crd-controllers/02-aianalysis/prompt-engineering-dependencies.md`
2. **Workflow Execution Mode**: `docs/analysis/WORKFLOW_EXECUTION_MODE_DETERMINATION.md`
3. **Technical Scenarios**: `docs/value-proposition/TECHNICAL_SCENARIOS.md`
4. **Canonical Action Types**: `docs/design/CANONICAL_ACTION_TYPES.md`
5. **ADR-018**: `docs/architecture/decisions/ADR-018-approval-notification-v1-integration.md`

---

**Document Owner**: Platform Architecture Team
**Review Frequency**: When new workflow patterns identified
**Next Review Date**: 2026-01-17

