# Effectiveness Monitor - Sequence Diagrams

**Date**: October 16, 2025
**Purpose**: Visual representation of Effectiveness Monitor workflows
**Service**: Effectiveness Monitor (Hybrid Automated + AI Analysis)
**Reference**: [DD-EFFECTIVENESS-001](decisions/DD-EFFECTIVENESS-001-Hybrid-Automated-AI-Analysis.md)

---

## Overview

The Effectiveness Monitor uses a **hybrid approach** with two distinct flows:

1. **Automated Assessment Only** (99.3% of cases) - Fast, computational analysis
2. **Automated + AI Analysis** (0.7% of cases) - Comprehensive AI-powered insights

This document provides detailed sequence diagrams for both scenarios with real-world examples.

---

## Flow 1: Automated Assessment Only (Routine Success)

### Scenario
- **Alert**: High memory usage (OOMKilled)
- **Action**: Scale deployment from 3 to 5 replicas
- **Result**: SUCCESS (memory stabilized)
- **Priority**: P2 (medium)
- **AI Decision**: SKIP (routine success, no anomalies)

### Sequence Diagram

```mermaid
sequenceDiagram
    participant K8s as Kubernetes API
    participant RR as RemediationRequest CRD
    participant EMC as EffectivenessMonitor<br/>Controller
    participant EMS as EffectivenessMonitor<br/>Service
    participant DS as Data Storage<br/>(PostgreSQL)
    participant IM as Infrastructure<br/>Monitoring

    Note over K8s,IM: Example: Scale deployment 3→5 replicas for OOMKilled alert

    %% Step 1: Remediation completes
    K8s->>RR: Update status.overallPhase = "completed"
    RR->>RR: Set success = true<br/>completionTime = 2025-10-16T10:30:00Z

    %% Step 2: Controller detects
    EMC->>RR: Watch event (overallPhase="completed")
    EMC->>EMC: Wait 5 minutes<br/>(stabilization period)

    Note over EMC: Watches RemediationRequest (user-facing API)<br/>Future-proof against workflow implementation changes

    EMC->>DS: Check if already processed<br/>(idempotency: RemediationRequest.UID)
    DS-->>EMC: Not processed

    %% Step 3: Trigger assessment
    EMC->>EMS: POST /internal/assess<br/>{<br/>  "workflow_execution_uid": "abc-123",<br/>  "trace_id": "trace-xyz",<br/>  "action_type": "scale_deployment"<br/>}

    %% Step 4: Automated assessment
    EMS->>DS: Get pre-execution metrics<br/>(t - 10min)
    DS-->>EMS: {"memory_usage": "95%", "pod_count": 3}

    EMS->>IM: Get post-execution metrics<br/>(current)
    IM-->>EMS: {"memory_usage": "62%", "pod_count": 5}

    EMS->>EMS: Calculate automated assessment:<br/>- Basic score: 0.85<br/>- Health checks: PASS<br/>- Metric improvements: memory -33%<br/>- Anomalies: []

    %% Step 5: Decision point
    EMS->>EMS: shouldCallAI()?<br/>- P2 priority + SUCCESS<br/>- No anomalies<br/>- Not new action type<br/>→ FALSE (routine success)

    Note over EMS: AI call skipped - routine success<br/>Automated assessment sufficient

    %% Step 6: Store results
    EMS->>DS: Store assessment:<br/>{<br/>  "effectiveness_score": 0.85,<br/>  "confidence": 0.75,<br/>  "analysis_type": "automated",<br/>  "metrics": {...}<br/>}
    DS-->>EMS: Stored (assessment_id: assess-456)

    %% Step 7: Update status
    EMS->>RR: Update annotation:<br/>"effectiveness.kubernaut.io/score": "0.85"<br/>"effectiveness.kubernaut.io/confidence": "0.75"
    RR-->>EMS: Updated

    EMS-->>EMC: 200 OK<br/>{<br/>  "assessment_id": "assess-456",<br/>  "effectiveness_score": 0.85,<br/>  "analysis_used": "automated_only"<br/>}

    Note over K8s,IM: Total time: ~150ms<br/>Cost: Negligible (computational only)
```

### Key Points - Automated Only

| Aspect | Value |
|--------|-------|
| **Trigger** | Routine success, no anomalies, non-P0 priority |
| **Duration** | ~150ms (fast) |
| **Cost** | Negligible (computational only) |
| **Frequency** | 3.65M/year (99.3% of all assessments) |
| **Confidence** | 70-80% (automated formulas) |
| **Components** | Controller, Service, Data Storage, Infra Monitoring |

---

## Flow 2: Automated + AI Analysis (P0 Failure)

### Scenario
- **Alert**: Critical API outage (P0)
- **Action**: Rollback deployment to previous version
- **Result**: FAILURE (API still down)
- **Priority**: P0 (critical)
- **AI Decision**: CALL AI (P0 failure requires deep analysis)

### Sequence Diagram

```mermaid
sequenceDiagram
    participant K8s as Kubernetes API
    participant RR as RemediationRequest CRD
    participant EMC as EffectivenessMonitor<br/>Controller
    participant EMS as EffectivenessMonitor<br/>Service
    participant HG as HolmesGPT API<br/>(AI Analysis)
    participant LLM as LLM Provider<br/>(GPT-4)
    participant DS as Data Storage<br/>(PostgreSQL)
    participant IM as Infrastructure<br/>Monitoring

    Note over K8s,LLM: Example: P0 API outage - rollback FAILED

    %% Step 1: Remediation completes with failure
    K8s->>RR: Update status.overallPhase = "failed"
    RR->>RR: Set success = false<br/>priority = "P0"<br/>failureReason = "API still unreachable"

    %% Step 2: Controller detects
    EMC->>RR: Watch event (overallPhase="failed", priority="P0")
    EMC->>EMC: Wait 5 minutes<br/>(stabilization period)

    Note over EMC: Watches RemediationRequest (user-facing API)<br/>Future-proof against workflow implementation changes

    EMC->>DS: Check if already processed
    DS-->>EMC: Not processed

    %% Step 3: Trigger assessment
    EMC->>EMS: POST /internal/assess<br/>{<br/>  "workflow_execution_uid": "def-456",<br/>  "trace_id": "trace-p0-abc",<br/>  "action_type": "rollback_deployment",<br/>  "priority": "P0"<br/>}

    %% Step 4: Automated assessment
    EMS->>DS: Get pre-execution metrics
    DS-->>EMS: {"api_latency": "timeout", "error_rate": "100%"}

    EMS->>IM: Get post-execution metrics
    IM-->>EMS: {"api_latency": "timeout", "error_rate": "100%"}

    EMS->>EMS: Calculate automated assessment:<br/>- Basic score: 0.15 (low)<br/>- Health checks: FAIL<br/>- No improvement detected<br/>- Anomalies: ["no_metric_improvement"]

    %% Step 5: Decision point - YES AI
    EMS->>EMS: shouldCallAI()?<br/>✅ P0 priority + FAILURE<br/>✅ Anomalies detected<br/>→ TRUE (AI analysis required)

    Note over EMS: AI call triggered - P0 failure needs deep analysis

    %% Step 6: Build AI context (Self-Documenting JSON)
    EMS->>EMS: Build investigation context<br/>(DD-HOLMESGPT-009 format)

    Note over EMS: Self-documenting JSON:<br/>~290 tokens, 0 legend overhead

    %% Step 7: Call HolmesGPT API
    EMS->>HG: POST /api/v1/postexec/analyze<br/>{<br/>  "investigation_id": "p0-rollback-def456",<br/>  "priority": "P0",<br/>  "environment": "production",<br/>  "action": {<br/>    "type": "rollback_deployment",<br/>    "target": "api-service",<br/>    "namespace": "production"<br/>  },<br/>  "pre_execution_state": {<br/>    "api_latency": "timeout",<br/>    "error_rate": 100,<br/>    "pod_status": "CrashLoopBackOff"<br/>  },<br/>  "post_execution_state": {<br/>    "api_latency": "timeout",<br/>    "error_rate": 100,<br/>    "pod_status": "CrashLoopBackOff"<br/>  },<br/>  "execution_success": false,<br/>  "task": "Analyze why rollback failed..."<br/>}

    Note over HG: Self-documenting keys:<br/>No legend needed

    %% Step 8: HolmesGPT processes
    HG->>HG: Parse self-documenting JSON<br/>(100% accuracy)
    HG->>LLM: Analyze with full context
    LLM-->>HG: AI insights

    %% Step 9: AI analysis response
    HG-->>EMS: 200 OK<br/>{<br/>  "analysis_id": "ai-analysis-789",<br/>  "root_cause": "database_connection_lost",<br/>  "recommendations": [<br/>    {<br/>      "id": "rec-001",<br/>      "action": "check_database_connectivity",<br/>      "probability": 0.92,<br/>      "rationale": "Rollback version also depends on DB..."<br/>    }<br/>  ],<br/>  "effectiveness_assessment": {<br/>    "action_addressed_symptom": true,<br/>    "action_addressed_root_cause": false,<br/>    "likely_outcome": "problem_masked_not_solved"<br/>  },<br/>  "confidence": 0.88<br/>}

    %% Step 10: Combine results
    EMS->>EMS: Combine automated + AI:<br/>- Automated score: 0.15<br/>- AI insights: root cause DB issue<br/>- Final score: 0.20<br/>- High confidence: 0.88 (AI-backed)

    %% Step 11: Store combined results
    EMS->>DS: Store assessment:<br/>{<br/>  "effectiveness_score": 0.20,<br/>  "confidence": 0.88,<br/>  "analysis_type": "automated_plus_ai",<br/>  "ai_analysis_id": "ai-analysis-789",<br/>  "root_cause": "database_connection_lost",<br/>  "lessons": [...]<br/>}
    DS-->>EMS: Stored (assessment_id: assess-p0-123)

    %% Step 12: Update status
    EMS->>RR: Update annotations:<br/>"effectiveness.kubernaut.io/score": "0.20"<br/>"effectiveness.kubernaut.io/root-cause": "database_connection_lost"<br/>"effectiveness.kubernaut.io/ai-analyzed": "true"
    RR-->>EMS: Updated

    EMS-->>EMC: 200 OK<br/>{<br/>  "assessment_id": "assess-p0-123",<br/>  "effectiveness_score": 0.20,<br/>  "analysis_used": "automated_plus_ai",<br/>  "ai_confidence": 0.88<br/>}

    Note over K8s,LLM: Total time: ~3-5s<br/>Cost: ~$0.50 (LLM API)<br/>Value: 88% confidence root cause identified
```

### Key Points - Automated + AI

| Aspect | Value |
|--------|-------|
| **Trigger** | P0 failure, anomalies, new action type, oscillation |
| **Duration** | ~3-5s (AI processing) |
| **Cost** | ~$0.50 per analysis (LLM API) |
| **Frequency** | 25.5K/year (0.7% of all assessments) |
| **Confidence** | 85-95% (AI-backed insights) |
| **Components** | +HolmesGPT API, +LLM Provider |
| **Format** | Self-Documenting JSON (DD-HOLMESGPT-009) |

---

## Decision Matrix

### When is AI Called?

```
┌─────────────────────────────────────────────────────────────────────┐
│                  shouldCallAI() Decision Logic                       │
└─────────────────────────────────────────────────────────────────────┘

IF (Priority == "P0" AND Success == false)
   → ✅ CALL AI (Learn from critical failures)

ELSE IF (IsNewActionType == true)
   → ✅ CALL AI (Build knowledge base)

ELSE IF (len(anomalies) > 0)
   → ✅ CALL AI (Investigate unexpected behavior)

ELSE IF (IsRecurringFailure == true)
   → ✅ CALL AI (Detect oscillation patterns)

ELSE
   → ❌ SKIP AI (Routine success, automated sufficient)
```

### Annual Volume Breakdown

| Scenario | Volume/Year | AI Called? | Analysis Type |
|----------|-------------|------------|---------------|
| **P0 Failures** | 18,250 | ✅ YES | Automated + AI |
| **New Action Types** | 3,650 | ✅ YES | Automated + AI |
| **Anomalies Detected** | 1,825 | ✅ YES | Automated + AI |
| **Oscillations** | 1,825 | ✅ YES | Automated + AI |
| **Routine Successes** | 3,650,000 | ❌ NO | Automated Only |
| **TOTAL** | **3,675,550** | **25.5K (0.7%)** | Hybrid |

---

## Data Flow Comparison

### Watch Strategy Note
**Design Decision**: [DD-EFFECTIVENESS-003](../decisions/DD-EFFECTIVENESS-003-RemediationRequest-Watch-Strategy.md)

The Effectiveness Monitor watches **RemediationRequest** CRD instead of WorkflowExecution:
- **Trigger**: `RR.status.overallPhase IN ("completed", "failed", "timeout")`
- **Aggregation**: RR aggregates child CRD statuses (RemediationProcessing, AIAnalysis, WorkflowExecution)
- **Future-Proof**: Decoupled from workflow implementation changes
- **Multi-Workflow**: Handles future scenarios with multiple workflows per remediation

All required data is available in `RR.status.workflowExecutionStatus` (summary). Detailed WE info can be fetched if needed (rare).

### Automated Only Flow
```
RemediationRequest CRD (status.overallPhase = "completed")
    ↓
Controller (5-min delay)
    ↓
Service: Automated Assessment
    ├─ Get pre-execution metrics (Data Storage)
    ├─ Get post-execution metrics (Infra Monitoring)
    ├─ Calculate basic score
    └─ shouldCallAI() → FALSE
    ↓
Store automated results (Data Storage)
    ↓
Update RR annotations
    ↓
DONE (~150ms, negligible cost)
```

### Automated + AI Flow
```
RemediationRequest CRD (status.overallPhase = "failed", priority = "P0")
    ↓
Controller (5-min delay)
    ↓
Service: Automated Assessment
    ├─ Get pre-execution metrics (Data Storage)
    ├─ Get post-execution metrics (Infra Monitoring)
    ├─ Calculate basic score
    ├─ Detect anomalies
    └─ shouldCallAI() → TRUE (P0 failure)
    ↓
Build Self-Documenting JSON Context (DD-HOLMESGPT-009)
    ├─ investigation_id, priority, environment
    ├─ action details, pre/post state
    └─ task directive (~290 tokens, 0 legend)
    ↓
Call HolmesGPT API POST /api/v1/postexec/analyze
    ↓
HolmesGPT → LLM Provider (GPT-4)
    ├─ Parse natural language JSON
    ├─ Analyze root cause
    └─ Generate recommendations
    ↓
Receive AI Analysis
    ├─ Root cause identification
    ├─ Effectiveness assessment
    └─ Confidence: 85-95%
    ↓
Combine automated + AI results
    ↓
Store combined results (Data Storage)
    ↓
Update CRD with AI insights
    ↓
DONE (~3-5s, ~$0.50 LLM cost)
```

---

## Cost/Benefit Analysis

### Hybrid Approach Economics

| Metric | Automated Only | Automated + AI | Hybrid (0.7% AI) |
|--------|----------------|----------------|------------------|
| **Volume/Year** | 3,650,000 | 25,550 | 3,675,550 |
| **Avg Duration** | 150ms | 3-5s | ~155ms avg |
| **Cost/Assessment** | $0.0001 | $0.50 | ~$0.0035 avg |
| **Annual Cost** | $365 | $12,775 | **$13,140** |
| **Confidence** | 70-80% | 85-95% | ~80% weighted |
| **Effectiveness** | 70% | 90% | **85-90%** |

**ROI Calculation**:
- **Additional Cost**: $12,775/year (AI calls)
- **Value Gained**: 15-20% effectiveness improvement
- **Prevented Incidents**: ~140 critical failures/year avoided
- **Incident Cost**: ~$1,000/incident (average)
- **Value**: $140,000/year
- **ROI**: **11x return on investment**

---

## Integration Points

### Services Calling Effectiveness Monitor

1. **Context API Service** - Retrieves effectiveness assessments
2. **Internal Controller** - Triggers post-execution assessments

### Services Called by Effectiveness Monitor

1. **Data Storage (PostgreSQL)** - Critical
   - Pre/post-execution metrics
   - Historical effectiveness data
   - Assessment results storage

2. **Infrastructure Monitoring (Prometheus)** - Graceful degradation
   - Current metrics (CPU, memory, latency)
   - Pod health status
   - Anomaly thresholds

3. **HolmesGPT API** - Selective (0.7% of cases)
   - POST /api/v1/postexec/analyze
   - Self-documenting JSON format (DD-HOLMESGPT-009)
   - Async, non-blocking

---

## References

- **Architecture**: [Effectiveness Monitor Overview](../services/stateless/effectiveness-monitor/overview.md)
- **Decision**: [DD-EFFECTIVENESS-001: Hybrid Automated + AI Analysis](decisions/DD-EFFECTIVENESS-001-Hybrid-Automated-AI-Analysis.md)
- **API Spec**: [Effectiveness Monitor API](../services/stateless/effectiveness-monitor/api-specification.md)
- **Integration**: [Integration Points](../services/stateless/effectiveness-monitor/integration-points.md)
- **Format**: [DD-HOLMESGPT-009: Self-Documenting JSON](decisions/DD-HOLMESGPT-009-Ultra-Compact-JSON-Format.md)

---

## Summary

The Effectiveness Monitor's hybrid approach provides:

✅ **Cost-Efficient**: 99.3% automated (fast, cheap)
✅ **High-Value AI**: 0.7% AI analysis (selective, targeted)
✅ **Excellent ROI**: 11x return on investment
✅ **High Confidence**: 85-95% with AI backing
✅ **Scalable**: Handles 3.65M assessments/year
✅ **Self-Documenting**: Zero legend overhead (DD-HOLMESGPT-009)

