# ADR-044: Workflow Execution Engine Delegation

**Status**: ✅ Approved
**Version**: 1.0
**Date**: 2025-11-28
**Confidence**: 98%

---

## Context

The WorkflowExecution controller needs to execute remediation workflows. Two architectural approaches exist:

1. **Complex Orchestration**: Controller parses workflow steps, creates per-step CRDs, manages dependencies, handles rollback
2. **Engine Delegation**: Controller creates a single execution request, delegates all orchestration to the execution engine (Tekton)

---

## Problem

The current implementation plan (v1.3) assumes complex orchestration:
- 30-33 days implementation
- Per-step KubernetesExecution CRDs
- Step dependency resolution
- Parallel execution coordination
- Rollback logic
- 38 BRs

This approach has significant drawbacks:
1. **Duplicates engine functionality**: Tekton already handles step orchestration
2. **Tight coupling**: Controller tied to Tekton internals
3. **Maintenance burden**: Any workflow change requires controller updates
4. **Engine lock-in**: Hard to support alternative engines (Argo, custom)

---

## Decision

**APPROVED: Engine Delegation (Option B)**

The WorkflowExecution controller:
1. **Creates a single Tekton PipelineRun** from OCI bundle
2. **Passes parameters** directly to the PipelineRun
3. **Monitors completion status** (Success/Failed)
4. **Does NOT orchestrate steps** - Tekton handles this
5. **Does NOT implement rollback** - Tekton `finally` tasks or operator responsibility
6. **Does NOT transform workflows** - OCI bundle used directly

---

## Rationale

### Why Engine Delegation?

| Aspect | Complex Orchestration | Engine Delegation |
|--------|----------------------|-------------------|
| **Implementation** | 30-33 days | ~12 days |
| **Step handling** | Controller logic | Tekton Pipeline |
| **Rollback** | Controller logic | Tekton finally / N/A |
| **Engine coupling** | Tight | Loose |
| **Engine portability** | Low | High |
| **Maintenance** | High | Low |

### Key Principles

1. **Single Responsibility**: Controller manages CRD lifecycle, engine executes workflow
2. **Engine Agnosticism**: Same controller could work with Argo Workflows (future)
3. **No Transformation**: OCI bundle → PipelineRun without modification
4. **Fault Isolation**: Execution failures are engine's domain, not ours

---

## Implementation

### Simplified CRD Schema

```go
type WorkflowExecutionSpec struct {
    // Parent reference for audit trail
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // OCI bundle reference (from catalog)
    WorkflowRef WorkflowRef `json:"workflowRef"`

    // Parameters from LLM (UPPER_SNAKE_CASE)
    Parameters map[string]string `json:"parameters"`

    // LLM confidence for audit
    Confidence float64 `json:"confidence,omitempty"`

    // Minimal execution config
    ExecutionConfig ExecutionConfig `json:"executionConfig,omitempty"`
}

type WorkflowRef struct {
    WorkflowID      string `json:"workflowId"`
    Version         string `json:"version"`
    ContainerImage  string `json:"containerImage"`   // OCI bundle URL
    ContainerDigest string `json:"containerDigest,omitempty"`
}

type ExecutionConfig struct {
    ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

type WorkflowExecutionStatus struct {
    Phase           string                    `json:"phase"` // Pending → Running → Completed/Failed
    PipelineRunRef  *corev1.ObjectReference   `json:"pipelineRunRef,omitempty"`
    StartTime       *metav1.Time              `json:"startTime,omitempty"`
    CompletionTime  *metav1.Time              `json:"completionTime,omitempty"`
    Outcome         string                    `json:"outcome,omitempty"` // Success, Failed, Timeout
    Message         string                    `json:"message,omitempty"`
    Conditions      []metav1.Condition        `json:"conditions,omitempty"`
}
```

### Controller Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│  WorkflowExecution Controller (Simplified)                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Phase: Pending                                                         │
│  ├── Validate spec (workflowRef, parameters)                           │
│  ├── Create Tekton PipelineRun:                                        │
│  │     pipelineRef:                                                     │
│  │       resolver: bundles                                              │
│  │       params:                                                        │
│  │         - bundle: spec.workflowRef.containerImage                   │
│  │     params: spec.parameters (converted to Tekton format)            │
│  └── Update status.phase = "Running"                                   │
│                                                                         │
│  Phase: Running                                                         │
│  ├── Watch PipelineRun status                                          │
│  ├── If PipelineRun.IsDone():                                          │
│  │     ├── Succeeded → phase = "Completed", outcome = "Success"        │
│  │     └── Failed → phase = "Failed", outcome = "Failed"               │
│  └── Requeue after 10s if still running                                │
│                                                                         │
│  NOTE: Global timeout (DD-TIMEOUT-001) handled by RemediationOrchestrator
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## What We DON'T Do

| Responsibility | Owner | Rationale |
|----------------|-------|-----------|
| **Step orchestration** | Tekton | Tekton Pipeline handles DAG |
| **Step dependencies** | Tekton | `runAfter` in Pipeline spec |
| **Parallel execution** | Tekton | Pipeline parallelism |
| **Step timeouts** | Tekton | Task timeout in Pipeline |
| **Rollback** | Tekton / N/A | `finally` tasks or not our concern |
| **Workflow timeout** | Tekton | PipelineRun timeout |
| **Retry logic** | Tekton | Task retry in Pipeline |

---

## Timeout Strategy

| Level | Handler | Default | Configurable |
|-------|---------|---------|--------------|
| **Workflow timeout** | Tekton PipelineRun | From workflow-schema.yaml | Yes |
| **Task timeout** | Tekton Task | From Pipeline definition | Yes |
| **Global remediation** | RemediationOrchestrator | 1 hour (DD-TIMEOUT-001) | Yes |

**Fallback**: If Tekton timeout fails, global timeout in DD-TIMEOUT-001 ensures no stuck remediations.

---

## Rollback Decision

**We do NOT implement rollback because:**

1. **No knowledge of workflow internals**: We don't parse the Pipeline, so we can't know what to rollback
2. **Engine responsibility**: Tekton `finally` tasks can handle cleanup if workflow author designs it
3. **Operator responsibility**: If remediation fails, operator decides next action
4. **Complexity avoidance**: Rollback logic is complex and error-prone
5. **Blast radius control**: We don't want to cause additional damage by automated rollback

**If rollback is needed**: Workflow author adds `finally` tasks to the Tekton Pipeline in the OCI bundle.

---

## Engine Portability (Future)

This architecture enables future support for alternative engines:

```go
type WorkflowRef struct {
    WorkflowID      string `json:"workflowId"`
    ContainerImage  string `json:"containerImage"`
    Engine          string `json:"engine,omitempty"` // "tekton" (default), "argo" (future)
}
```

Controller can dispatch to different engine handlers without changing the CRD contract.

---

## Consequences

### Positive

- ✅ **60% effort reduction**: ~12 days vs 33 days
- ✅ **Engine agnostic**: Same interface for different engines
- ✅ **No transformation**: OCI bundle used directly
- ✅ **Clear responsibility**: Controller = CRD lifecycle, Engine = execution
- ✅ **Lower maintenance**: No step orchestration code to maintain

### Negative

- ⚠️ **Less visibility**: Can't report per-step status (only overall)
  - **Mitigation**: Tekton TaskRun status available for deep debugging
- ⚠️ **No rollback**: Controller can't undo failed workflows
  - **Mitigation**: Workflow author uses `finally` tasks

### Neutral

- 🔄 **Existing plan obsolete**: v1.3 plan needs replacement
- 🔄 **BR reduction**: Many BRs no longer applicable

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| **DD-CONTRACT-001** | WorkflowRef schema definition |
| **ADR-043** | Workflow schema in OCI bundle |
| **DD-TIMEOUT-001** | Global timeout as fallback |
| **ADR-024** | Eliminate ActionExecution layer (precursor) |

---

## Supersedes

- WorkflowExecution Implementation Plan v1.3 (complex orchestration)
- DD-002: Per-Step Validation Framework (no longer applicable)
- BR-WF-016, BR-WF-052, BR-WF-053 (step validation BRs)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-11-28 | Initial ADR: Engine delegation architecture approved |

