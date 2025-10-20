# Multi-Step Remediation Capabilities Analysis

**Date**: October 17, 2025
**Purpose**: Address concerns about whether action constraints prevent sophisticated multi-step remediation workflows
**Question**: "Does restricting valid action types prevent HolmesGPT from generating complex multi-step remediations?"

---

## 🎯 **TL;DR - YOUR CONCERN IS VALID BUT ADDRESSED**

**Your Concern**: By restricting HolmesGPT to predefined action types (e.g., OOMKill → `increase_resources`, `scale_deployment`, `escalate`), we might prevent sophisticated multi-step remediation workflows.

**Reality**: ✅ **Multi-step workflows ARE fully supported**

**Key Distinction**:
- **Action TYPES are constrained** (29 canonical actions) ← This is intentional for safety/validation
- **Workflow COMPOSITION is flexible** (HolmesGPT composes actions into multi-step workflows) ← This enables sophistication

**Think of it like LEGO blocks**: You have a finite set of block types, but infinite combinations create complex structures.

---

## 📋 **ARCHITECTURE OVERVIEW**

### **How Multi-Step Workflows Work**

```
┌─────────────────────────────────────────────────────────────┐
│ HolmesGPT AI Analysis                                      │
│                                                             │
│ INPUT: Alert (OOMKill), Context (metrics, logs, history)  │
│                                                             │
│ OUTPUT: Multi-Step Workflow with Dependencies             │
│ ┌─────────────────────────────────────────────────────┐   │
│ │ Step 1: collect_diagnostics                        │   │
│ │   deps: []                                          │   │
│ │   why: "Capture heap dump before restart"          │   │
│ ├─────────────────────────────────────────────────────┤   │
│ │ Step 2: increase_resources                         │   │
│ │   deps: [1]                                         │   │
│ │   why: "Prevent immediate OOMKill recurrence"      │   │
│ ├─────────────────────────────────────────────────────┤   │
│ │ Step 3: restart_pod                                │   │
│   deps: [1,2]                                        │   │
│ │   why: "Apply new resource limits"                 │   │
│ ├─────────────────────────────────────────────────────┤   │
│ │ Step 4: monitor_metrics (custom params)            │   │
│ │   deps: [3]                                         │   │
│ │   why: "Verify memory stabilized"                  │   │
│ ├─────────────────────────────────────────────────────┤   │
│ │ Step 5: update_hpa                                 │   │
│ │   deps: [4]                                         │   │
│ │   why: "Adjust HPA to prevent scale-out loops"     │   │
│ └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Key Point**: HolmesGPT **composes** 29 action types into **arbitrary multi-step workflows** using:
1. **Dependencies** (`deps: []`) - Define execution order
2. **Parameters** (action-specific) - Customize behavior
3. **Risk Levels** (low/medium/high/critical) - Safety gates
4. **Confidence Scores** (0-1.0) - Success probability

---

## 🔍 **REALISTIC EXAMPLE: OOMKill Multi-Step Remediation**

### **Scenario: Java Microservice Memory Leak**

**Alert**: `OOMKilled` for `payment-service` pod
**Context**:
- JVM heap usage: 1.8GB / 2GB limit (90% utilization)
- Memory grows 50MB/hour (linear leak pattern)
- Similar incident 3 weeks ago (92% success with 5-step workflow)
- Production environment (requires approval for restarts)

### **HolmesGPT Generated Workflow (7 Steps)**

**Source**: HolmesGPT prompt includes 29 canonical action types + dependency specification

```json
{
  "recommendations": [
    {
      "id": "rec-001",
      "action": "collect_diagnostics",
      "description": "Capture heap dump before restart for post-mortem analysis",
      "target": {
        "kind": "Pod",
        "name": "payment-service-abc123",
        "namespace": "production"
      },
      "parameters": {
        "diagnosticType": "heap_dump",
        "path": "/tmp/heap-dump-20251017.hprof",
        "compress": true
      },
      "dependencies": [],
      "probability": 0.98,
      "riskLevel": "low",
      "reasoning": {
        "rootCause": "Memory leak in payment processing coroutine (50MB/hr growth)",
        "evidence": [
          "Linear memory growth pattern over 6 hours",
          "Similar incident resolved 3 weeks ago with heap dump analysis",
          "No code deployment in last 24h (triggered by traffic pattern)"
        ],
        "historicalSuccess": 0.92,
        "alternativeConsidered": "Immediate restart without diagnostics (faster but loses forensic data)"
      }
    },
    {
      "id": "rec-002",
      "action": "backup_data",
      "description": "Backup pod logs before restart",
      "target": {
        "kind": "Pod",
        "name": "payment-service-abc123",
        "namespace": "production"
      },
      "parameters": {
        "destination": "s3://incident-logs/2025-10-17/oomkill-payment-service",
        "include": ["application.log", "gc.log", "heap-dump"]
      },
      "dependencies": ["rec-001"],
      "probability": 0.95,
      "riskLevel": "low",
      "reasoning": {
        "rationale": "Preserve forensic evidence for development team investigation"
      }
    },
    {
      "id": "rec-003",
      "action": "increase_resources",
      "description": "Increase memory limit from 2Gi to 3Gi",
      "target": {
        "kind": "Deployment",
        "name": "payment-service",
        "namespace": "production"
      },
      "parameters": {
        "resources": {
          "limits": {
            "memory": "3Gi"
          },
          "requests": {
            "memory": "2.5Gi"
          }
        }
      },
      "dependencies": ["rec-001", "rec-002"],
      "probability": 0.88,
      "riskLevel": "medium",
      "reasoning": {
        "rationale": "50MB/hr leak requires 1Gi buffer for 20-hour safety margin before next OOMKill",
        "costImpact": "+$50/month per replica (acceptable for production stability)",
        "historicalSuccess": 0.92
      }
    },
    {
      "id": "rec-004",
      "action": "restart_pod",
      "description": "Rolling restart to apply new resource limits",
      "target": {
        "kind": "Deployment",
        "name": "payment-service",
        "namespace": "production"
      },
      "parameters": {
        "strategy": "one-at-a-time",
        "gracePeriod": "30s",
        "waitForReady": true,
        "healthCheck": {
          "endpoint": "/health",
          "expectedStatus": 200
        }
      },
      "dependencies": ["rec-003"],
      "probability": 0.95,
      "riskLevel": "medium",
      "reasoning": {
        "rationale": "Graceful restart minimizes service disruption (30s drain)",
        "approvalRequired": true,
        "reason": "Production restart requires manual approval"
      }
    },
    {
      "id": "rec-005",
      "action": "enable_debug_mode",
      "description": "Enable memory profiling for 1 hour",
      "target": {
        "kind": "Deployment",
        "name": "payment-service",
        "namespace": "production"
      },
      "parameters": {
        "debugLevel": "memory-profiling",
        "duration": "1h",
        "endpoint": "/debug/pprof"
      },
      "dependencies": ["rec-004"],
      "probability": 0.90,
      "riskLevel": "low",
      "reasoning": {
        "rationale": "Capture live profiling data to identify leak source",
        "performanceImpact": "5-10% CPU overhead (acceptable for troubleshooting)"
      }
    },
    {
      "id": "rec-006",
      "action": "update_hpa",
      "description": "Adjust HPA to prevent scale-out loops",
      "target": {
        "kind": "HorizontalPodAutoscaler",
        "name": "payment-service-hpa",
        "namespace": "production"
      },
      "parameters": {
        "maxReplicas": 6,
        "targetMemoryUtilization": 70,
        "scaleDownStabilization": "10m"
      },
      "dependencies": ["rec-004"],
      "probability": 0.85,
      "riskLevel": "low",
      "reasoning": {
        "rationale": "Prevent HPA from scaling out due to leak (makes problem worse)",
        "originalMax": 10,
        "newMax": 6,
        "reason": "Cap replicas until leak is fixed in code"
      }
    },
    {
      "id": "rec-007",
      "action": "notify_only",
      "description": "File bug report with heap dump analysis",
      "target": {
        "kind": "Notification",
        "name": "dev-team-slack",
        "namespace": "production"
      },
      "parameters": {
        "channel": "#payment-team",
        "priority": "high",
        "attachments": [
          "heap-dump.hprof",
          "memory-profile.json",
          "root-cause-analysis.md"
        ],
        "actionRequired": "Code fix needed for memory leak in payment processing coroutine"
      },
      "dependencies": ["rec-005"],
      "probability": 1.0,
      "riskLevel": "low",
      "reasoning": {
        "rationale": "Immediate fix is temporary, code fix required for permanent solution"
      }
    }
  ],
  "workflowMetadata": {
    "totalSteps": 7,
    "estimatedDuration": "4-5 minutes",
    "criticalPath": ["rec-001", "rec-002", "rec-003", "rec-004"],
    "parallelizable": {
      "rec-005": "Can run parallel with rec-006 after rec-004",
      "rec-006": "Can run parallel with rec-005 after rec-004"
    },
    "approvalGates": ["rec-004"],
    "rollbackPlan": {
      "rec-003": "Revert resource limits to original values",
      "rec-004": "Rollback deployment to previous ReplicaSet",
      "rec-006": "Restore original HPA settings"
    }
  }
}
```

### **Workflow Execution Timeline**

```
Time: 0s ────────────────────── 4 min ────────────── End
       │                                              │
       │ rec-001 (collect_diagnostics) [30s]         │
       │   ↓                                          │
       │ rec-002 (backup_data) [60s] ───────────┐    │
       │   ↓                                     │    │
       │ rec-003 (increase_resources) [20s]     │    │
       │   ↓                                     │    │
       │ rec-004 (restart_pod) [90s] ⚠️ APPROVAL │    │
       │   ↓                                     │    │
       │ rec-005 (enable_debug) [10s] ──┐       │    │
       │                                 │       │    │
       │ rec-006 (update_hpa) [10s] ────┴───────┴────┤
       │   ↓                                          │
       │ rec-007 (notify_only) [5s]                   │
       │                                              │
       └──────────────────────────────────────────────┘

Total MTTR: 4 minutes (vs 60-90 min manual)
```

---

## 🔑 **KEY ARCHITECTURAL INSIGHTS**

### **1. Action Types Are Constrained (Intentionally)**

**Why 29 Canonical Actions?**
- ✅ **Safety**: Each action has validated handlers, RBAC policies, rollback procedures
- ✅ **Validation**: System can validate workflows at creation time (no unknown actions)
- ✅ **Testing**: 29 actions = finite test matrix, achievable 100% coverage
- ✅ **Documentation**: Every action has clear docs, examples, safety guidelines
- ✅ **Extensibility**: New actions added via formal process (not ad-hoc)

**Source**: `docs/design/CANONICAL_ACTION_TYPES.md`

### **2. Workflow Composition Is Unlimited**

**What Makes Workflows Sophisticated?**
- ✅ **Dependencies**: Arbitrary DAG (directed acyclic graph) of steps
- ✅ **Parallel Execution**: Independent steps run simultaneously
- ✅ **Conditional Logic**: Risk levels, confidence scores trigger different paths
- ✅ **Custom Parameters**: Each action customizable (e.g., `gracePeriod: 30s`)
- ✅ **Safety Gates**: Approval requirements, dry-run validation, rollback plans

**Example Complexity**:
- **Diamond Pattern**: Step 1 → (Step 2 || Step 3) → Step 4
- **Fan-Out**: Step 1 → [Step 2, Step 3, Step 4, Step 5] (all parallel)
- **Conditional**: If `riskLevel: high` → require approval, else auto-execute

**Source**: `docs/analysis/WORKFLOW_EXECUTION_MODE_DETERMINATION.md`

### **3. HolmesGPT Prompt Engineering**

**How Does HolmesGPT Know What Actions Exist?**

**System Prompt** (Simplified):
```python
SYSTEM_PROMPT = """
You are HolmesGPT, Kubernetes troubleshooting expert.

VALID ACTIONS (29 types):
- Core: scale_deployment, restart_pod, increase_resources, rollback_deployment, expand_pvc
- Infrastructure: drain_node, cordon_node, uncordon_node, taint_node, untaint_node, quarantine_pod
- Storage: cleanup_storage, backup_data, compact_storage
- Application: update_hpa, restart_daemonset, scale_statefulset
- Security: rotate_secrets, audit_logs, update_network_policy
- Network: restart_network, reset_service_mesh
- Database: failover_database, repair_database
- Monitoring: enable_debug_mode, create_heap_dump, collect_diagnostics
- Resource: optimize_resources, migrate_workload
- Fallback: notify_only

COMPOSE these actions into multi-step workflows using:
- Dependencies (deps): Define execution order
- Parameters: Customize each action
- Risk levels: low/medium/high/critical
- Confidence: 0.0-1.0 success probability

DEPENDENCY PATTERNS:
- Sequential: Step 2 after Step 1 → {"id":"rec-002","deps":["rec-001"]}
- Parallel: Steps 2,3 after Step 1 → both have deps:["rec-001"]
- Join: Step 4 after Steps 2,3 → {"id":"rec-004","deps":["rec-002","rec-003"]}
- Conditional: If high risk → set riskLevel: "high" (triggers approval gate)

EXAMPLE: OOMKill Remediation
1. collect_diagnostics (deps:[]) → Capture heap dump
2. increase_resources (deps:[1]) → Prevent recurrence
3. restart_pod (deps:[1,2]) → Apply new limits
4. update_hpa (deps:[3]) → Prevent scale-out loops
5. notify_only (deps:[3,4]) → File bug report

VALIDATION:
- All action types must be from VALID ACTIONS list
- All dep IDs must exist
- No circular dependencies
"""
```

**Key Point**: HolmesGPT is **constrained to 29 action types** but **free to compose them** into arbitrary multi-step workflows.

**Source**: `docs/services/crd-controllers/02-aianalysis/prompt-engineering-dependencies.md`

---

## 🚨 **ADDRESSING YOUR SPECIFIC CONCERN**

### **Your Example: OOMKill → ["increase_memory", "increase_replica_count", "escalate"]**

**Question**: "If we restrict responses to these 3 options, don't we prevent multi-step workflows?"

**Answer**: ❌ **This is a MISUNDERSTANDING of the architecture**

**What Actually Happens**:

1. **HolmesGPT Receives**: OOMKill alert + context (metrics, logs, history)
2. **HolmesGPT Analyzes**: Root cause = memory leak in payment-service
3. **HolmesGPT Generates**: Multi-step workflow using 29 action types:
   ```json
   {
     "recommendations": [
       {"id": "rec-001", "action": "collect_diagnostics", "deps": []},
       {"id": "rec-002", "action": "backup_data", "deps": ["rec-001"]},
       {"id": "rec-003", "action": "increase_resources", "deps": ["rec-001","rec-002"]},
       {"id": "rec-004", "action": "restart_pod", "deps": ["rec-003"]},
       {"id": "rec-005", "action": "update_hpa", "deps": ["rec-004"]},
       {"id": "rec-006", "action": "notify_only", "deps": ["rec-005"]}
     ]
   }
   ```

**NOT Constrained To**:
```json
{
  "recommendations": [
    {"action": "increase_memory"},
    {"action": "increase_replica_count"},
    {"action": "escalate"}
  ]
}
```

**The Difference**:
- **Action TYPES** (29 canonical) ← Constrained for safety
- **Action COMPOSITION** (unlimited combinations) ← Flexible for sophistication
- **Action PARAMETERS** (action-specific) ← Customizable for context

---

## 📊 **EXAMPLES OF MULTI-STEP SCENARIOS**

### **Scenario 1: Cascading Failure (7 Steps)**

**Source**: `docs/value-proposition/TECHNICAL_SCENARIOS.md` lines 302-360

**Problem**: `checkout-service` latency → traces to `payment-service` → traces to `fraud-detection-service` → **root cause**: PostgreSQL connection pool exhaustion

**Workflow**:
1. **backup_data** (database config) - deps: []
2. **increase_resources** (PostgreSQL pool size 100→200) - deps: [1]
3. **restart_pod** (fraud-detection) - deps: [2]
4. **scale_deployment** (fraud-detection 5→8 replicas) - deps: [3]
5. **restart_pod** (payment-service) - deps: [4]
6. **scale_deployment** (payment-service 8→10) - deps: [5]
7. **notify_only** (SRE team: root cause + GitOps PR) - deps: [6]

**MTTR**: 5 minutes (vs 45-60 min manual)

### **Scenario 2: Node Disk Pressure (5 Steps)**

**Problem**: Node disk at 95% capacity, mixed-criticality workloads at risk

**Workflow**:
1. **collect_diagnostics** (identify space-consuming process) - deps: []
2. **cleanup_storage** (delete old logs 40GB) - deps: [1]
3. **backup_data** (preserve audit logs) - deps: [1]
4. **cordon_node** (prevent new pods) - deps: [2,3]
5. **notify_only** (if cleanup insufficient, escalate for node expansion) - deps: [4]

**MTTR**: 3 minutes (vs 45-60 min manual)

### **Scenario 3: Database Deadlock (4 Steps)**

**Problem**: PostgreSQL deadlock in `orders` table

**Workflow**:
1. **collect_diagnostics** (capture lock wait graph) - deps: []
2. **backup_data** (table snapshot) - deps: []
3. **repair_database** (terminate deadlocked transactions) - deps: [1,2]
4. **notify_only** (DBA team: deadlock pattern analysis) - deps: [3]

**MTTR**: 7 minutes (vs 60-95 min manual DBA intervention)

**Source**: `docs/value-proposition/EXECUTIVE_SUMMARY.md` lines 88-96

---

## ✅ **VALIDATION: HOW DO WE KNOW MULTI-STEP WORKS?**

### **1. Documentation Evidence**

**File**: `docs/analysis/WORKFLOW_EXECUTION_MODE_DETERMINATION.md`
- Shows dependency-based execution planning
- Example: 7-step cascading failure workflow
- Parallel execution support (diamond patterns)

**File**: `docs/value-proposition/TECHNICAL_SCENARIOS.md`
- 6 detailed multi-step scenarios (Memory Leak, Cascading Failure, Config Drift, Disk Pressure, DB Deadlock, Alert Storm)
- Average 4-7 steps per workflow
- MTTR: 2-8 minutes (vs 30-120 min manual)

**File**: `docs/services/crd-controllers/02-aianalysis/prompt-engineering-dependencies.md`
- HolmesGPT prompt engineering for dependency specification
- Examples of sequential, parallel, join patterns
- Validation rules for dependency graphs

### **2. Test Evidence**

**File**: `test/integration/workflow_automation/execution/multi_stage_remediation_test.go`
- Integration tests for multi-step workflows
- Validates dependency resolution
- Tests parallel execution

### **3. CRD Schema Evidence**

**Field**: `AIAnalysis.spec.recommendations[].dependencies`
- Type: `[]string` (array of recommendation IDs)
- Purpose: Define execution order
- Validation: All IDs must exist, no circular deps

**Field**: `WorkflowExecution.spec.steps[].dependsOn`
- Type: `[]int` (array of step numbers)
- Purpose: Dependency graph for execution planning
- Used by: WorkflowExecution Controller for parallel optimization

---

## 🎯 **CONCLUSION**

### **Your Concern Was:**
> "By restricting HolmesGPT responses to predefined actions (e.g., OOMKill → increase_memory, increase_replicas, escalate), we prevent sophisticated multi-step remediations."

### **Reality:**

✅ **Action TYPES are constrained** (29 canonical) - **INTENTIONAL** for safety, validation, testing
✅ **Workflow COMPOSITION is unlimited** - HolmesGPT creates **arbitrary multi-step workflows**
✅ **Dependencies define order** - Sequential, parallel, conditional execution
✅ **Parameters customize behavior** - Each action has rich configuration
✅ **Real-world examples** - 6 documented scenarios, average 4-7 steps, MTTR 2-8 min

### **Think of It Like Programming:**
- **29 Action Types** = Functions in a standard library (e.g., Python's `os`, `shutil`, `subprocess`)
- **Workflow Composition** = Writing programs that call these functions in complex sequences
- **Dependencies** = Control flow (if/else, loops, parallel threads)
- **Parameters** = Function arguments (customize behavior)

**Result**: Finite action types, **infinite workflow sophistication**.

### **Example Analogy:**

```python
# You have 29 "functions" (action types):
def collect_diagnostics(...): ...
def increase_resources(...): ...
def restart_pod(...): ...
def update_hpa(...): ...
def notify_only(...): ...

# But you can compose them into sophisticated workflows:
def remediate_oomkill(pod, context):
    # Step 1: Capture forensics
    heap_dump = collect_diagnostics(pod, type="heap_dump")

    # Step 2: Prevent immediate recurrence
    if context.memory_leak_detected:
        increase_resources(pod.deployment, memory="3Gi")

    # Step 3: Apply fix
    restart_pod(pod.deployment, gracePeriod=30, waitForReady=True)

    # Step 4: Adjust autoscaling
    if context.hpa_enabled:
        update_hpa(pod.deployment, maxReplicas=6, targetMemory=70)

    # Step 5: Alert dev team
    notify_only(
        channel="#payment-team",
        priority="high",
        attachments=[heap_dump],
        action="Code fix needed for memory leak"
    )
```

**This is exactly what HolmesGPT does** - composes 29 action types into multi-step workflows with dependencies, parameters, and safety gates.

---

## 📚 **REFERENCES**

1. **Action Types**: `docs/design/CANONICAL_ACTION_TYPES.md`
2. **Workflow Dependencies**: `docs/analysis/WORKFLOW_EXECUTION_MODE_DETERMINATION.md`
3. **Multi-Step Scenarios**: `docs/value-proposition/TECHNICAL_SCENARIOS.md`
4. **HolmesGPT Prompts**: `docs/services/crd-controllers/02-aianalysis/prompt-engineering-dependencies.md`
5. **CRD Schemas**: `docs/architecture/CRD_SCHEMAS.md`
6. **Integration Tests**: `test/integration/workflow_automation/execution/`

---

**Document Owner**: Platform Architecture Team
**Review Frequency**: When new action types are added or workflow capabilities change
**Next Review Date**: 2026-01-17

